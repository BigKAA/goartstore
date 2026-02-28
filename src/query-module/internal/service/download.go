// download.go — сервис proxy download файлов из Storage Elements.
// Полный pipeline: FileRecord (cache/DB) → SE URL (Admin Module) → streaming download.
// Поддержка HTTP Range requests, ленивая очистка при 404 от SE.
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/bigkaa/goartstore/query-module/internal/adminclient"
	"github.com/bigkaa/goartstore/query-module/internal/domain/model"
	"github.com/bigkaa/goartstore/query-module/internal/repository"
	"github.com/bigkaa/goartstore/query-module/internal/seclient"
)

// Ошибки download service.
var (
	// ErrFileDeleted — файл помечен как удалённый (lazy cleanup).
	ErrFileDeleted = fmt.Errorf("файл удалён из Storage Element")
)

// Prometheus-метрики download.
var (
	downloadsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "qm_downloads_total",
		Help: "Общее количество запросов на скачивание (по статусу).",
	}, []string{"status"})

	downloadDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "qm_download_duration_seconds",
		Help:    "Длительность proxy download (от запроса до завершения streaming).",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300},
	})

	downloadBytesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "qm_download_bytes_total",
		Help: "Общее количество переданных байт при скачивании.",
	})

	activeDownloads = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "qm_active_downloads",
		Help: "Количество активных (in-progress) proxy downloads.",
	})

	lazyCleanupTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "qm_lazy_cleanup_total",
		Help: "Количество операций lazy cleanup (файл не найден на SE).",
	})
)

// DownloadService — сервис proxy download файлов из Storage Elements.
type DownloadService struct {
	fileRepo    repository.FileRepository
	cache       *CacheService
	adminClient *adminclient.Client
	seClient    *seclient.Client
	logger      *slog.Logger
}

// NewDownloadService создаёт сервис proxy download.
func NewDownloadService(
	fileRepo repository.FileRepository,
	cache *CacheService,
	adminClient *adminclient.Client,
	seClient *seclient.Client,
	logger *slog.Logger,
) *DownloadService {
	return &DownloadService{
		fileRepo:    fileRepo,
		cache:       cache,
		adminClient: adminClient,
		seClient:    seClient,
		logger:      logger.With(slog.String("component", "download_service")),
	}
}

