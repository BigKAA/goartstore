// metrics.go — Prometheus HTTP метрики для Admin Module.
// Регистрирует метрики: am_http_requests_total, am_http_request_duration_seconds.
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
			Name: "am_http_requests_total",
			Help: "Общее количество HTTP-запросов к Admin Module",
		},
		[]string{"method", "path", "status"},
	)

	// httpRequestDuration — гистограмма длительности HTTP-запросов.
	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "am_http_request_duration_seconds",
			Help:    "Длительность HTTP-запросов к Admin Module в секундах",
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
// /api/v1/admin-users/a1b2c3d4-... → /api/v1/admin-users/{id}
func normalizePath(path string) string {
	// Статические пути — возвращаем как есть
	switch path {
	case "/health/live", "/health/ready", "/metrics",
		"/api/v1/admin-auth/me",
		"/api/v1/admin-users",
		"/api/v1/service-accounts",
		"/api/v1/storage-elements",
		"/api/v1/storage-elements/discover",
		"/api/v1/files",
		"/api/v1/idp/status",
		"/api/v1/idp/sync-sa":
		return path
	}

	// Динамические пути с UUID
	prefixes := []struct {
		prefix string
		result string
	}{
		{"/api/v1/admin-users/", "/api/v1/admin-users/{id}"},
		{"/api/v1/service-accounts/", "/api/v1/service-accounts/{id}"},
		{"/api/v1/storage-elements/", "/api/v1/storage-elements/{id}"},
		{"/api/v1/files/", "/api/v1/files/{id}"},
	}

	for _, p := range prefixes {
		if len(path) > len(p.prefix) && path[:len(p.prefix)] == p.prefix {
			suffix := ""
			// Проверяем суффиксы после UUID (36 символов)
			if len(path) > len(p.prefix)+36 {
				suffix = path[len(p.prefix)+36:]
			}
			switch suffix {
			case "/role-override":
				return p.result + "/role-override"
			case "/rotate-secret":
				return p.result + "/rotate-secret"
			case "/sync":
				return p.result + "/sync"
			default:
				return p.result
			}
		}
	}

	return path
}
