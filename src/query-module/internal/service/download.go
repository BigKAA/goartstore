// download.go — сервис proxy download файлов из Storage Elements.
// Полный pipeline: FileRecord (cache/DB) → SE URL (Admin Module) → streaming download.
// Поддержка HTTP Range requests, hard delete при 404 от SE.
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
	// ErrFileArchived — файл находится в архивном SE (mode=ar), скачивание невозможно.
	ErrFileArchived = fmt.Errorf("файл находится в архивном хранилище и недоступен для скачивания")
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

	hardDeleteTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "qm_hard_delete_total",
		Help: "Количество операций hard delete (файл не найден на SE → удаление из AM + БД + кэша).",
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
//  4. Если SE вернул 404 → hard delete (удаление из AM + QM DB + инвалидация кэша)
//  5. Streaming copy в ResponseWriter с пробросом заголовков
//
// Возвращает ошибку только при невосстановимых проблемах. При 404 от SE
// выполняет hard delete и возвращает ErrNotFound.
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
		slog.String("se_mode", seInfo.Mode),
	)

	// 2.5. Проверка SE mode: архивный SE не содержит binary файлов
	if seInfo.Mode == "ar" {
		ds.logger.Info("Попытка скачивания файла из архивного SE",
			slog.String("file_id", fileID),
			slog.String("se_id", record.StorageElementID),
			slog.String("se_mode", seInfo.Mode),
		)
		downloadsTotal.WithLabelValues("archived").Inc()
		return ErrFileArchived
	}

	// 3. Запросить файл у SE (streaming)
	resp, err := ds.seClient.Download(ctx, seInfo.URL, fileID, rangeHeader)
	if err != nil {
		downloadsTotal.WithLabelValues("se_error").Inc()
		return fmt.Errorf("скачивание файла %s из SE: %w", fileID, err)
	}
	defer resp.Body.Close()

	// 4. SE вернул 404 → hard delete (AM + БД + кэш)
	if resp.StatusCode == http.StatusNotFound {
		ds.logger.Warn("Файл не найден на SE, выполняется hard delete",
			slog.String("file_id", fileID),
			slog.String("se_id", record.StorageElementID),
			slog.String("se_url", seInfo.URL),
		)
		ds.hardDeleteOnNotFound(ctx, fileID)
		downloadsTotal.WithLabelValues("hard_delete").Inc()
		return ErrNotFound
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

// hardDeleteOnNotFound выполняет hard delete файла: удаляет из AM, из локальной БД и инвалидирует кэш.
// Выполняется при 404 от SE — файл физически отсутствует на SE.
func (ds *DownloadService) hardDeleteOnNotFound(ctx context.Context, fileID string) {
	hardDeleteTotal.Inc()

	// 1. Удаляем файл через Admin Module API (hard delete)
	if err := ds.adminClient.DeleteFile(ctx, fileID); err != nil {
		ds.logger.Error("Hard delete: ошибка удаления файла через AM",
			slog.String("file_id", fileID),
			slog.String("error", err.Error()),
		)
		// Продолжаем — AM sync подхватит при следующей синхронизации
	}

	// 2. Удаляем запись из локальной БД
	if err := ds.fileRepo.Delete(ctx, fileID); err != nil {
		ds.logger.Error("Hard delete: ошибка удаления записи из БД",
			slog.String("file_id", fileID),
			slog.String("error", err.Error()),
		)
	}

	// 3. Инвалидируем кэш
	ds.cache.Delete(fileID)

	ds.logger.Info("Hard delete завершён: файл удалён",
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
