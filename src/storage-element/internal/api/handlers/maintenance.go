// maintenance.go — обработчик POST /api/v1/maintenance/reconcile.
// Заглушка: полная реализация reconciliation — Phase 4.
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/arturkryukov/artsore/storage-element/internal/api/generated"
)

// MaintenanceHandler — обработчик endpoints обслуживания.
type MaintenanceHandler struct{}

// NewMaintenanceHandler создаёт обработчик maintenance endpoints.
func NewMaintenanceHandler() *MaintenanceHandler {
	return &MaintenanceHandler{}
}

// Reconcile обрабатывает POST /api/v1/maintenance/reconcile.
// Заглушка: возвращает пустой результат. Полная реализация в Phase 4.
func (h *MaintenanceHandler) Reconcile(w http.ResponseWriter, r *http.Request) {
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
}
