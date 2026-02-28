// maintenance.go — обработчик POST /api/v1/maintenance/reconcile.
// Делегирует reconciliation в ReconcileService.
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	apierrors "github.com/bigkaa/goartstore/storage-element/internal/api/errors"
	"github.com/bigkaa/goartstore/storage-element/internal/api/generated"
)

// ReconcileRunner — интерфейс для запуска reconciliation.
// Позволяет тестировать handler без полного ReconcileService.
type ReconcileRunner interface {
	// RunOnce выполняет один цикл reconciliation.
	// Возвращает результат и флаг "уже выполняется".
	RunOnce() (*generated.ReconcileResponse, bool)
	// IsInProgress возвращает true, если reconciliation выполняется.
	IsInProgress() bool
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
// Если reconciliation уже выполняется — 409 RECONCILE_IN_PROGRESS.
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
	result, inProgress := h.reconciler.RunOnce()
	if inProgress {
		apierrors.ReconcileInProgress(w, "Reconciliation уже выполняется")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}
