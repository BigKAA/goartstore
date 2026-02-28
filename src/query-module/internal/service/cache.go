// Пакет service — бизнес-логика Query Module.
// CacheService — LRU-кэш метаданных файлов с TTL.
// Обёртка над hashicorp/golang-lru/v2/expirable.
package service

import (
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/bigkaa/goartstore/query-module/internal/domain/model"
)

// Prometheus-метрики кэша.
var (
	cacheHitsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "qm_cache_hits_total",
		Help: "Общее количество попаданий в LRU-кэш метаданных.",
	})
	cacheMissesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "qm_cache_misses_total",
		Help: "Общее количество промахов LRU-кэша метаданных.",
	})
)

// CacheService — LRU-кэш метаданных файлов с автоматическим TTL.
// Каждый экземпляр QM имеет собственный in-memory кэш (per-instance, stateless архитектура).
type CacheService struct {
	cache *expirable.LRU[string, *model.FileRecord]
}

// NewCacheService создаёт LRU-кэш с указанным максимальным размером и TTL.
// maxSize — максимальное количество записей в кэше.
// ttl — время жизни записи после добавления.
func NewCacheService(maxSize int, ttl time.Duration) *CacheService {
	cache := expirable.NewLRU[string, *model.FileRecord](maxSize, nil, ttl)
	return &CacheService{cache: cache}
}

// Get возвращает FileRecord из кэша по fileID.
// Возвращает (запись, true) при hit или (nil, false) при miss.
// Обновляет Prometheus-метрики hit/miss.
func (c *CacheService) Get(fileID string) (*model.FileRecord, bool) {
	val, ok := c.cache.Get(fileID)
	if ok {
		cacheHitsTotal.Inc()
		return val, true
	}
	cacheMissesTotal.Inc()
	return nil, false
}

// Set добавляет или обновляет запись в кэше.
func (c *CacheService) Set(fileID string, record *model.FileRecord) {
	c.cache.Add(fileID, record)
}

// Delete удаляет запись из кэша (инвалидация при lazy cleanup).
func (c *CacheService) Delete(fileID string) {
	c.cache.Remove(fileID)
}
