// Пакет service реализует бизнес-логику Demo Client —
// тонкую прослойку над Gateway Client с записью операций в Activity Log.
package service

import (
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/bigkaa/goartstore/demo-client/internal/activity"
	"github.com/bigkaa/goartstore/demo-client/internal/gateway"
)

// UploadService — сервис загрузки файлов (single + batch).
type UploadService struct {
	gw     *gateway.Client
	log    *activity.Log
	logger *slog.Logger
}

// NewUploadService создаёт новый сервис загрузки.
func NewUploadService(gw *gateway.Client, log *activity.Log, logger *slog.Logger) *UploadService {
	return &UploadService{
		gw:     gw,
		log:    log,
		logger: logger,
	}
}

// Upload загружает один файл через Gateway.
func (s *UploadService) Upload(filename string, reader io.Reader, fileSize int64, params gateway.UploadParams) (*gateway.UploadResult, error) {
	start := time.Now()

	result, err := s.gw.Upload(filename, reader, fileSize, params)
	duration := time.Since(start)

	// Записываем операцию в Activity Log.
	entry := activity.Entry{
		Timestamp:   time.Now(),
		Method:      "POST",
		Path:        "/upload/api/v1/files/upload",
		DurationMs:  duration.Milliseconds(),
		Description: fmt.Sprintf("загрузка файла: %s", filename),
	}

	if err != nil {
		entry.StatusCode = 0
		entry.Error = err.Error()
		s.log.Append(entry)
		s.logger.Error("ошибка загрузки файла",
			"filename", filename,
			"error", err,
			"duration_ms", duration.Milliseconds(),
		)
		return nil, err
	}

	entry.StatusCode = 201
	s.log.Append(entry)
	s.logger.Info("файл загружен успешно",
		"filename", filename,
		"file_id", result.FileID,
		"size", result.Size,
		"retention", result.RetentionPolicy,
		"duration_ms", duration.Milliseconds(),
	)
	return result, nil
}

// BatchUploadResult — результат загрузки одного файла в batch-операции.
type BatchUploadResult struct {
	Filename string
	Result   *gateway.UploadResult
	Error    error
}

// BatchUpload загружает несколько файлов последовательно.
// Для каждого файла вызывает callback с промежуточным результатом.
// Возвращает сводку: успешно/ошибки/всего.
func (s *UploadService) BatchUpload(
	files []BatchUploadFile,
	params gateway.UploadParams,
	onProgress func(current int, total int, result BatchUploadResult),
) BatchUploadSummary {
	summary := BatchUploadSummary{
		Total:   len(files),
		Results: make([]BatchUploadResult, 0, len(files)),
	}

	for i, f := range files {
		result, err := s.Upload(f.Filename, f.Reader, f.Size, params)

		batchResult := BatchUploadResult{
			Filename: f.Filename,
			Result:   result,
			Error:    err,
		}
		summary.Results = append(summary.Results, batchResult)

		if err != nil {
			summary.Failed++
		} else {
			summary.Success++
		}

		if onProgress != nil {
			onProgress(i+1, len(files), batchResult)
		}
	}

	return summary
}

// BatchUploadFile — файл для batch-загрузки.
type BatchUploadFile struct {
	Filename string
	Reader   io.Reader
	Size     int64
}

// BatchUploadSummary — сводка по batch-загрузке.
type BatchUploadSummary struct {
	Total   int
	Success int
	Failed  int
	Results []BatchUploadResult
}
