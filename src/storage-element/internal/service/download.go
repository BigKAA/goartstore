// download.go — сервис скачивания файлов.
package service

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"

	apierrors "github.com/bigkaa/goartstore/storage-element/internal/api/errors"
	"github.com/bigkaa/goartstore/storage-element/internal/api/middleware"
	"github.com/bigkaa/goartstore/storage-element/internal/backend"
	"github.com/bigkaa/goartstore/storage-element/internal/domain/mode"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/index"
)

// DownloadService — сервис скачивания файлов.
type DownloadService struct {
	files  backend.FileStore
	idx    *index.Index
	sm     *mode.StateMachine
	logger *slog.Logger
}

// NewDownloadService создаёт сервис скачивания файлов.
func NewDownloadService(
	files backend.FileStore,
	idx *index.Index,
	sm *mode.StateMachine,
	logger *slog.Logger,
) *DownloadService {
	return &DownloadService{
		files:  files,
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

// Serve отдаёт файл клиенту.
// Если backend возвращает io.ReadSeeker — используется http.ServeContent
// (поддержка Range requests, 206 Partial Content, ETag).
// Иначе — streaming через io.Copy.
//
// Параметры:
//   - w, r: HTTP writer и request
//   - fileID: идентификатор файла
func (s *DownloadService) Serve(w http.ResponseWriter, r *http.Request, fileID string) *DownloadError {
	ctx := r.Context()

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

	// 3. Открываем файл
	rc, err := s.files.ReadFile(ctx, meta.StoragePath)
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
	defer rc.Close()

	// 5. Устанавливаем заголовки
	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", meta.OriginalFilename))
	w.Header().Set("ETag", fmt.Sprintf("%q", meta.Checksum))
	w.Header().Set("Accept-Ranges", "bytes")

	// 6. Если backend поддерживает Seek — используем http.ServeContent.
	// Для LocalFS (*os.File) это всегда true.
	// Для S3 (GetObject Body) — нет, используем streaming.
	if rs, ok := rc.(io.ReadSeeker); ok {
		// Используем UploadedAt как ModTime (вместо file.Stat().ModTime())
		http.ServeContent(w, r, meta.OriginalFilename, meta.UploadedAt, rs)
	} else {
		// Streaming fallback для backend-ов без Seek (S3)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", meta.Size))
		w.WriteHeader(http.StatusOK)
		if _, copyErr := io.Copy(w, rc); copyErr != nil {
			s.logger.Error("Ошибка streaming файла",
				slog.String("file_id", fileID),
				slog.String("error", copyErr.Error()),
			)
		}
	}

	// 7. Метрики
	middleware.OperationsTotal.WithLabelValues("download", "success").Inc()

	s.logger.Debug("Файл скачан",
		slog.String("file_id", fileID),
		slog.String("filename", meta.OriginalFilename),
		slog.Int64("size", meta.Size),
	)

	return nil
}
