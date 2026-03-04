// locks_test.go — unit-тесты для Lock API handlers.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bigkaa/goartstore/storage-element/internal/backend"
)

// mockLockStore — мок для backend.LockStore.
type mockLockStore struct {
	locks      []backend.LockInfo
	listErr    error
	result     *backend.CleanupResult
	cleanupErr error
	// lastForce запоминает аргумент force из последнего вызова Cleanup.
	lastForce bool
}

func (m *mockLockStore) Acquire(_ context.Context, _ string) error           { return nil }
func (m *mockLockStore) Release(_ context.Context, _ string) error           { return nil }
func (m *mockLockStore) IsLocked(_ context.Context, _ string) (bool, *backend.LockInfo, error) {
	return false, nil, nil
}
func (m *mockLockStore) List(_ context.Context) ([]backend.LockInfo, error) {
	return m.locks, m.listErr
}
func (m *mockLockStore) Cleanup(_ context.Context, force bool) (*backend.CleanupResult, error) {
	m.lastForce = force
	return m.result, m.cleanupErr
}
func (m *mockLockStore) TTL() time.Duration { return 120 * time.Second }
func (m *mockLockStore) EnsureDir() error   { return nil }

// TestListLocks_Empty проверяет ответ при отсутствии lock-ов.
func TestListLocks_Empty(t *testing.T) {
	mock := &mockLockStore{locks: nil}
	h := NewLocksHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/locks", nil)
	rec := httptest.NewRecorder()

	h.ListLocks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался статус 200, получен %d", rec.Code)
	}

	var resp lockListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("ошибка декодирования ответа: %v", err)
	}

	if resp.Total != 0 {
		t.Errorf("total: ожидалось 0, получено %d", resp.Total)
	}
	if resp.Active != 0 {
		t.Errorf("active: ожидалось 0, получено %d", resp.Active)
	}
	if resp.Expired != 0 {
		t.Errorf("expired: ожидалось 0, получено %d", resp.Expired)
	}
	if len(resp.Locks) != 0 {
		t.Errorf("locks: ожидался пустой массив, получено %d элементов", len(resp.Locks))
	}
}

// TestListLocks_WithActiveLock проверяет ответ с активным lock-ом.
func TestListLocks_WithActiveLock(t *testing.T) {
	now := time.Now().UTC()
	mock := &mockLockStore{
		locks: []backend.LockInfo{
			{
				Holder:     "se-edit-1-abc",
				FileID:     "file-001",
				AcquiredAt: now,
				TTLSeconds: 300, // далеко в будущем
			},
		},
	}
	h := NewLocksHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/locks", nil)
	rec := httptest.NewRecorder()

	h.ListLocks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался статус 200, получен %d", rec.Code)
	}

	var resp lockListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("ошибка декодирования ответа: %v", err)
	}

	if resp.Total != 1 {
		t.Errorf("total: ожидалось 1, получено %d", resp.Total)
	}
	if resp.Active != 1 {
		t.Errorf("active: ожидалось 1, получено %d", resp.Active)
	}
	if resp.Expired != 0 {
		t.Errorf("expired: ожидалось 0, получено %d", resp.Expired)
	}
	if resp.Locks[0].FileID != "file-001" {
		t.Errorf("file_id: ожидалось file-001, получено %s", resp.Locks[0].FileID)
	}
	if resp.Locks[0].Expired {
		t.Error("lock не должен быть expired")
	}
}

// TestListLocks_WithExpiredLock проверяет ответ с expired lock-ом.
func TestListLocks_WithExpiredLock(t *testing.T) {
	past := time.Now().UTC().Add(-10 * time.Minute)
	mock := &mockLockStore{
		locks: []backend.LockInfo{
			{
				Holder:     "se-edit-1-dead",
				FileID:     "file-expired",
				AcquiredAt: past,
				TTLSeconds: 120, // 2 минуты, давно истекло
			},
		},
	}
	h := NewLocksHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/locks", nil)
	rec := httptest.NewRecorder()

	h.ListLocks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался статус 200, получен %d", rec.Code)
	}

	var resp lockListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("ошибка декодирования ответа: %v", err)
	}

	if resp.Active != 0 {
		t.Errorf("active: ожидалось 0, получено %d", resp.Active)
	}
	if resp.Expired != 1 {
		t.Errorf("expired: ожидалось 1, получено %d", resp.Expired)
	}
	if !resp.Locks[0].Expired {
		t.Error("lock должен быть expired")
	}
}

// TestListLocks_MixedActiveAndExpired проверяет ответ со смешанными lock-ами.
func TestListLocks_MixedActiveAndExpired(t *testing.T) {
	now := time.Now().UTC()
	mock := &mockLockStore{
		locks: []backend.LockInfo{
			{Holder: "pod-1", FileID: "active-1", AcquiredAt: now, TTLSeconds: 300},
			{Holder: "pod-dead", FileID: "expired-1", AcquiredAt: now.Add(-10 * time.Minute), TTLSeconds: 60},
			{Holder: "pod-2", FileID: "active-2", AcquiredAt: now, TTLSeconds: 120},
		},
	}
	h := NewLocksHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/locks", nil)
	rec := httptest.NewRecorder()

	h.ListLocks(rec, req)

	var resp lockListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("ошибка декодирования: %v", err)
	}

	if resp.Total != 3 {
		t.Errorf("total: ожидалось 3, получено %d", resp.Total)
	}
	if resp.Active != 2 {
		t.Errorf("active: ожидалось 2, получено %d", resp.Active)
	}
	if resp.Expired != 1 {
		t.Errorf("expired: ожидалось 1, получено %d", resp.Expired)
	}
}

