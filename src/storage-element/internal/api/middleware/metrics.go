// metrics.go — Prometheus HTTP метрики для Storage Element.
// Регистрирует метрики: se_http_requests_total, se_http_request_duration_seconds.
// Бизнес-метрики (se_files_total, se_storage_bytes и др.) регистрируются
// в соответствующих пакетах и обновляются из сервисного слоя.
package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// HTTP метрики
var (
	// httpRequestsTotal — общее количество HTTP-запросов.
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "se_http_requests_total",
			Help: "Общее количество HTTP-запросов к Storage Element",
		},
		[]string{"method", "path", "status"},
	)

	// httpRequestDuration — гистограмма длительности HTTP-запросов.
	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "se_http_request_duration_seconds",
			Help:    "Длительность HTTP-запросов к Storage Element в секундах",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

// Бизнес-метрики (экспортируются для обновления из сервисного слоя)
var (
	// FilesTotal — текущее количество файлов в хранилище (gauge).
	FilesTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "se_files_total",
			Help: "Текущее количество файлов в хранилище",
		},
		[]string{"status"},
	)

	// StorageBytes — объём занятого дискового пространства (gauge).
	StorageBytes = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "se_storage_bytes",
			Help: "Объём занятого дискового пространства в байтах",
		},
	)

	// OperationsTotal — общее количество файловых операций.
	OperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "se_operations_total",
			Help: "Общее количество файловых операций",
		},
		[]string{"operation", "result"},
	)
)

// MetricsMiddleware возвращает HTTP middleware для сбора Prometheus метрик.
// Записывает количество запросов и длительность для каждого endpoint.
func MetricsMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Нормализуем путь для лейблов метрик
			// (заменяем UUID на {id} для предотвращения кардинальности)
			normalizedPath := normalizePath(r.URL.Path)

			wrapped := newMetricsResponseWriter(w)
			next.ServeHTTP(wrapped, r)

			duration := time.Since(start).Seconds()
			status := strconv.Itoa(wrapped.statusCode)

			httpRequestsTotal.WithLabelValues(r.Method, normalizedPath, status).Inc()
			httpRequestDuration.WithLabelValues(r.Method, normalizedPath).Observe(duration)
		})
	}
}

// metricsResponseWriter — обёртка для перехвата статус-кода.
type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newMetricsResponseWriter(w http.ResponseWriter) *metricsResponseWriter {
	return &metricsResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *metricsResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Unwrap позволяет http.ResponseController получить доступ к оригинальному ResponseWriter.
func (rw *metricsResponseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// normalizePath заменяет UUID-сегменты пути на {id} для предотвращения
// взрывного роста кардинальности метрик.
// /api/v1/files/a1b2c3d4-e5f6-7890-abcd-ef1234567890 → /api/v1/files/{id}
func normalizePath(path string) string {
	// Простые паттерны для основных endpoints
	switch {
	case path == "/health/live":
		return "/health/live"
	case path == "/health/ready":
		return "/health/ready"
	case path == "/metrics":
		return "/metrics"
	case path == "/api/v1/info":
		return "/api/v1/info"
	case path == "/api/v1/files":
		return "/api/v1/files"
	case path == "/api/v1/files/upload":
		return "/api/v1/files/upload"
	case path == "/api/v1/mode/transition":
		return "/api/v1/mode/transition"
	case path == "/api/v1/maintenance/reconcile":
		return "/api/v1/maintenance/reconcile"
	case len(path) > len("/api/v1/files/") && isUUIDSegment(path, "/api/v1/files/"):
		// /api/v1/files/{uuid}/download или /api/v1/files/{uuid}
		suffix := path[len("/api/v1/files/")+36:]
		if suffix == "/download" {
			return "/api/v1/files/{id}/download"
		}
		if suffix == "" {
			return "/api/v1/files/{id}"
		}
	}
	return path
}

// isUUIDSegment проверяет, начинается ли сегмент пути после prefix с UUID.
func isUUIDSegment(path, prefix string) bool {
	if len(path) < len(prefix)+36 {
		return false
	}
	segment := path[len(prefix) : len(prefix)+36]
	// Проверяем формат UUID: 8-4-4-4-12
	if len(segment) != 36 {
		return false
	}
	for i, c := range segment {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}
