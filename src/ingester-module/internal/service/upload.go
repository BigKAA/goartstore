// upload.go — сервис upload pipeline: валидация → temp-file → SE selection → SE upload → AM register.
// Координирует весь процесс загрузки файла с retry при 507 (Storage Full).
//
// Pipeline (9 шагов):
//  1. Валидация: retention_policy, ttl_days
//  2. Сохранение во временный файл (emptyDir), defer cleanup
//  3. Проверка fileSize ≤ MaxFileSize
//  4. Выбор SE через Selector (Sequential Fill)
//  5. Seek(0,0) temp-file
//  6. SE upload (multipart streaming)
//  7. При 507: exclude SE, Seek(0,0), retry с п.4 (до MaxRetries)
//  8. Регистрация файла в AM
//  9. Формирование UploadResponse
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/bigkaa/goartstore/ingester-module/internal/adminclient"
	"github.com/bigkaa/goartstore/ingester-module/internal/seclient"
)

// Константы для retention_policy.
const (
	RetentionTemporary = "temporary"
	RetentionPermanent = "permanent"
)

// Ошибки upload service.
var (
	// ErrFileTooLarge — файл превышает MaxFileSize.
	ErrFileTooLarge = fmt.Errorf("файл превышает максимально допустимый размер")

	// ErrStorageFull — все SE исчерпаны (все retry использованы).
	ErrStorageFull = fmt.Errorf("нет свободного места на всех доступных SE")

	// ErrSEUploadFailed — ошибка загрузки в SE (не 507 и не 413).
	ErrSEUploadFailed = fmt.Errorf("ошибка загрузки файла в Storage Element")

	// ErrAMUnavailable — Admin Module недоступен при регистрации файла.
	ErrAMUnavailable = fmt.Errorf("admin module недоступен при регистрации файла") //nolint:stylecheck // русское описание ошибки

	// ErrValidation — ошибка валидации входных параметров.
	ErrValidation = fmt.Errorf("ошибка валидации параметров upload")
)

// Prometheus-метрики upload.
var (
	uploadsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "im_uploads_total",
		Help: "Общее количество операций upload (по политике хранения и статусу).",
	}, []string{"retention_policy", "status"})

	uploadDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "im_upload_duration_seconds",
			Help:    "Длительность полного upload pipeline (от получения запроса до ответа).",
			Buckets: []float64{0.5, 1, 5, 10, 30, 60, 120, 300, 600},
		}, []string{"retention_policy"})

	uploadSizeBytes = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "im_upload_size_bytes",
			Help:    "Размер загружаемых файлов (в байтах).",
			Buckets: []float64{1024, 10240, 102400, 1048576, 10485760, 104857600, 536870912, 1073741824},
		}, []string{"retention_policy"})

	seUploadDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "im_se_upload_duration_seconds",
		Help:    "Длительность передачи файла в Storage Element.",
		Buckets: []float64{0.5, 1, 5, 10, 30, 60, 120, 300, 600},
	})

	activeUploads = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "im_active_uploads",
		Help: "Количество активных (in-progress) upload операций.",
	})

	retryTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "im_retry_total",
		Help: "Количество retry операций (по причине).",
	}, []string{"reason"})
)

// UploadParams — параметры загрузки, извлечённые из HTTP-запроса.
type UploadParams struct {
	// UploadedBy — sub из JWT конечного пользователя (не SA)
	UploadedBy string
	// Description — описание файла (опционально)
	Description *string
	// Tags — теги файла (опционально)
	Tags []string
	// RetentionPolicy — "temporary" или "permanent"
	RetentionPolicy string
	// TTLDays — срок хранения в днях (1-365, обязателен для temporary)
	TTLDays *int
}

// UploadResponse — результат загрузки файла (для отправки клиенту).
type UploadResponse struct {
	FileID           string
	OriginalFilename string
	ContentType      string
	Size             int64
	Checksum         string
	UploadedBy       string
	UploadedAt       time.Time
	Description      *string
	Tags             []string
	Status           string
	RetentionPolicy  string
	TTLDays          *int
	ExpiresAt        *time.Time
	StorageElementID string
}

