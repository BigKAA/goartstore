package service

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/bigkaa/goartstore/demo-client/internal/activity"
	"github.com/bigkaa/goartstore/demo-client/internal/gateway"
)

// SearchService — сервис поиска файлов.
type SearchService struct {
	gw     *gateway.Client
	log    *activity.Log
	logger *slog.Logger
}

// NewSearchService создаёт новый сервис поиска.
func NewSearchService(gw *gateway.Client, log *activity.Log, logger *slog.Logger) *SearchService {
	return &SearchService{
		gw:     gw,
		log:    log,
		logger: logger,
	}
}

// Search выполняет поиск файлов через Gateway.
func (s *SearchService) Search(params gateway.SearchRequest) (*gateway.SearchResponse, error) {
	start := time.Now()

	result, err := s.gw.Search(params)
	duration := time.Since(start)

	// Формируем описание для Activity Log.
	desc := "поиск файлов"
	if params.Query != "" {
		desc = fmt.Sprintf("поиск: %q", params.Query)
	} else if params.Filename != "" {
		desc = fmt.Sprintf("поиск по имени: %q", params.Filename)
	}

	entry := activity.Entry{
		Timestamp:   time.Now(),
		Method:      "POST",
		Path:        "/query/api/v1/search",
		DurationMs:  duration.Milliseconds(),
		Description: desc,
	}

	if err != nil {
		entry.StatusCode = 0
		entry.Error = err.Error()
		s.log.Append(entry)
		s.logger.Error("ошибка поиска файлов",
			"query", params.Query,
			"error", err,
			"duration_ms", duration.Milliseconds(),
		)
		return nil, err
	}

	entry.StatusCode = 200
	entry.Description = fmt.Sprintf("%s (найдено: %d)", desc, result.Total)
	s.log.Append(entry)
	s.logger.Info("поиск завершён",
		"query", params.Query,
		"total", result.Total,
		"returned", len(result.Items),
		"duration_ms", duration.Milliseconds(),
	)
	return result, nil
}

// DeleteFile удаляет файл через Gateway (Ingester Module).
func (s *SearchService) DeleteFile(fileID, storageElementID string) error {
	start := time.Now()

	err := s.gw.DeleteFile(fileID, storageElementID)
	duration := time.Since(start)

	entry := activity.Entry{
		Timestamp:   time.Now(),
		Method:      "DELETE",
		Path:        "/upload/api/v1/files/" + fileID,
		DurationMs:  duration.Milliseconds(),
		Description: fmt.Sprintf("удаление файла: %s", fileID),
	}

	if err != nil {
		entry.StatusCode = 0
		entry.Error = err.Error()
		s.log.Append(entry)
		s.logger.Error("ошибка удаления файла",
			"file_id", fileID,
			"storage_element_id", storageElementID,
			"error", err,
			"duration_ms", duration.Milliseconds(),
		)
		return err
	}

	entry.StatusCode = 204
	s.log.Append(entry)
	s.logger.Info("файл удалён",
		"file_id", fileID,
		"storage_element_id", storageElementID,
		"duration_ms", duration.Milliseconds(),
	)
	return nil
}

// GetMetadata получает метаданные файла через Gateway.
func (s *SearchService) GetMetadata(fileID string) (*gateway.FileInfo, error) {
	start := time.Now()

	result, err := s.gw.GetMetadata(fileID)
	duration := time.Since(start)

	entry := activity.Entry{
		Timestamp:   time.Now(),
		Method:      "GET",
		Path:        "/query/api/v1/files/" + fileID,
		DurationMs:  duration.Milliseconds(),
		Description: fmt.Sprintf("метаданные файла: %s", fileID),
	}

	if err != nil {
		entry.StatusCode = 0
		entry.Error = err.Error()
		s.log.Append(entry)
		return nil, err
	}

	entry.StatusCode = 200
	s.log.Append(entry)
	return result, nil
}
