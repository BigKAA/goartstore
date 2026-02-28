// metrics.go — Prometheus HTTP метрики для Query Module.
// Регистрирует метрики: qm_http_requests_total, qm_http_request_duration_seconds.
// Нормализация путей предотвращает взрывной рост кардинальности.
package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// HTTP метрики Query Module
var (
	// httpRequestsTotal — общее количество HTTP-запросов.
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "qm_http_requests_total",
			Help: "Общее количество HTTP-запросов к Query Module",
		},
		[]string{"method", "path", "status"},
	)

	// httpRequestDuration — гистограмма длительности HTTP-запросов.
	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "qm_http_request_duration_seconds",
			Help:    "Длительность HTTP-запросов к Query Module в секундах",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
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
// /api/v1/files/a1b2c3d4-... → /api/v1/files/{id}
// /api/v1/files/a1b2c3d4-.../download → /api/v1/files/{id}/download
func normalizePath(path string) string {
	// Статические пути — возвращаем как есть
	switch path {
	case "/health/live", "/health/ready", "/metrics",
		"/api/v1/search":
		return path
	}

	// Динамические пути с UUID
	const filesPrefix = "/api/v1/files/"
	if len(path) > len(filesPrefix) && path[:len(filesPrefix)] == filesPrefix {
		// Проверяем суффиксы после UUID (36 символов)
		suffix := ""
		if len(path) > len(filesPrefix)+36 {
			suffix = path[len(filesPrefix)+36:]
		}
		switch suffix {
		case "/download":
			return "/api/v1/files/{id}/download"
		default:
			return "/api/v1/files/{id}"
		}
	}

	return path
}