// UploadService — координатор upload pipeline.
type UploadService struct {
	selector    *SelectorService
	adminClient *adminclient.Client
	seClient    *seclient.Client
	maxFileSize int64
	maxRetries  int
	logger      *slog.Logger
}

// NewUploadService создаёт upload service.
func NewUploadService(
	selector *SelectorService,
	adminClient *adminclient.Client,
	seClient *seclient.Client,
	maxFileSize int64,
	maxRetries int,
	logger *slog.Logger,
) *UploadService {
	return &UploadService{
		selector:    selector,
		adminClient: adminClient,
		seClient:    seClient,
		maxFileSize: maxFileSize,
		maxRetries:  maxRetries,
		logger:      logger.With(slog.String("component", "upload_service")),
	}
}

// Upload выполняет полный upload pipeline.
//
// Pipeline:
//  1. Валидация retention_policy и ttl_days
//  2. Сохранение файла во временный файл
//  3. Проверка размера файла
//  4. Выбор SE (Sequential Fill) с retry при 507
//  5. Загрузка файла в SE
//  6. Регистрация файла в Admin Module
//  7. Формирование UploadResponse
func (s *UploadService) Upload(
	ctx context.Context,
	file multipart.File,
	header *multipart.FileHeader,
	params UploadParams,
) (*UploadResponse, error) {
	start := time.Now()
	activeUploads.Inc()
	defer activeUploads.Dec()

	// 1. Валидация
	if err := s.validateParams(params); err != nil {
		uploadsTotal.WithLabelValues(params.RetentionPolicy, "error").Inc()
		return nil, err
	}

	// 2. Сохраняем файл во временный файл (emptyDir /tmp/uploads или системный temp)
	tempFile, err := s.saveTempFile(file)
	if err != nil {
		uploadsTotal.WithLabelValues(params.RetentionPolicy, "error").Inc()
		return nil, fmt.Errorf("сохранение temp-файла: %w", err)
	}
	defer s.cleanupTempFile(tempFile)

	// 3. Проверяем размер файла (по stat, не по header.Size — может быть неточным)
	stat, err := tempFile.Stat()
	if err != nil {
		uploadsTotal.WithLabelValues(params.RetentionPolicy, "error").Inc()
		return nil, fmt.Errorf("получение размера temp-файла: %w", err)
	}
	fileSize := stat.Size()

	if fileSize > s.maxFileSize {
		uploadsTotal.WithLabelValues(params.RetentionPolicy, "error").Inc()
		return nil, fmt.Errorf("%w: %d байт (лимит %d байт)", ErrFileTooLarge, fileSize, s.maxFileSize)
	}

	uploadSizeBytes.WithLabelValues(params.RetentionPolicy).Observe(float64(fileSize))

	// 4-7. Upload в SE с retry при 507
	uploadResult, selectedSE, err := s.uploadToSEWithRetry(ctx, tempFile, header.Filename, params)
	if err != nil {
		uploadsTotal.WithLabelValues(params.RetentionPolicy, "error").Inc()
		return nil, err
	}

	// 8. Регистрация файла в Admin Module
	fileRecord, err := s.registerFile(ctx, uploadResult, selectedSE, params)
	if err != nil {
		// Файл остаётся на SE как сирота — будет удалён GC
		s.logger.Warn("Файл загружен в SE, но не зарегистрирован в AM (сирота)",
			slog.String("file_id", uploadResult.FileID),
			slog.String("se_id", selectedSE.ID),
			slog.String("se_url", selectedSE.URL),
			slog.String("error", err.Error()),
		)
		uploadsTotal.WithLabelValues(params.RetentionPolicy, "error").Inc()
		return nil, fmt.Errorf("%w: %s", ErrAMUnavailable, err.Error())
	}

	// 9. Формируем UploadResponse
	resp := buildUploadResponse(fileRecord, selectedSE)

	// Метрики успеха
	duration := time.Since(start)
	uploadsTotal.WithLabelValues(params.RetentionPolicy, "success").Inc()
	uploadDuration.WithLabelValues(params.RetentionPolicy).Observe(duration.Seconds())

	s.logger.Info("Upload завершён успешно",
		slog.String("file_id", uploadResult.FileID),
		slog.String("se_id", selectedSE.ID),
		slog.Int64("size", fileSize),
		slog.Duration("duration", duration),
		slog.String("retention_policy", params.RetentionPolicy),
	)

	return resp, nil
}

