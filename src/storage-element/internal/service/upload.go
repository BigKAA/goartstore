// Пакет service — бизнес-логика Storage Element.
// upload.go — сервис загрузки файлов.
//
// Stateless архитектура: per-file lock с TTL защищает файл
// от преждевременного GC/Reconcile/Delete во время upload-а.
package service

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"

	apierrors "github.com/bigkaa/goartstore/storage-element/internal/api/errors"
	"github.com/bigkaa/goartstore/storage-element/internal/api/middleware"
	"github.com/bigkaa/goartstore/storage-element/internal/config"
	"github.com/bigkaa/goartstore/storage-element/internal/domain/mode"
	"github.com/bigkaa/goartstore/storage-element/internal/domain/model"
	"github.com/bigkaa/goartstore/storage-element/internal/lockfile"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/attr"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/filestore"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/index"
)

// UploadParams — параметры загрузки файла.
type UploadParams struct {
	// Reader — поток данных файла
	Reader io.Reader
	// OriginalFilename — оригинальное имя файла
	OriginalFilename string
	// ContentType — MIME-тип файла
	ContentType string
	// Size — размер файла (из Content-Length multipart part)
	Size int64
	// UploadedBy — идентификатор пользователя (sub из JWT)
	UploadedBy string
	// Description — описание файла (опционально)
	Description string
	// Tags — теги файла (опционально, JSON-строка)
	TagsJSON string
}

// UploadResult — результат загрузки файла.
type UploadResult struct {
	Metadata *model.FileMetadata
}

// UploadError — ошибка загрузки с HTTP-кодом.
type UploadError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *UploadError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// UploadService — сервис загрузки файлов.
type UploadService struct {
	cfg         *config.Config
	store       *filestore.FileStore
	idx         *index.Index
	sm          *mode.StateMachine
	lockManager *lockfile.LockManager
	logger      *slog.Logger
}

// NewUploadService создаёт сервис загрузки файлов.
func NewUploadService(
	cfg *config.Config,
	store *filestore.FileStore,
	idx *index.Index,
	sm *mode.StateMachine,
	lockManager *lockfile.LockManager,
	logger *slog.Logger,
) *UploadService {
	return &UploadService{
		cfg:         cfg,
		store:       store,
		idx:         idx,
		sm:          sm,
		lockManager: lockManager,
		logger:      logger.With(slog.String("component", "upload_service")),
	}
}

