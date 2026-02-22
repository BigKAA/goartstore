// Пакет handlers — HTTP-обработчики Admin Module.
// stub.go — заглушка, реализующая ServerInterface.
// Все endpoints кроме health/metrics возвращают 501 Not Implemented
// через встроенный Unimplemented из oapi-codegen.
// Health и metrics делегируются в HealthHandler.
package handlers

import (
	"net/http"

	"github.com/arturkryukov/artsore/admin-module/internal/api/generated"
)

// StubHandler — заглушка ServerInterface.
// Встраивает generated.Unimplemented для 501 ответов на все endpoints.
// Переопределяет HealthLive, HealthReady, GetMetrics через HealthHandler.
type StubHandler struct {
	generated.Unimplemented
	health *HealthHandler
}

// NewStubHandler создаёт заглушку ServerInterface с health обработчиком.
func NewStubHandler(health *HealthHandler) *StubHandler {
	return &StubHandler{health: health}
}

// HealthLive — liveness probe (делегируется в HealthHandler).
func (s *StubHandler) HealthLive(w http.ResponseWriter, r *http.Request) {
	s.health.HealthLive(w, r)
}

// HealthReady — readiness probe (делегируется в HealthHandler).
func (s *StubHandler) HealthReady(w http.ResponseWriter, r *http.Request) {
	s.health.HealthReady(w, r)
}

// GetMetrics — Prometheus метрики (делегируется в HealthHandler).
func (s *StubHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	s.health.GetMetrics(w, r)
}
