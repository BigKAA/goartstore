// health.go — обработчики health endpoints Admin Module.
// /health/live — liveness probe (процесс жив)
// /health/ready — readiness probe (PostgreSQL + Keycloak доступны)
// /metrics — Prometheus метрики
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/bigkaa/goartstore/admin-module/internal/config"
)

// ReadinessChecker — интерфейс проверки готовности зависимости.
type ReadinessChecker interface {
	// CheckReady возвращает статус ("ok", "degraded", "fail") и сообщение.
	CheckReady() (status string, message string)
}

// HealthHandler — обработчик health endpoints.
type HealthHandler struct {
	pgChecker ReadinessChecker
	kcChecker ReadinessChecker
	promHandler http.Handler
}

// NewHealthHandler создаёт обработчик health endpoints.
// pgChecker — проверка PostgreSQL, kcChecker — проверка Keycloak.
// Оба могут быть nil (readiness вернёт "fail" для nil зависимостей).
func NewHealthHandler(pgChecker, kcChecker ReadinessChecker) *HealthHandler {
	return &HealthHandler{
		pgChecker:   pgChecker,
		kcChecker:   kcChecker,
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
		Keycloak   healthCheckResult `json:"keycloak"`
	} `json:"checks"`
}

// HealthLive — liveness probe. Возвращает 200 если процесс жив.
func (h *HealthHandler) HealthLive(w http.ResponseWriter, r *http.Request) {
	resp := healthLiveResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   config.Version,
		Service:   "admin-module",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// HealthReady — readiness probe. Проверяет PostgreSQL и Keycloak.
// Возвращает 200 (ok/degraded) или 503 (fail).
func (h *HealthHandler) HealthReady(w http.ResponseWriter, r *http.Request) {
	resp := healthReadyResponse{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   config.Version,
		Service:   "admin-module",
	}

	// Проверяем PostgreSQL
	if h.pgChecker != nil {
		pgStatus, pgMsg := h.pgChecker.CheckReady()
		resp.Checks.PostgreSQL = healthCheckResult{Status: pgStatus, Message: pgMsg}
	} else {
		resp.Checks.PostgreSQL = healthCheckResult{Status: "fail", Message: "не инициализирован"}
	}

	// Проверяем Keycloak
	if h.kcChecker != nil {
		kcStatus, kcMsg := h.kcChecker.CheckReady()
		resp.Checks.Keycloak = healthCheckResult{Status: kcStatus, Message: kcMsg}
	} else {
		resp.Checks.Keycloak = healthCheckResult{Status: "fail", Message: "не инициализирован"}
	}

	// Определяем итоговый статус
	resp.Status = overallStatus(resp.Checks.PostgreSQL.Status, resp.Checks.Keycloak.Status)

	w.Header().Set("Content-Type", "application/json")
	if resp.Status == "fail" {
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

// overallStatus определяет итоговый статус из статусов зависимостей.
// Если хотя бы одна зависимость fail — итог fail.
// Если хотя бы одна degraded — итог degraded.
// Иначе — ok.
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
