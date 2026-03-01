// health.go — обработчики health endpoints Ingester Module.
// /health/live — liveness probe (процесс жив)
// /health/ready — readiness probe (stub: всегда ok, полная реализация в Phase 3.7)
// /metrics — Prometheus метрики
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/bigkaa/goartstore/ingester-module/internal/config"
)

// ReadinessChecker — интерфейс проверки готовности зависимости.
type ReadinessChecker interface {
	// CheckReady возвращает статус ("ok", "degraded", "fail") и сообщение.
	CheckReady() (status, message string)
}

// HealthHandler — обработчик health endpoints.
type HealthHandler struct {
	promHandler http.Handler
}

// NewHealthHandler создаёт обработчик health endpoints.
// Checkers для Admin Module и JWKS будут добавлены в Phase 3.7.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{
		promHandler: promhttp.Handler(),
	}
}

// healthCheckResult — результат проверки одной зависимости.
type healthCheckResult struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// healthLiveResponse — ответ liveness probe.
type healthLiveResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Version   string `json:"version"`
	Service   string `json:"service"`
}

// healthReadyResponse — ответ readiness probe.
type healthReadyResponse struct {
	Status    string                       `json:"status"`
	Timestamp string                       `json:"timestamp"`
	Version   string                       `json:"version"`
	Service   string                       `json:"service"`
	Checks    map[string]healthCheckResult `json:"checks"`
}

// HealthLive — liveness probe. Возвращает 200 если процесс жив.
func (h *HealthHandler) HealthLive(w http.ResponseWriter, _ *http.Request) {
	resp := healthLiveResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   config.Version,
		Service:   "ingester-module",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// HealthReady — readiness probe (stub: всегда ok).
// Полная реализация (Admin Module + JWKS checkers) — в Phase 3.7.
func (h *HealthHandler) HealthReady(w http.ResponseWriter, _ *http.Request) {
	resp := healthReadyResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   config.Version,
		Service:   "ingester-module",
		Checks: map[string]healthCheckResult{
			"admin_module": {Status: "ok", Message: "stub — полная проверка в Phase 3.7"},
			"jwks":         {Status: "ok", Message: "stub — полная проверка в Phase 3.7"},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// GetMetrics — Prometheus метрики.
func (h *HealthHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	h.promHandler.ServeHTTP(w, r)
}

// overallStatus определяет итоговый статус из статусов зависимостей.
// Если хотя бы одна зависимость fail — итог fail.
// Если хотя бы одна degraded — итог degraded.
// Иначе — ok.
//
//nolint:unused // используется в Phase 3.7
func overallStatus(statuses ...string) string {
	hasDegraded := false
	for _, s := range statuses {
		if s == "fail" {
			return "fail"
		}
		if s == "degraded" {
			hasDegraded = true
		}
	}
	if hasDegraded {
		return "degraded"
	}
	return "ok"
}
