// search.go — сервис поиска и получения метаданных файлов.
// Координирует repository, LRU cache и Prometheus-метрики.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/bigkaa/goartstore/query-module/internal/domain/model"
	"github.com/bigkaa/goartstore/query-module/internal/repository"
)

// Ошибки сервисного слоя.
var (
	// ErrNotFound — файл не найден.
	ErrNotFound = errors.New("файл не найден")
)

// Prometheus-метрики поиска.
var (
	searchTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "qm_search_total",
		Help: "Общее количество поисковых запросов.",
	})
	searchDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "qm_search_duration_seconds",
		Help:    "Длительность поисковых запросов.",
		Buckets: prometheus.DefBuckets,
	})
)

// SearchResult — результат поиска с пагинацией.
type SearchResult struct {
	// Items — найденные файлы
	Items []*model.FileRecord
	// Total — общее количество совпадений
	Total int
	// Limit — запрошенный лимит
	Limit int
	// Offset — текущее смещение
	Offset int
	// HasMore — есть ли ещё результаты
	HasMore bool
}

// SearchService — сервис поиска файлов и получения метаданных.
type SearchService struct {
	fileRepo repository.FileRepository
	cache    *CacheService
	logger   *slog.Logger
}

// NewSearchService создаёт сервис поиска.
func NewSearchService(
	fileRepo repository.FileRepository,
	cache *CacheService,
	logger *slog.Logger,
) *SearchService {
	return &SearchService{
		fileRepo: fileRepo,
		cache:    cache,
		logger:   logger.With(slog.String("component", "search_service")),
	}
}

// Search выполняет поиск файлов по параметрам.
// Обновляет Prometheus-метрики (search_total, search_duration_seconds).
func (s *SearchService) Search(ctx context.Context, params repository.SearchParams) (*SearchResult, error) {
	start := time.Now()
	searchTotal.Inc()

	items, total, err := s.fileRepo.Search(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("поиск файлов: %w", err)
	}

	duration := time.Since(start)
	searchDuration.Observe(duration.Seconds())

	s.logger.Debug("Поиск выполнен",
		slog.Int("total", total),
		slog.Int("returned", len(items)),
		slog.Duration("duration", duration),
	)

	return &SearchResult{
		Items:   items,
		Total:   total,
		Limit:   params.Limit,
		Offset:  params.Offset,
		HasMore: params.Offset+len(items) < total,
	}, nil
}

// GetFileMetadata возвращает метаданные файла.
// Сначала проверяет LRU-кэш, при промахе — запрос к PostgreSQL, результат кэшируется.
func (s *SearchService) GetFileMetadata(ctx context.Context, fileID string) (*model.FileRecord, error) {
	// Проверяем кэш
	if record, ok := s.cache.Get(fileID); ok {
		s.logger.Debug("Кэш hit для файла", slog.String("file_id", fileID))
		return record, nil
	}

	// Cache miss — запрос к БД
	record, err := s.fileRepo.GetByID(ctx, fileID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("получение метаданных файла: %w", err)
	}

	// Сохраняем в кэш
	s.cache.Set(fileID, record)

	return record, nil
}
