// metrics.go — Prometheus HTTP метрики для Ingester Module.
// Регистрирует метрики: im_http_requests_total, im_http_request_duration_seconds.
// Нормализация путей проще чем в QM — все пути статические (нет UUID-параметров).
package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// HTTP метрики Ingester Module
var (
	// httpRequestsTotal — общее количество HTTP-запросов.
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "im_http_requests_total",
			Help: "Общее количество HTTP-запросов к Ingester Module",
		},
		[]string{"method", "path", "status"},
	)

	// httpRequestDuration — гистограмма длительности HTTP-запросов.
	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "im_http_request_duration_seconds",
			Help:    "Длительность HTTP-запросов к Ingester Module в секундах",
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

// normalizePath нормализует путь для лейблов метрик.
// IM имеет только статические пути (без UUID-параметров),
// поэтому нормализация проще чем в QM.
func normalizePath(path string) string {
	switch path {
	case "/health/live", "/health/ready", "/metrics",
		"/api/v1/files/upload":
		return path
	}

	// Неизвестные пути — возвращаем как есть
	return path
}
