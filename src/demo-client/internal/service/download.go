package service

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/bigkaa/goartstore/demo-client/internal/activity"
	"github.com/bigkaa/goartstore/demo-client/internal/gateway"
)

// DownloadService — сервис скачивания файлов.
type DownloadService struct {
	gw     *gateway.Client
	log    *activity.Log
	logger *slog.Logger
}

// NewDownloadService создаёт новый сервис скачивания.
func NewDownloadService(gw *gateway.Client, log *activity.Log, logger *slog.Logger) *DownloadService {
	return &DownloadService{
		gw:     gw,
		log:    log,
		logger: logger,
	}
}

// ArchivedFileInfo — информация об архивном файле (для UI модалки при 410).
type ArchivedFileInfo struct {
	FileID   string
	Filename string
	FileInfo *gateway.FileInfo
}

// DownloadResult содержит либо поток данных (при успехе), либо информацию об архивном файле (при 410).
type DownloadResult struct {
	// Response — поток данных файла (при успешном скачивании).
	// Вызывающий обязан закрыть Body.
	Response *gateway.DownloadResponse

	// Archived — информация об архивном файле (если файл в SE mode=ar).
	// Заполняется при HTTP 410 вместо Response.
	Archived *ArchivedFileInfo
}

// Download скачивает файл через Gateway.
// При HTTP 410 (FILE_ARCHIVED) возвращает ArchivedFileInfo вместо ошибки,
// чтобы UI мог отобразить модалку с file_id и кнопкой копирования.
func (s *DownloadService) Download(fileID string) (*DownloadResult, error) {
	start := time.Now()

	resp, err := s.gw.Download(fileID)
	duration := time.Since(start)

	entry := activity.Entry{
		Timestamp:   time.Now(),
		Method:      "GET",
		Path:        "/query/api/v1/files/" + fileID + "/download",
		DurationMs:  duration.Milliseconds(),
		Description: fmt.Sprintf("скачивание файла: %s", fileID),
	}

	if err != nil {
		// Проверяем, является ли ошибка FILE_ARCHIVED (410).
		if errors.Is(err, gateway.ErrFileArchived) {
			entry.StatusCode = 410
			entry.Description = fmt.Sprintf("файл в архиве: %s", fileID)
			s.log.Append(entry)

			s.logger.Info("файл находится в архиве",
				"file_id", fileID,
				"duration_ms", duration.Milliseconds(),
			)

			// Пытаемся получить метаданные для отображения в UI.
			info, metaErr := s.gw.GetMetadata(fileID)
			archived := &ArchivedFileInfo{
				FileID: fileID,
			}
			if metaErr == nil && info != nil {
				archived.Filename = info.OriginalFilename
				archived.FileInfo = info
			}

			return &DownloadResult{Archived: archived}, nil
		}

		// Другие ошибки — возвращаем как есть.
		entry.StatusCode = 0
		entry.Error = err.Error()
		s.log.Append(entry)
		s.logger.Error("ошибка скачивания файла",
			"file_id", fileID,
			"error", err,
			"duration_ms", duration.Milliseconds(),
		)
		return nil, err
	}

	entry.StatusCode = 200
	if resp.Filename != "" {
		entry.Description = fmt.Sprintf("скачивание файла: %s (%s)", resp.Filename, fileID)
	}
	s.log.Append(entry)
	s.logger.Info("файл скачан успешно",
		"file_id", fileID,
		"filename", resp.Filename,
		"size", resp.ContentLength,
		"duration_ms", duration.Milliseconds(),
	)

	return &DownloadResult{Response: resp}, nil
}
