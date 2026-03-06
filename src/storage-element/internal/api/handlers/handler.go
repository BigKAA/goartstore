// handler.go — APIHandler реализует generated.ServerInterface,
// делегируя вызовы в отдельные handler'ы по доменам.
package handlers

import (
	"net/http"

	"github.com/bigkaa/goartstore/storage-element/internal/api/generated"
	"github.com/bigkaa/goartstore/storage-element/internal/server"
)

// APIHandler — единая реализация ServerInterface, собирающая
// все доменные handlers в один объект.
type APIHandler struct {
	files       *FilesHandler
	system      *SystemHandler
	modeHandler *ModeHandler
	maintenance *MaintenanceHandler
	locks       *LocksHandler
	health      *HealthHandler
	metrics     *server.MetricsHandler
}

// NewAPIHandler создаёт единый handler для всех endpoints.
func NewAPIHandler(
	files *FilesHandler,
	system *SystemHandler,
	modeHandler *ModeHandler,
	maintenance *MaintenanceHandler,
	locks *LocksHandler,
	health *HealthHandler,
	metrics *server.MetricsHandler,
) *APIHandler {
	return &APIHandler{
		files:       files,
		system:      system,
		modeHandler: modeHandler,
		maintenance: maintenance,
		locks:       locks,
		health:      health,
		metrics:     metrics,
	}
}

// --- File Operations ---

func (h *APIHandler) ListFiles(w http.ResponseWriter, r *http.Request, params generated.ListFilesParams) {
	h.files.ListFiles(w, r, params)
}

func (h *APIHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	h.files.UploadFile(w, r)
}

func (h *APIHandler) DeleteFile(w http.ResponseWriter, r *http.Request, fileId generated.FileId) { //nolint:revive // имя fileId задано сгенерированным интерфейсом
	h.files.DeleteFile(w, r, fileId)
}

func (h *APIHandler) GetFileMetadata(w http.ResponseWriter, r *http.Request, fileId generated.FileId) { //nolint:revive // имя fileId задано сгенерированным интерфейсом
	h.files.GetFileMetadata(w, r, fileId)
}

func (h *APIHandler) UpdateFileMetadata(w http.ResponseWriter, r *http.Request, fileId generated.FileId) { //nolint:revive // имя fileId задано сгенерированным интерфейсом
	h.files.UpdateFileMetadata(w, r, fileId)
}

func (h *APIHandler) DownloadFile(w http.ResponseWriter, r *http.Request, fileId generated.FileId, params generated.DownloadFileParams) { //nolint:revive // имя fileId задано сгенерированным интерфейсом
	h.files.DownloadFile(w, r, fileId, params)
}

// --- System ---

func (h *APIHandler) GetStorageInfo(w http.ResponseWriter, r *http.Request) {
	h.system.GetStorageInfo(w, r)
}

// --- Mode ---

func (h *APIHandler) TransitionMode(w http.ResponseWriter, r *http.Request) {
	h.modeHandler.TransitionMode(w, r)
}

// --- Maintenance ---

func (h *APIHandler) Reconcile(w http.ResponseWriter, r *http.Request) {
	h.maintenance.Reconcile(w, r)
}

// --- Locks ---

// ListLocks делегирует GET /api/v1/locks.
func (h *APIHandler) ListLocks(w http.ResponseWriter, r *http.Request) {
	h.locks.ListLocks(w, r)
}

// CleanupLocks делегирует POST /api/v1/locks/cleanup.
func (h *APIHandler) CleanupLocks(w http.ResponseWriter, r *http.Request, params generated.CleanupLocksParams) {
	h.locks.CleanupLocks(w, r, params)
}

// --- Health ---

func (h *APIHandler) HealthLive(w http.ResponseWriter, r *http.Request) {
	h.health.HealthLive(w, r)
}

func (h *APIHandler) HealthReady(w http.ResponseWriter, r *http.Request) {
	h.health.HealthReady(w, r)
}

// --- Metrics ---

func (h *APIHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	h.metrics.ServeHTTP(w, r)
}

// Проверка соответствия интерфейсу на этапе компиляции.
var _ generated.ServerInterface = (*APIHandler)(nil)