// validateParams валидирует параметры upload.
func (s *UploadService) validateParams(params UploadParams) error {
	// Проверка retention_policy
	if params.RetentionPolicy != RetentionTemporary && params.RetentionPolicy != RetentionPermanent {
		return fmt.Errorf("%w: retention_policy должен быть 'temporary' или 'permanent'", ErrValidation)
	}

	// Проверка ttl_days для temporary
	if params.RetentionPolicy == RetentionTemporary {
		if params.TTLDays == nil {
			return fmt.Errorf("%w: ttl_days обязателен для retention_policy=temporary", ErrValidation)
		}
		if *params.TTLDays < 1 || *params.TTLDays > 365 {
			return fmt.Errorf("%w: ttl_days должен быть в диапазоне 1-365 (получено: %d)", ErrValidation, *params.TTLDays)
		}
	}

	// ttl_days не должен быть задан для permanent
	if params.RetentionPolicy == RetentionPermanent && params.TTLDays != nil {
		return fmt.Errorf("%w: ttl_days не применим для retention_policy=permanent", ErrValidation)
	}

	return nil
}

// saveTempFile сохраняет multipart файл во временный файл.
// Возвращает *os.File, позиция — конец файла (требуется Seek(0,0) перед чтением).
func (s *UploadService) saveTempFile(file multipart.File) (*os.File, error) {
	// Пробуем /tmp/uploads (emptyDir volume в K8s), fallback на системный temp
	tempDir := "/tmp/uploads"
	if _, statErr := os.Stat(tempDir); os.IsNotExist(statErr) {
		tempDir = ""
	}

	tempFile, err := os.CreateTemp(tempDir, "im-upload-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("создание temp-файла: %w", err)
	}

	if _, copyErr := io.Copy(tempFile, file); copyErr != nil {
		s.cleanupTempFile(tempFile)
		return nil, fmt.Errorf("копирование в temp-файл: %w", copyErr)
	}

	return tempFile, nil
}

// cleanupTempFile закрывает и удаляет временный файл.
func (s *UploadService) cleanupTempFile(f *os.File) {
	name := f.Name()
	if closeErr := f.Close(); closeErr != nil {
		s.logger.Debug("Ошибка закрытия temp-файла", slog.String("error", closeErr.Error()))
	}
	if removeErr := os.Remove(name); removeErr != nil && !os.IsNotExist(removeErr) { //nolint:gosec // G703: путь из os.CreateTemp, контролируем
		s.logger.Debug("Ошибка удаления temp-файла", slog.String("error", removeErr.Error()))
	}
}