// Download выполняет полный pipeline proxy download файла.
//
// Pipeline:
//  1. Получить FileRecord (из кэша или БД)
//  2. Получить SE URL из Admin Module (по storage_element_id)
//  3. Запросить файл у SE (пробросить Range header)
//  4. Если SE вернул 404 → lazy cleanup (mark deleted + invalidate cache)
//  5. Streaming copy в ResponseWriter с пробросом заголовков
//
// Возвращает ошибку только при невосстановимых проблемах. При 404 от SE
// записывает ответ клиенту напрямую и возвращает ErrFileDeleted.
func (ds *DownloadService) Download(ctx context.Context, w http.ResponseWriter, fileID, rangeHeader string) error {
	start := time.Now()
	activeDownloads.Inc()
	defer activeDownloads.Dec()

	// 1. Получить FileRecord (кэш или БД)
	record, err := ds.getFileRecord(ctx, fileID)
	if err != nil {
		downloadsTotal.WithLabelValues("error").Inc()
		return err
	}

	// Проверяем статус файла
	if record.Status == "deleted" {
		downloadsTotal.WithLabelValues("not_found").Inc()
		return ErrNotFound
	}

	// 2. Получить SE URL из Admin Module
	seInfo, err := ds.adminClient.GetStorageElement(ctx, record.StorageElementID)
	if err != nil {
		downloadsTotal.WithLabelValues("am_error").Inc()
		return fmt.Errorf("получение информации о SE %s: %w", record.StorageElementID, err)
	}

	ds.logger.Debug("SE URL получен",
		slog.String("file_id", fileID),
		slog.String("se_id", record.StorageElementID),
		slog.String("se_url", seInfo.URL),
	)

	// 3. Запросить файл у SE (streaming)
	resp, err := ds.seClient.Download(ctx, seInfo.URL, fileID, rangeHeader)
	if err != nil {
		downloadsTotal.WithLabelValues("se_error").Inc()
		return fmt.Errorf("скачивание файла %s из SE: %w", fileID, err)
	}
	defer resp.Body.Close()

	// 4. SE вернул 404 → lazy cleanup
	if resp.StatusCode == http.StatusNotFound {
		ds.logger.Warn("Файл не найден на SE, выполняется lazy cleanup",
			slog.String("file_id", fileID),
			slog.String("se_id", record.StorageElementID),
			slog.String("se_url", seInfo.URL),
		)
		ds.lazyCleanup(ctx, fileID)
		downloadsTotal.WithLabelValues("lazy_cleanup").Inc()
		return ErrFileDeleted
	}

	// Проверяем допустимые статусы: 200 (полный файл) или 206 (частичный контент)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		downloadsTotal.WithLabelValues("se_error").Inc()
		return fmt.Errorf("SE вернул неожиданный статус %d для файла %s", resp.StatusCode, fileID)
	}

	// 5. Streaming copy: проброс заголовков и тела ответа
	ds.copyHeaders(w, resp)
	w.WriteHeader(resp.StatusCode)

	written, err := io.Copy(w, resp.Body)
	if err != nil {
		// Ошибка при streaming — заголовки уже отправлены, логируем
		ds.logger.Error("Ошибка streaming download",
			slog.String("file_id", fileID),
			slog.Int64("bytes_written", written),
			slog.String("error", err.Error()),
		)
		downloadsTotal.WithLabelValues("stream_error").Inc()
		return nil // заголовки уже отправлены, не можем вернуть ошибку клиенту
	}

	// Обновляем метрики
	duration := time.Since(start)
	downloadsTotal.WithLabelValues("success").Inc()
	downloadDuration.Observe(duration.Seconds())
	downloadBytesTotal.Add(float64(written))

	ds.logger.Debug("Download завершён",
		slog.String("file_id", fileID),
		slog.Int64("bytes", written),
		slog.Duration("duration", duration),
		slog.Int("status", resp.StatusCode),
	)

	return nil
}

// getFileRecord получает FileRecord из кэша или БД.
func (ds *DownloadService) getFileRecord(ctx context.Context, fileID string) (*model.FileRecord, error) {
	// Проверяем кэш
	if record, ok := ds.cache.Get(fileID); ok {
		return record, nil
	}

	// Cache miss — запрос к БД
	record, err := ds.fileRepo.GetByID(ctx, fileID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("получение записи файла: %w", err)
	}

	// Сохраняем в кэш
	ds.cache.Set(fileID, record)

	return record, nil
}

// lazyCleanup помечает файл как удалённый в БД и инвалидирует кэш.
// Выполняется при 404 от SE — файл физически отсутствует на SE.
func (ds *DownloadService) lazyCleanup(ctx context.Context, fileID string) {
	lazyCleanupTotal.Inc()

	// Помечаем как удалённый в БД
	if err := ds.fileRepo.MarkDeleted(ctx, fileID); err != nil {
		ds.logger.Error("Ошибка lazy cleanup: не удалось пометить файл как удалённый",
			slog.String("file_id", fileID),
			slog.String("error", err.Error()),
		)
		return
	}

	// Инвалидируем кэш
	ds.cache.Delete(fileID)

	ds.logger.Info("Lazy cleanup завершён: файл помечен как удалённый",
		slog.String("file_id", fileID),
	)
}

// copyHeaders пробрасывает заголовки ответа SE в ответ клиенту.
// Копирует только релевантные заголовки для download.
func (ds *DownloadService) copyHeaders(w http.ResponseWriter, resp *http.Response) {
	// Заголовки для проброса
	headersToProxy := []string{
		"Content-Type",
		"Content-Length",
		"Content-Disposition",
		"Content-Range",
		"Accept-Ranges",
		"ETag",
		"Last-Modified",
		"Cache-Control",
	}

	for _, h := range headersToProxy {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
}
