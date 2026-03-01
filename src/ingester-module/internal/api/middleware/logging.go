// logging.go — middleware логирования входящих HTTP-запросов через slog.
// Перехватывает статус-код, размер ответа и длительность обработки.
package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// responseWriter — обёртка для перехвата статус-кода ответа.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int64
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.written += int64(n)
	return n, err
}

// Unwrap позволяет http.ResponseController получить доступ к оригинальному ResponseWriter.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// RequestLogger возвращает middleware, логирующий каждый HTTP-запрос:
// метод, путь, статус, длительность, размер ответа, remote_addr.
// Уровень логирования зависит от статус-кода: INFO (1xx-3xx), WARN (4xx), ERROR (5xx).
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := newResponseWriter(w)

			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)

			// Уровень логирования зависит от статус-кода
			level := slog.LevelInfo
			if wrapped.statusCode >= 500 {
				level = slog.LevelError
			} else if wrapped.statusCode >= 400 {
				level = slog.LevelWarn
			}

			logger.LogAttrs(r.Context(), level, "HTTP запрос",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", wrapped.statusCode),
				slog.Duration("duration", duration),
				slog.Int64("bytes", wrapped.written),
				slog.String("remote_addr", r.RemoteAddr),
			)
		})
	}
}