// Upload загружает файл в хранилище.
//
// Pipeline (stateless, per-file lock):
//  1. Проверка mode (edit/rw)
//  2. Проверка размера файла и capacity
//  3. lockManager.Acquire(fileID)
//  4. SaveFile (streaming + SHA-256)
//  5. WriteAttrFile (point of no return)
//  6. index.Add
//  7. lockManager.Release(fileID) — в defer
//
// При ошибке — release lock + cleanup (удаление файла, attr.json).
func (s *UploadService) Upload(params UploadParams) (*UploadResult, *UploadError) {
	// 1. Проверяем допустимость операции upload в текущем режиме
	if !s.sm.CanPerform(mode.OpUpload) {
		return nil, &UploadError{
			StatusCode: 409,
			Code:       apierrors.CodeModeNotAllowed,
			Message:    fmt.Sprintf("Загрузка файлов недоступна в режиме %s", s.sm.CurrentMode()),
		}
	}

	// 2. Проверяем размер файла
	if params.Size > s.cfg.MaxFileSize {
		return nil, &UploadError{
			StatusCode: 413,
			Code:       apierrors.CodeFileTooLarge,
			Message:    fmt.Sprintf("Размер файла %d байт превышает максимум %d байт", params.Size, s.cfg.MaxFileSize),
		}
	}

	// 2.1. Проверяем наличие свободного места в пределах сконфигурированного лимита.
	// Примечание: проверка racy (между check и Add другой upload может занять место),
	// но это приемлемо — pre-check отсекает 99% случаев.
	if params.Size > 0 && s.idx.TotalActiveSize()+params.Size > s.cfg.MaxCapacity {
		return nil, &UploadError{
			StatusCode: 507,
			Code:       apierrors.CodeStorageFull,
			Message: fmt.Sprintf("Недостаточно места: требуется %d байт, доступно %d байт",
				params.Size, s.cfg.MaxCapacity-s.idx.TotalActiveSize()),
		}
	}

	// 3. Генерируем file_id и захватываем lock
	fileID := uuid.New().String()

	if err := s.lockManager.Acquire(fileID); err != nil {
		s.logger.Error("Ошибка захвата lock для файла",
			slog.String("file_id", fileID),
			slog.String("error", err.Error()),
		)
		return nil, &UploadError{
			StatusCode: 500,
			Code:       apierrors.CodeInternalError,
			Message:    "Ошибка захвата lock для файла",
		}
	}

	// defer release lock — гарантированная очистка при любом исходе
	defer func() {
		if relErr := s.lockManager.Release(fileID); relErr != nil {
			s.logger.Warn("Ошибка освобождения lock (TTL подстрахует)",
				slog.String("file_id", fileID),
				slog.String("error", relErr.Error()),
			)
		}
	}()

	// Cleanup при ошибке (удаление частично записанных файлов)
	var savedResult *filestore.SaveResult
	cleanup := func() {
		if savedResult != nil {
			_ = s.store.DeleteFile(savedResult.StoragePath)
			attrPath := attr.AttrFilePath(s.store.FullPath(savedResult.StoragePath))
			_ = attr.Delete(attrPath)
		}
	}

	// 4. SaveFile (streaming + SHA-256)
	var err error
	savedResult, err = s.store.SaveFile(params.Reader, params.OriginalFilename, params.UploadedBy)
	if err != nil {
		cleanup()
		s.logger.Error("Ошибка сохранения файла",
			slog.String("file_id", fileID),
			slog.String("error", err.Error()),
		)
		return nil, &UploadError{
			StatusCode: 500,
			Code:       apierrors.CodeInternalError,
			Message:    "Ошибка сохранения файла на диск",
		}
	}

	// 5. Определяем retention policy из режима
	retentionPolicy := model.RetentionPermanent
	var ttlDays *int
	var expiresAt *time.Time
	if s.sm.CurrentMode() == mode.ModeEdit {
		retentionPolicy = model.RetentionTemporary
		defaultTTL := 30
		ttlDays = &defaultTTL
		exp := time.Now().UTC().AddDate(0, 0, defaultTTL)
		expiresAt = &exp
	}

	// 6. Парсим теги
	var tags []string
	if params.TagsJSON != "" {
		if err := json.Unmarshal([]byte(params.TagsJSON), &tags); err != nil {
			cleanup()
			return nil, &UploadError{
				StatusCode: 400,
				Code:       apierrors.CodeValidationError,
				Message:    fmt.Sprintf("Некорректный формат тегов: %s", err.Error()),
			}
		}
	}

	// 7. Формируем метаданные
	now := time.Now().UTC()
	metadata := &model.FileMetadata{
		FileID:           fileID,
		OriginalFilename: params.OriginalFilename,
		StoragePath:      savedResult.StoragePath,
		ContentType:      params.ContentType,
		Size:             savedResult.Size,
		Checksum:         savedResult.Checksum,
		UploadedBy:       params.UploadedBy,
		UploadedAt:       now,
		Status:           model.StatusActive,
		RetentionPolicy:  retentionPolicy,
		TTLDays:          ttlDays,
		ExpiresAt:        expiresAt,
		Tags:             tags,
		Description:      params.Description,
	}

	// 8. Записываем attr.json (point of no return)
	attrPath := attr.AttrFilePath(s.store.FullPath(savedResult.StoragePath))
	if err := attr.Write(attrPath, metadata); err != nil {
		cleanup()
		s.logger.Error("Ошибка записи attr.json",
			slog.String("file_id", fileID),
			slog.String("error", err.Error()),
		)
		return nil, &UploadError{
			StatusCode: 500,
			Code:       apierrors.CodeInternalError,
			Message:    "Ошибка записи метаданных",
		}
	}

	// 9. Добавляем в индекс
	s.idx.Add(metadata)

	// 10. Обновляем метрики
	middleware.OperationsTotal.WithLabelValues("upload", "success").Inc()
	middleware.FilesTotal.WithLabelValues(string(model.StatusActive)).Inc()

	s.logger.Info("Файл загружен",
		slog.String("file_id", fileID),
		slog.String("filename", params.OriginalFilename),
		slog.Int64("size", savedResult.Size),
		slog.String("checksum", savedResult.Checksum),
		slog.String("uploaded_by", params.UploadedBy),
		slog.String("retention", string(retentionPolicy)),
	)

	// Lock освобождается через defer
	return &UploadResult{Metadata: metadata}, nil
}
