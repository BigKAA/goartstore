// maintenance.go — обработчик POST /api/v1/maintenance/reconcile.
// Делегирует reconciliation в ReconcileService.
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/bigkaa/goartstore/storage-element/internal/api/generated"
)

// ReconcileRunner — интерфейс для запуска reconciliation.
// Позволяет тестировать handler без полного ReconcileService.
type ReconcileRunner interface {
	// RunOnce выполняет один цикл reconciliation.
	// Idempotent: несколько pod-ов могут запускать одновременно.
	RunOnce() *generated.ReconcileResponse
}

// MaintenanceHandler — обработчик endpoints обслуживания.
type MaintenanceHandler struct {
	reconciler ReconcileRunner
}

// NewMaintenanceHandler создаёт обработчик maintenance endpoints.
// reconciler может быть nil (заглушка — возвращает пустой результат).
func NewMaintenanceHandler(reconciler ...ReconcileRunner) *MaintenanceHandler {
	h := &MaintenanceHandler{}
	if len(reconciler) > 0 {
		h.reconciler = reconciler[0]
	}
	return h
}

// Reconcile обрабатывает POST /api/v1/maintenance/reconcile.
// Запускает синхронный цикл reconciliation и возвращает результат.
func (h *MaintenanceHandler) Reconcile(w http.ResponseWriter, _ *http.Request) {
	// Если reconciler не настроен — возвращаем заглушку
	if h.reconciler == nil {
		now := time.Now().UTC()
		resp := generated.ReconcileResponse{
			StartedAt:    now,
			CompletedAt:  now,
			FilesChecked: 0,
			Issues:       []generated.ReconcileIssue{},
			Summary: generated.ReconcileSummary{
				Ok:                 0,
				OrphanedFiles:      0,
				MissingFiles:       0,
				ChecksumMismatches: 0,
				SizeMismatches:     0,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	// Запускаем reconciliation
	result := h.reconciler.RunOnce()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}
