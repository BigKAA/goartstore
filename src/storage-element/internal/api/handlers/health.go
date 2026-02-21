// health.go — обработчики health endpoints для Kubernetes probes.
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/arturkryukov/artsore/storage-element/internal/config"
)

// HealthHandler реализует health endpoints: /health/live, /health/ready.
type HealthHandler struct {
	version string
}

// NewHealthHandler создаёт обработчик health endpoints.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{
		version: config.Version,
	}
}

// HealthLive обрабатывает GET /health/live.
// Возвращает 200, если процесс SE жив. Не проверяет зависимости.
func (h *HealthHandler) HealthLive(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   h.version,
		"service":   "storage-element",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// HealthReady обрабатывает GET /health/ready.
// Заглушка: возвращает ok. Полная реализация (проверка FS, WAL, индекс) — Phase 3.
func (h *HealthHandler) HealthReady(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   h.version,
		"service":   "storage-element",
		"checks": map[string]any{
			"filesystem": map[string]any{
				"status":  "ok",
				"message": "Заглушка — проверка не выполняется",
			},
			"wal": map[string]any{
				"status":  "ok",
				"message": "Заглушка — проверка не выполняется",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
