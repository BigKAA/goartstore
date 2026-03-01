// health.go — обработчики health endpoints Ingester Module.
// /health/live — liveness probe (процесс жив)
// /health/ready — readiness probe (Admin Module + JWKS checkers)
// /metrics — Prometheus метрики
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/bigkaa/goartstore/ingester-module/internal/config"
)

// Статусы readiness check.
const statusFail = "fail"

// ReadinessChecker — интерфейс проверки готовности зависимости.
type ReadinessChecker interface {
	// CheckReady возвращает статус ("ok", "degraded", "fail") и сообщение.
	CheckReady() (status, message string)
}

// HealthHandler — обработчик health endpoints.
type HealthHandler struct {
	promHandler  http.Handler
	adminChecker ReadinessChecker
	jwksChecker  ReadinessChecker
}

// NewHealthHandler создаёт обработчик health endpoints.
// adminChecker и jwksChecker могут быть nil (тогда используются stubs).
func NewHealthHandler(adminChecker, jwksChecker ReadinessChecker) *HealthHandler {
	return &HealthHandler{
		promHandler:  promhttp.Handler(),
		adminChecker: adminChecker,
		jwksChecker:  jwksChecker,
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

// HealthReady — readiness probe. Проверяет Admin Module и JWKS endpoint.
func (h *HealthHandler) HealthReady(w http.ResponseWriter, _ *http.Request) {
	checks := make(map[string]healthCheckResult)
	var statuses []string

	// Проверка Admin Module
	if h.adminChecker != nil {
		status, msg := h.adminChecker.CheckReady()
		checks["admin_module"] = healthCheckResult{Status: status, Message: msg}
		statuses = append(statuses, status)
	} else {
		checks["admin_module"] = healthCheckResult{Status: "ok", Message: "checker не сконфигурирован"}
		statuses = append(statuses, "ok")
	}

	// Проверка JWKS endpoint
	if h.jwksChecker != nil {
		status, msg := h.jwksChecker.CheckReady()
		checks["jwks"] = healthCheckResult{Status: status, Message: msg}
		statuses = append(statuses, status)
	} else {
		checks["jwks"] = healthCheckResult{Status: "ok", Message: "checker не сконфигурирован"}
		statuses = append(statuses, "ok")
	}

	overall := overallStatus(statuses...)

	resp := healthReadyResponse{
		Status:    overall,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   config.Version,
		Service:   "ingester-module",
		Checks:    checks,
	}

	// Статус HTTP: 503 для fail, 200 для остальных
	httpStatus := http.StatusOK
	if overall == statusFail {
		httpStatus = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
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
