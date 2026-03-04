package lockfile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bigkaa/goartstore/storage-element/internal/backend"
)

// ctx — общий контекст для тестов.
var ctx = context.Background()

// setupTestLockManager создаёт LockManager для тестов с temp-директорией.
func setupTestLockManager(t *testing.T, ttl time.Duration) (*LockManager, string) {
	t.Helper()
	dataDir := t.TempDir()
	lm := NewLockManager(dataDir, ttl, "test-pod-abc123")
	if err := lm.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	return lm, dataDir
}

func TestEnsureDir(t *testing.T) {
	dataDir := t.TempDir()
	lm := NewLockManager(dataDir, 120*time.Second, "test-pod")

	// Первый вызов — создаёт директорию
	if err := lm.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	// Проверяем что директория существует
	info, err := os.Stat(filepath.Join(dataDir, lockDir))
	if err != nil {
		t.Fatalf("директория .locks не создана: %v", err)
	}
	if !info.IsDir() {
		t.Fatal(".locks не является директорией")
	}

	// Повторный вызов — идемпотентен
	if err := lm.EnsureDir(); err != nil {
		t.Fatalf("повторный EnsureDir: %v", err)
	}
}

func TestAcquireAndRelease(t *testing.T) {
	lm, _ := setupTestLockManager(t, 120*time.Second)

	// Acquire
	if err := lm.Acquire(ctx, "file-001"); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// Проверяем lock-файл существует
	lockPath := lm.lockPath("file-001")
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock-файл не найден: %v", err)
	}

	// IsLocked — должен быть locked
	locked, info, err := lm.IsLocked(ctx, "file-001")
	if err != nil {
		t.Fatalf("IsLocked: %v", err)
	}
	if !locked {
		t.Fatal("ожидалось locked=true")
	}
	if info == nil {
		t.Fatal("ожидалась информация о lock")
	}
	if info.Holder != "test-pod-abc123" {
		t.Errorf("holder: ожидалось 'test-pod-abc123', получено %q", info.Holder)
	}
	if info.FileID != "file-001" {
		t.Errorf("file_id: ожидалось 'file-001', получено %q", info.FileID)
	}
	if info.TTLSeconds != 120 {
		t.Errorf("ttl_seconds: ожидалось 120, получено %d", info.TTLSeconds)
	}

	// Release
	if err := lm.Release(ctx, "file-001"); err != nil {
		t.Fatalf("Release: %v", err)
	}

	// Проверяем lock-файл удалён
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatal("lock-файл не удалён после Release")
	}

	// IsLocked — не locked
	locked, info, err = lm.IsLocked(ctx, "file-001")
	if err != nil {
		t.Fatalf("IsLocked после Release: %v", err)
	}
	if locked {
		t.Fatal("ожидалось locked=false после Release")
	}
	if info != nil {
		t.Fatal("ожидалось info=nil после Release")
	}
}

func TestReleaseIdempotent(t *testing.T) {
	lm, _ := setupTestLockManager(t, 120*time.Second)

	// Release несуществующего lock — не ошибка
	if err := lm.Release(ctx, "nonexistent-file"); err != nil {
		t.Fatalf("Release несуществующего: %v", err)
	}

	// Double release — не ошибка
	if err := lm.Acquire(ctx, "file-002"); err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := lm.Release(ctx, "file-002"); err != nil {
		t.Fatalf("первый Release: %v", err)
	}
	if err := lm.Release(ctx, "file-002"); err != nil {
		t.Fatalf("второй Release: %v", err)
	}
}

func TestIsLockedExpired(t *testing.T) {
	// TTL = 1 наносекунда — lock мгновенно истекает
	lm, _ := setupTestLockManager(t, time.Nanosecond)

	if err := lm.Acquire(ctx, "file-003"); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// Ждём чтобы TTL гарантированно истёк
	time.Sleep(time.Millisecond)

	locked, info, err := lm.IsLocked(ctx, "file-003")
	if err != nil {
		t.Fatalf("IsLocked: %v", err)
	}
	if locked {
		t.Fatal("ожидалось locked=false (TTL истёк)")
	}
	if info == nil {
		t.Fatal("ожидалась info с данными lock-а (expired)")
	}
	if info.FileID != "file-003" {
		t.Errorf("file_id: ожидалось 'file-003', получено %q", info.FileID)
	}
}

func TestList(t *testing.T) {
	lm, _ := setupTestLockManager(t, 120*time.Second)

	// Пустой список
	locks, err := lm.List(ctx)
	if err != nil {
		t.Fatalf("List (пустой): %v", err)
	}
	if len(locks) != 0 {
		t.Fatalf("ожидался пустой список, получено %d", len(locks))
	}

	// Добавляем lock-и
	for _, id := range []string{"file-a", "file-b", "file-c"} {
		if err := lm.Acquire(ctx, id); err != nil {
			t.Fatalf("Acquire(%s): %v", id, err)
		}
	}

	locks, err = lm.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(locks) != 3 {
		t.Fatalf("ожидалось 3 lock-а, получено %d", len(locks))
	}

	// Проверяем что все file_id присутствуют
	ids := make(map[string]bool)
	for _, l := range locks {
		ids[l.FileID] = true
	}
	for _, expected := range []string{"file-a", "file-b", "file-c"} {
		if !ids[expected] {
			t.Errorf("lock для %s не найден в списке", expected)
		}
	}
}