// uploadToSEWithRetry выполняет загрузку в SE с retry при 507.
// При каждом 507 исключает SE из выбора и пробует следующий.
func (s *UploadService) uploadToSEWithRetry(
	ctx context.Context,
	tempFile *os.File,
	filename string,
	params UploadParams,
) (*seclient.UploadResult, *adminclient.SEInfo, error) {
	var excludedSEIDs []string
	stat, _ := tempFile.Stat()
	fileSize := stat.Size()

	// Content-type определяется SE автоматически
	contentType := "application/octet-stream"

	for attempt := 0; attempt <= s.maxRetries; attempt++ {
		// 4. Выбор SE
		selectedSE, selectErr := s.selector.SelectSE(ctx, fileSize, params.RetentionPolicy, excludedSEIDs)
		if selectErr != nil {
			if errors.Is(selectErr, ErrNoStorageAvailable) {
				return nil, nil, ErrNoStorageAvailable
			}
			return nil, nil, fmt.Errorf("выбор SE: %w", selectErr)
		}

		// 5. Seek(0,0) — перемотать на начало (после предыдущей попытки или записи)
		if _, seekErr := tempFile.Seek(0, io.SeekStart); seekErr != nil {
			return nil, nil, fmt.Errorf("перемотка temp-файла: %w", seekErr)
		}

		// 6. Загрузка в SE
		seUploadStart := time.Now()
		description := ""
		if params.Description != nil {
			description = *params.Description
		}

		result, uploadErr := s.seClient.Upload(
			ctx, selectedSE.URL, tempFile, filename, contentType,
			description, params.Tags,
		)
		seUploadDuration.Observe(time.Since(seUploadStart).Seconds())

		// 7. Обработка ошибок
		if uploadErr != nil {
			if errors.Is(uploadErr, seclient.ErrStorageFull) {
				// 507 — исключаем SE, retry
				s.logger.Warn("SE вернул 507, retry с другим SE",
					slog.String("se_id", selectedSE.ID),
					slog.String("se_url", selectedSE.URL),
					slog.Int("attempt", attempt+1),
					slog.Int("max_retries", s.maxRetries),
				)
				excludedSEIDs = append(excludedSEIDs, selectedSE.ID)
				retryTotal.WithLabelValues("507").Inc()
				continue
			}

			if errors.Is(uploadErr, seclient.ErrFileTooLarge) {
				// 413 — SE имеет своё ограничение, пробрасываем
				return nil, nil, ErrFileTooLarge
			}

			// Другие ошибки — пробрасываем как SE_UPLOAD_FAILED
			return nil, nil, fmt.Errorf("%w: %s", ErrSEUploadFailed, uploadErr.Error())
		}

		// Успех
		return result, selectedSE, nil
	}

	// Все retry исчерпаны
	return nil, nil, ErrStorageFull
}

// registerFile регистрирует файл в Admin Module.
func (s *UploadService) registerFile(
	ctx context.Context,
	uploadResult *seclient.UploadResult,
	selectedSE *adminclient.SEInfo,
	params UploadParams,
) (*adminclient.FileRecord, error) {
	req := adminclient.FileRegisterRequest{
		FileID:           uploadResult.FileID,
		OriginalFilename: uploadResult.OriginalFilename,
		ContentType:      uploadResult.ContentType,
		Size:             uploadResult.Size,
		Checksum:         uploadResult.Checksum,
		StorageElementID: selectedSE.ID,
		RetentionPolicy:  params.RetentionPolicy,
		UploadedBy:       params.UploadedBy,
		Description:      params.Description,
		Tags:             params.Tags,
		TTLDays:          params.TTLDays,
	}

	return s.adminClient.RegisterFile(ctx, req)
}

// buildUploadResponse собирает UploadResponse из данных AM FileRecord и выбранного SE.
func buildUploadResponse(
	fileRecord *adminclient.FileRecord,
	selectedSE *adminclient.SEInfo,
) *UploadResponse {
	return &UploadResponse{
		FileID:           fileRecord.FileID,
		OriginalFilename: fileRecord.OriginalFilename,
		ContentType:      fileRecord.ContentType,
		Size:             fileRecord.Size,
		Checksum:         fileRecord.Checksum,
		UploadedBy:       fileRecord.UploadedBy,
		UploadedAt:       fileRecord.UploadedAt,
		Description:      fileRecord.Description,
		Tags:             fileRecord.Tags,
		Status:           fileRecord.Status,
		RetentionPolicy:  fileRecord.RetentionPolicy,
		TTLDays:          fileRecord.TTLDays,
		ExpiresAt:        fileRecord.ExpiresAt,
		StorageElementID: selectedSE.ID,
	}
}
