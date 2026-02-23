// download.go — сервис скачивания файлов.
package service

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	apierrors "github.com/bigkaa/goartstore/storage-element/internal/api/errors"
	"github.com/bigkaa/goartstore/storage-element/internal/api/middleware"
	"github.com/bigkaa/goartstore/storage-element/internal/domain/mode"
	"github.com/bigkaa/goartstore/storage-element/internal/domain/model"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/filestore"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/index"
)

// DownloadService — сервис скачивания файлов.
type DownloadService struct {
	store  *filestore.FileStore
	idx    *index.Index
	sm     *mode.StateMachine
	logger *slog.Logger
}

// NewDownloadService создаёт сервис скачивания файлов.
func NewDownloadService(
	store *filestore.FileStore,
	idx *index.Index,
	sm *mode.StateMachine,
	logger *slog.Logger,
) *DownloadService {
	return &DownloadService{
		store:  store,
		idx:    idx,
		sm:     sm,
		logger: logger.With(slog.String("component", "download_service")),
	}
}

// DownloadError — ошибка скачивания с HTTP-кодом.
type DownloadError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *DownloadError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Serve отдаёт файл клиенту через http.ServeContent.
// Поддерживает Range requests (206 Partial Content) и ETag (If-None-Match).
// Параметры:
//   - w, r: HTTP writer и request
//   - fileID: идентификатор файла
func (s *DownloadService) Serve(w http.ResponseWriter, r *http.Request, fileID string) *DownloadError {
	// 1. Проверяем допустимость download в текущем режиме
	if !s.sm.CanPerform(mode.OpDownload) {
		return &DownloadError{
			StatusCode: 409,
			Code:       apierrors.CodeModeNotAllowed,
			Message:    fmt.Sprintf("Скачивание файлов недоступно в режиме %s", s.sm.CurrentMode()),
		}
	}

	// 2. Ищем метаданные в индексе
	meta := s.idx.Get(fileID)
	if meta == nil {
		return &DownloadError{
			StatusCode: 404,
			Code:       apierrors.CodeNotFound,
			Message:    fmt.Sprintf("Файл %s не найден", fileID),
		}
	}

	// 3. Проверяем статус
	if meta.Status != model.StatusActive {
		return &DownloadError{
			StatusCode: 409,
			Code:       apierrors.CodeModeNotAllowed,
			Message:    fmt.Sprintf("Файл %s имеет статус %s, скачивание недоступно", fileID, meta.Status),
		}
	}

	// 4. Открываем файл
	file, err := s.store.ReadFile(meta.StoragePath)
	if err != nil {
		s.logger.Error("Файл не найден на диске",
			slog.String("file_id", fileID),
			slog.String("storage_path", meta.StoragePath),
			slog.String("error", err.Error()),
		)
		return &DownloadError{
			StatusCode: 404,
			Code:       apierrors.CodeNotFound,
			Message:    fmt.Sprintf("Файл %s не найден на диске", fileID),
		}
	}
	defer file.Close()

	// 5. Получаем информацию о файле для http.ServeContent
	stat, err := file.Stat()
	if err != nil {
		s.logger.Error("Ошибка получения stat файла",
			slog.String("file_id", fileID),
			slog.String("error", err.Error()),
		)
		return &DownloadError{
			StatusCode: 500,
			Code:       apierrors.CodeInternalError,
			Message:    "Ошибка чтения файла",
		}
	}

	// 6. Устанавливаем заголовки
	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", meta.OriginalFilename))
	w.Header().Set("ETag", fmt.Sprintf("\"%s\"", meta.Checksum))
	w.Header().Set("Accept-Ranges", "bytes")

	// 7. http.ServeContent автоматически обрабатывает:
	//    - Range requests (206 Partial Content)
	//    - If-None-Match (304 Not Modified через ETag)
	//    - If-Modified-Since
	//    - Content-Length
	http.ServeContent(w, r, meta.OriginalFilename, stat.ModTime(), file)

	// 8. Метрики
	middleware.OperationsTotal.WithLabelValues("download", "success").Inc()

	s.logger.Debug("Файл скачан",
		slog.String("file_id", fileID),
		slog.String("filename", meta.OriginalFilename),
		slog.Int64("size", meta.Size),
	)

	return nil
}

// GetFileForServing возвращает файл и метаданные для отдачи.
// Используется когда нужен контроль над отдачей (не через Serve).
func (s *DownloadService) GetFileForServing(fileID string) (*os.File, *model.FileMetadata, *DownloadError) {
	// Проверяем допустимость
	if !s.sm.CanPerform(mode.OpDownload) {
		return nil, nil, &DownloadError{
			StatusCode: 409,
			Code:       apierrors.CodeModeNotAllowed,
			Message:    fmt.Sprintf("Скачивание файлов недоступно в режиме %s", s.sm.CurrentMode()),
		}
	}

	meta := s.idx.Get(fileID)
	if meta == nil {
		return nil, nil, &DownloadError{
			StatusCode: 404,
			Code:       apierrors.CodeNotFound,
			Message:    fmt.Sprintf("Файл %s не найден", fileID),
		}
	}

	if meta.Status != model.StatusActive {
		return nil, nil, &DownloadError{
			StatusCode: 409,
			Code:       apierrors.CodeModeNotAllowed,
			Message:    fmt.Sprintf("Файл %s имеет статус %s", fileID, meta.Status),
		}
	}

	file, err := s.store.ReadFile(meta.StoragePath)
	if err != nil {
		return nil, nil, &DownloadError{
			StatusCode: 404,
			Code:       apierrors.CodeNotFound,
			Message:    fmt.Sprintf("Файл %s не найден на диске", fileID),
		}
	}

	return file, meta, nil
}
