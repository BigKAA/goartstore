// Пакет service — бизнес-логика Storage Element.
// upload.go — сервис загрузки файлов с WAL-транзакциями.
package service

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	apierrors "github.com/arturkryukov/artstore/storage-element/internal/api/errors"
	"github.com/arturkryukov/artstore/storage-element/internal/api/middleware"
	"github.com/arturkryukov/artstore/storage-element/internal/config"
	"github.com/arturkryukov/artstore/storage-element/internal/domain/mode"
	"github.com/arturkryukov/artstore/storage-element/internal/domain/model"
	"github.com/arturkryukov/artstore/storage-element/internal/storage/attr"
	"github.com/arturkryukov/artstore/storage-element/internal/storage/filestore"
	"github.com/arturkryukov/artstore/storage-element/internal/storage/index"
	"github.com/arturkryukov/artstore/storage-element/internal/storage/wal"
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
	cfg       *config.Config
	walEngine *wal.WAL
	store     *filestore.FileStore
	idx       *index.Index
	sm        *mode.StateMachine
	logger    *slog.Logger
}

// NewUploadService создаёт сервис загрузки файлов.
func NewUploadService(
	cfg *config.Config,
	walEngine *wal.WAL,
	store *filestore.FileStore,
	idx *index.Index,
	sm *mode.StateMachine,
	logger *slog.Logger,
) *UploadService {
	return &UploadService{
		cfg:       cfg,
		walEngine: walEngine,
		store:     store,
		idx:       idx,
		sm:        sm,
		logger:    logger.With(slog.String("component", "upload_service")),
	}
}

// Upload загружает файл в хранилище с WAL-транзакцией.
//
// Поток:
//  1. Проверка mode (edit/rw)
//  2. Проверка размера файла
//  3. WAL StartTransaction
//  4. SaveFile (streaming + SHA-256)
//  5. WriteAttrFile
//  6. index.Add
//  7. WAL Commit
//
// При ошибке — cleanup (удаление файла, attr.json) + WAL Rollback.
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

	// 3. Генерируем file_id
	fileID := uuid.New().String()

	// 4. WAL StartTransaction
	walEntry, err := s.walEngine.StartTransaction(wal.OpFileCreate, fileID)
	if err != nil {
		s.logger.Error("Ошибка создания WAL-транзакции", slog.String("error", err.Error()))
		return nil, &UploadError{
			StatusCode: 500,
			Code:       apierrors.CodeInternalError,
			Message:    "Внутренняя ошибка при создании транзакции",
		}
	}

	// Cleanup при ошибке
	var savedResult *filestore.SaveResult
	rollback := func() {
		// Удаляем файл если был сохранён
		if savedResult != nil {
			_ = s.store.DeleteFile(savedResult.StoragePath)
			// Удаляем attr.json
			attrPath := attr.AttrFilePath(s.store.FullPath(savedResult.StoragePath))
			_ = attr.Delete(attrPath)
		}
		// Откатываем WAL
		if rbErr := s.walEngine.Rollback(walEntry.TransactionID); rbErr != nil {
			s.logger.Error("Ошибка отката WAL",
				slog.String("tx_id", walEntry.TransactionID),
				slog.String("error", rbErr.Error()),
			)
		}
	}

	// 5. SaveFile (streaming + SHA-256)
	savedResult, err = s.store.SaveFile(params.Reader, params.OriginalFilename, params.UploadedBy)
	if err != nil {
		rollback()
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

	// 6. Определяем retention policy из режима
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

	// 7. Парсим теги
	var tags []string
	if params.TagsJSON != "" {
		if err := json.Unmarshal([]byte(params.TagsJSON), &tags); err != nil {
			rollback()
			return nil, &UploadError{
				StatusCode: 400,
				Code:       apierrors.CodeValidationError,
				Message:    fmt.Sprintf("Некорректный формат тегов: %s", err.Error()),
			}
		}
	}

	// 8. Формируем метаданные
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
		TtlDays:          ttlDays,
		ExpiresAt:        expiresAt,
		Tags:             tags,
		Description:      params.Description,
	}

	// 9. Записываем attr.json
	attrPath := attr.AttrFilePath(s.store.FullPath(savedResult.StoragePath))
	if err := attr.Write(attrPath, metadata); err != nil {
		rollback()
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

	// 10. Добавляем в индекс
	s.idx.Add(metadata)

	// 11. WAL Commit
	if err := s.walEngine.Commit(walEntry.TransactionID); err != nil {
		s.logger.Error("Ошибка коммита WAL (данные сохранены)",
			slog.String("tx_id", walEntry.TransactionID),
			slog.String("file_id", fileID),
			slog.String("error", err.Error()),
		)
		// Данные уже записаны, коммит WAL — best effort
	}

	// 12. Обновляем метрики
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

	return &UploadResult{Metadata: metadata}, nil
}

// detectContentType определяет Content-Type из заголовка multipart part.
// Если не указан — используется application/octet-stream.
func detectContentType(contentType string) string {
	if contentType == "" {
		return "application/octet-stream"
	}
	// Убираем параметры (charset и т.д.)
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = strings.TrimSpace(contentType[:idx])
	}
	return contentType
}
