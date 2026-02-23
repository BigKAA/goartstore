// Пакет handlers — заглушки для всех endpoints ServerInterface.
// Все методы возвращают 501 Not Implemented до реализации в Phase 3.
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/arturkryukov/artstore/storage-element/internal/api/generated"
)

// StubHandler реализует generated.ServerInterface.
// Все методы возвращают 501 Not Implemented.
type StubHandler struct{}

// NewStubHandler создаёт заглушку ServerInterface.
func NewStubHandler() *StubHandler {
	return &StubHandler{}
}

// notImplemented отправляет стандартный ответ 501 в формате ошибки Artstore.
func notImplemented(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    "NOT_IMPLEMENTED",
			"message": "Endpoint ещё не реализован",
		},
	})
}

// --- File Operations ---

func (s *StubHandler) ListFiles(w http.ResponseWriter, r *http.Request, params generated.ListFilesParams) {
	notImplemented(w)
}

func (s *StubHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	notImplemented(w)
}

func (s *StubHandler) DeleteFile(w http.ResponseWriter, r *http.Request, fileId generated.FileId) {
	notImplemented(w)
}

func (s *StubHandler) GetFileMetadata(w http.ResponseWriter, r *http.Request, fileId generated.FileId) {
	notImplemented(w)
}

func (s *StubHandler) UpdateFileMetadata(w http.ResponseWriter, r *http.Request, fileId generated.FileId) {
	notImplemented(w)
}

func (s *StubHandler) DownloadFile(w http.ResponseWriter, r *http.Request, fileId generated.FileId, params generated.DownloadFileParams) {
	notImplemented(w)
}

// --- System ---

func (s *StubHandler) GetStorageInfo(w http.ResponseWriter, r *http.Request) {
	notImplemented(w)
}

// --- Maintenance ---

func (s *StubHandler) Reconcile(w http.ResponseWriter, r *http.Request) {
	notImplemented(w)
}

// --- Mode ---

func (s *StubHandler) TransitionMode(w http.ResponseWriter, r *http.Request) {
	notImplemented(w)
}

// --- Health ---
// Health endpoints реализуются в health.go, здесь оставлены заглушки для компиляции.

func (s *StubHandler) HealthLive(w http.ResponseWriter, r *http.Request) {
	notImplemented(w)
}

func (s *StubHandler) HealthReady(w http.ResponseWriter, r *http.Request) {
	notImplemented(w)
}

func (s *StubHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	notImplemented(w)
}

// Проверка соответствия интерфейсу на этапе компиляции.
var _ generated.ServerInterface = (*StubHandler)(nil)
