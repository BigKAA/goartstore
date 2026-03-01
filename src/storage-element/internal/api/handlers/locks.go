// locks.go — обработчики Lock API для диагностики и очистки lock-файлов.
//
// GET  /api/v1/locks          — список текущих lock-ов
// POST /api/v1/locks/cleanup  — удаление expired lock-ов (force=true для всех)
//
// Оба endpoint-а доступны только пользователям с правами storage:write.
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/bigkaa/goartstore/storage-element/internal/api/errors"
	"github.com/bigkaa/goartstore/storage-element/internal/lockfile"
)

// LockLister — интерфейс для получения списка lock-ов и их очистки.
// Позволяет тестировать handler без реального LockManager.
type LockLister interface {
	// List возвращает все lock-и (активные и expired).
	List() ([]lockfile.LockInfo, error)
	// Cleanup удаляет expired (или все при force=true) lock-файлы.
	Cleanup(force bool) (*lockfile.CleanupResult, error)
}

// LocksHandler — обработчик Lock API endpoints.
type LocksHandler struct {
	locker LockLister
}

// NewLocksHandler создаёт обработчик Lock API.
func NewLocksHandler(locker LockLister) *LocksHandler {
	return &LocksHandler{locker: locker}
}

// lockInfoResponse — представление одного lock-а в API-ответе.
type lockInfoResponse struct {
	FileID     string `json:"file_id"`
	Holder     string `json:"holder"`
	AcquiredAt string `json:"acquired_at"`
	TTLSeconds int    `json:"ttl_seconds"`
	ExpiresAt  string `json:"expires_at"`
	Expired    bool   `json:"expired"`
}

// lockListResponse — ответ GET /api/v1/locks.
type lockListResponse struct {
	Locks   []lockInfoResponse `json:"locks"`
	Total   int                `json:"total"`
	Active  int                `json:"active"`
	Expired int                `json:"expired"`
}

// lockCleanupRemovedItem — информация об удалённом lock-е.
type lockCleanupRemovedItem struct {
	FileID    string `json:"file_id"`
	Holder    string `json:"holder"`
	ExpiredAt string `json:"expired_at"`
}

// lockCleanupActiveItem — информация об активном lock-е.
type lockCleanupActiveItem struct {
	FileID    string `json:"file_id"`
	Holder    string `json:"holder"`
	ExpiresAt string `json:"expires_at"`
}

// lockCleanupResponse — ответ POST /api/v1/locks/cleanup.
type lockCleanupResponse struct {
	Cleaned   int                      `json:"cleaned"`
	Remaining int                      `json:"remaining"`
	Removed   []lockCleanupRemovedItem `json:"removed"`
	Active    []lockCleanupActiveItem  `json:"active"`
}

// ListLocks обрабатывает GET /api/v1/locks.
// Возвращает список всех lock-ов с информацией об их статусе.
func (h *LocksHandler) ListLocks(w http.ResponseWriter, _ *http.Request) {
	locks, err := h.locker.List()
	if err != nil {
		errors.InternalError(w, "не удалось получить список lock-ов: "+err.Error())
		return
	}

	now := time.Now().UTC()

	// Формируем ответ
	resp := lockListResponse{
		Locks: make([]lockInfoResponse, 0, len(locks)),
	}

	for _, l := range locks {
		expired := l.IsExpired(now)
		item := lockInfoResponse{
			FileID:     l.FileID,
			Holder:     l.Holder,
			AcquiredAt: l.AcquiredAt.Format(time.RFC3339),
			TTLSeconds: l.TTLSeconds,
			ExpiresAt:  l.ExpiresAt().Format(time.RFC3339),
			Expired:    expired,
		}
		resp.Locks = append(resp.Locks, item)

		if expired {
			resp.Expired++
		} else {
			resp.Active++
		}
	}

	resp.Total = len(resp.Locks)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// CleanupLocks обрабатывает POST /api/v1/locks/cleanup.
// Без параметров — удаляет только expired lock-и.
// С query-параметром force=true — удаляет все lock-и (аварийная очистка).
func (h *LocksHandler) CleanupLocks(w http.ResponseWriter, r *http.Request) {
	force := r.URL.Query().Get("force") == "true"

	result, err := h.locker.Cleanup(force)
	if err != nil {
		errors.InternalError(w, "ошибка очистки lock-ов: "+err.Error())
		return
	}

	// Формируем ответ
	resp := lockCleanupResponse{
		Cleaned:   result.Cleaned,
		Remaining: result.Remaining,
		Removed:   make([]lockCleanupRemovedItem, 0, len(result.Removed)),
		Active:    make([]lockCleanupActiveItem, 0, len(result.Active)),
	}

	for _, l := range result.Removed {
		resp.Removed = append(resp.Removed, lockCleanupRemovedItem{
			FileID:    l.FileID,
			Holder:    l.Holder,
			ExpiredAt: l.ExpiresAt().Format(time.RFC3339),
		})
	}

	for _, l := range result.Active {
		resp.Active = append(resp.Active, lockCleanupActiveItem{
			FileID:    l.FileID,
			Holder:    l.Holder,
			ExpiresAt: l.ExpiresAt().Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
