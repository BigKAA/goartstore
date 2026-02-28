// health.go — обработчики health endpoints Query Module.
// /health/live — liveness probe (процесс жив)
// /health/ready — readiness probe (PostgreSQL доступен)
// /metrics — Prometheus метрики
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/bigkaa/goartstore/query-module/internal/config"
)

// ReadinessChecker — интерфейс проверки готовности зависимости.
type ReadinessChecker interface {
	// CheckReady возвращает статус ("ok", "degraded", "fail") и сообщение.
	CheckReady() (status, message string)
}

// HealthHandler — обработчик health endpoints.
type HealthHandler struct {
	pgChecker   ReadinessChecker
	promHandler http.Handler
}

// NewHealthHandler создаёт обработчик health endpoints.
// pgChecker — проверка PostgreSQL (может быть nil — readiness вернёт "fail").
func NewHealthHandler(pgChecker ReadinessChecker) *HealthHandler {
	return &HealthHandler{
		pgChecker:   pgChecker,
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
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Version   string `json:"version"`
	Service   string `json:"service"`
	Checks    struct {
		PostgreSQL healthCheckResult `json:"postgresql"`
	} `json:"checks"`
}

// HealthLive — liveness probe. Возвращает 200 если процесс жив.
func (h *HealthHandler) HealthLive(w http.ResponseWriter, _ *http.Request) {
	resp := healthLiveResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   config.Version,
		Service:   "query-module",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// HealthReady — readiness probe. Проверяет PostgreSQL.
// Возвращает 200 (ok/degraded) или 503 (fail).
func (h *HealthHandler) HealthReady(w http.ResponseWriter, _ *http.Request) {
	resp := healthReadyResponse{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   config.Version,
		Service:   "query-module",
	}

	// Проверяем PostgreSQL
	if h.pgChecker != nil {
		pgStatus, pgMsg := h.pgChecker.CheckReady()
		resp.Checks.PostgreSQL = healthCheckResult{Status: pgStatus, Message: pgMsg}
	} else {
		resp.Checks.PostgreSQL = healthCheckResult{Status: statusFail, Message: "не инициализирован"}
	}

	// Определяем итоговый статус
	resp.Status = overallStatus(resp.Checks.PostgreSQL.Status)

	w.Header().Set("Content-Type", "application/json")
	if resp.Status == statusFail {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// GetMetrics — Prometheus метрики.
func (h *HealthHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	h.promHandler.ServeHTTP(w, r)
}

// Константы статусов health check.
const statusFail = "fail"

// overallStatus определяет итоговый статус из статусов зависимостей.
// Если хотя бы одна зависимость fail — итог fail.
// Если хотя бы одна degraded — итог degraded.
// Иначе — ok.
func overallStatus(statuses ...string) string {
	hasDegraded := false
	for _, s := range statuses {
		if s == statusFail {
			return statusFail
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