// TestListLocks_Error проверяет обработку ошибки List().
func TestListLocks_Error(t *testing.T) {
	mock := &mockLockStore{
		listErr: errTest,
	}
	h := NewLocksHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/locks", nil)
	rec := httptest.NewRecorder()

	h.ListLocks(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("ожидался статус 500, получен %d", rec.Code)
	}
}

// TestCleanupLocks_ExpiredOnly проверяет очистку только expired lock-ов.
func TestCleanupLocks_ExpiredOnly(t *testing.T) {
	now := time.Now().UTC()
	mock := &mockLockStore{
		result: &backend.CleanupResult{
			Cleaned:   2,
			Remaining: 1,
			Removed: []backend.LockInfo{
				{FileID: "exp-1", Holder: "dead-pod", AcquiredAt: now.Add(-10 * time.Minute), TTLSeconds: 60},
				{FileID: "exp-2", Holder: "dead-pod", AcquiredAt: now.Add(-5 * time.Minute), TTLSeconds: 60},
			},
			Active: []backend.LockInfo{
				{FileID: "active-1", Holder: "live-pod", AcquiredAt: now, TTLSeconds: 300},
			},
		},
	}
	h := NewLocksHandler(mock)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/locks/cleanup", nil)
	rec := httptest.NewRecorder()

	h.CleanupLocks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался статус 200, получен %d", rec.Code)
	}

	// Проверяем, что force не передан
	if mock.lastForce {
		t.Error("force должен быть false при запросе без параметра")
	}

	var resp lockCleanupResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("ошибка декодирования: %v", err)
	}

	if resp.Cleaned != 2 {
		t.Errorf("cleaned: ожидалось 2, получено %d", resp.Cleaned)
	}
	if resp.Remaining != 1 {
		t.Errorf("remaining: ожидалось 1, получено %d", resp.Remaining)
	}
	if len(resp.Removed) != 2 {
		t.Errorf("removed: ожидалось 2 элемента, получено %d", len(resp.Removed))
	}
	if len(resp.Active) != 1 {
		t.Errorf("active: ожидалось 1 элемент, получено %d", len(resp.Active))
	}
}

// TestCleanupLocks_ForceTrue проверяет принудительную очистку всех lock-ов.
func TestCleanupLocks_ForceTrue(t *testing.T) {
	mock := &mockLockStore{
		result: &backend.CleanupResult{
			Cleaned:   3,
			Remaining: 0,
			Removed: []backend.LockInfo{
				{FileID: "f1", Holder: "p1", AcquiredAt: time.Now().UTC(), TTLSeconds: 60},
				{FileID: "f2", Holder: "p2", AcquiredAt: time.Now().UTC(), TTLSeconds: 120},
				{FileID: "f3", Holder: "p3", AcquiredAt: time.Now().UTC(), TTLSeconds: 300},
			},
			Active: nil,
		},
	}
	h := NewLocksHandler(mock)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/locks/cleanup?force=true", nil)
	rec := httptest.NewRecorder()

	h.CleanupLocks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался статус 200, получен %d", rec.Code)
	}

	if !mock.lastForce {
		t.Error("force должен быть true при запросе с force=true")
	}

	var resp lockCleanupResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("ошибка декодирования: %v", err)
	}

	if resp.Cleaned != 3 {
		t.Errorf("cleaned: ожидалось 3, получено %d", resp.Cleaned)
	}
	if resp.Remaining != 0 {
		t.Errorf("remaining: ожидалось 0, получено %d", resp.Remaining)
	}
}

// TestCleanupLocks_Error проверяет обработку ошибки Cleanup().
func TestCleanupLocks_Error(t *testing.T) {
	mock := &mockLockStore{
		cleanupErr: errTest,
	}
	h := NewLocksHandler(mock)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/locks/cleanup", nil)
	rec := httptest.NewRecorder()

	h.CleanupLocks(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("ожидался статус 500, получен %d", rec.Code)
	}
}

// TestCleanupLocks_EmptyResult проверяет ответ при отсутствии lock-ов для очистки.
func TestCleanupLocks_EmptyResult(t *testing.T) {
	mock := &mockLockStore{
		result: &backend.CleanupResult{
			Cleaned:   0,
			Remaining: 0,
			Removed:   nil,
			Active:    nil,
		},
	}
	h := NewLocksHandler(mock)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/locks/cleanup", nil)
	rec := httptest.NewRecorder()

	h.CleanupLocks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался статус 200, получен %d", rec.Code)
	}

	var resp lockCleanupResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("ошибка декодирования: %v", err)
	}

	if resp.Cleaned != 0 {
		t.Errorf("cleaned: ожидалось 0, получено %d", resp.Cleaned)
	}
	if len(resp.Removed) != 0 {
		t.Errorf("removed: ожидался пустой массив, получено %d", len(resp.Removed))
	}
	if len(resp.Active) != 0 {
		t.Errorf("active: ожидался пустой массив, получено %d", len(resp.Active))
	}
}

// errTest — тестовая ошибка для моков.
var errTest = fmt.Errorf("test error")