func TestCleanup(t *testing.T) {
	// TTL = 1 наносекунда — все lock-и мгновенно истекают
	lm, _ := setupTestLockManager(t, time.Nanosecond)

	// Создаём expired lock-и
	for _, id := range []string{"file-x", "file-y"} {
		if err := lm.Acquire(ctx, id); err != nil {
			t.Fatalf("Acquire(%s): %v", id, err)
		}
	}

	time.Sleep(time.Millisecond)

	result, err := lm.Cleanup(ctx, false)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if result.Cleaned != 2 {
		t.Errorf("cleaned: ожидалось 2, получено %d", result.Cleaned)
	}
	if result.Remaining != 0 {
		t.Errorf("remaining: ожидалось 0, получено %d", result.Remaining)
	}

	// Проверяем что lock-файлы удалены
	locks, _ := lm.List(ctx)
	if len(locks) != 0 {
		t.Errorf("после cleanup: ожидалось 0 lock-ов, получено %d", len(locks))
	}
}

func TestCleanupMixed(t *testing.T) {
	lm, _ := setupTestLockManager(t, 120*time.Second)

	// Создаём активный lock
	if err := lm.Acquire(ctx, "active-file"); err != nil {
		t.Fatalf("Acquire active: %v", err)
	}

	// Создаём expired lock вручную (записываем файл с прошлым acquired_at)
	expiredInfo := backend.LockInfo{
		Holder:     "dead-pod",
		FileID:     "expired-file",
		AcquiredAt: time.Now().UTC().Add(-24 * time.Hour),
		TTLSeconds: 120,
	}
	if err := writeTestLock(lm.lockPath("expired-file"), expiredInfo); err != nil {
		t.Fatalf("создание expired lock: %v", err)
	}

	result, err := lm.Cleanup(ctx, false)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	if result.Cleaned != 1 {
		t.Errorf("cleaned: ожидалось 1, получено %d", result.Cleaned)
	}
	if result.Remaining != 1 {
		t.Errorf("remaining: ожидалось 1, получено %d", result.Remaining)
	}

	// Активный lock должен остаться
	locked, _, err := lm.IsLocked(ctx, "active-file")
	if err != nil {
		t.Fatalf("IsLocked active: %v", err)
	}
	if !locked {
		t.Fatal("активный lock должен остаться после cleanup")
	}

	// Expired lock должен быть удалён
	locked, _, err = lm.IsLocked(ctx, "expired-file")
	if err != nil {
		t.Fatalf("IsLocked expired: %v", err)
	}
	if locked {
		t.Fatal("expired lock не должен быть locked после cleanup")
	}
}

func TestCleanupForce(t *testing.T) {
	lm, _ := setupTestLockManager(t, 120*time.Second)

	// Создаём активный lock
	if err := lm.Acquire(ctx, "force-file"); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// Force cleanup удаляет даже активные
	result, err := lm.Cleanup(ctx, true)
	if err != nil {
		t.Fatalf("Cleanup force: %v", err)
	}
	if result.Cleaned != 1 {
		t.Errorf("cleaned: ожидалось 1, получено %d", result.Cleaned)
	}

	locks, _ := lm.List(ctx)
	if len(locks) != 0 {
		t.Errorf("после force cleanup: ожидалось 0 lock-ов, получено %d", len(locks))
	}
}

func TestLockInfoExpiresAt(t *testing.T) {
	acquired := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	info := backend.LockInfo{
		AcquiredAt: acquired,
		TTLSeconds: 120,
	}

	expected := time.Date(2026, 3, 1, 12, 2, 0, 0, time.UTC)
	if !info.ExpiresAt().Equal(expected) {
		t.Errorf("ExpiresAt: ожидалось %v, получено %v", expected, info.ExpiresAt())
	}
}

func TestLockInfoIsExpired(t *testing.T) {
	acquired := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	info := backend.LockInfo{
		AcquiredAt: acquired,
		TTLSeconds: 120,
	}

	// Не истёк
	beforeExpiry := time.Date(2026, 3, 1, 12, 1, 0, 0, time.UTC)
	if info.IsExpired(beforeExpiry) {
		t.Error("ожидалось not expired в 12:01")
	}

	// Истёк
	afterExpiry := time.Date(2026, 3, 1, 12, 3, 0, 0, time.UTC)
	if !info.IsExpired(afterExpiry) {
		t.Error("ожидалось expired в 12:03")
	}
}

// writeTestLock записывает lock-файл для тестов.
func writeTestLock(path string, info backend.LockInfo) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
