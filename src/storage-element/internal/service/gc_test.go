package service

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bigkaa/goartstore/storage-element/internal/domain/model"
	"github.com/bigkaa/goartstore/storage-element/internal/lockfile"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/attr"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/filestore"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/index"
)

// setupGCTestEnv создаёт тестовое окружение для GC тестов.
// Возвращает: dir, store, idx, lockMgr.
func setupGCTestEnv(t *testing.T) (string, *filestore.FileStore, *index.Index, *lockfile.LockManager) {
	t.Helper()

	dir := t.TempDir()
	store, err := filestore.New(dir)
	if err != nil {
		t.Fatalf("Ошибка создания FileStore: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	idx := index.New(logger)

	lockMgr := lockfile.NewLockManager(dir, 120*time.Second, "test-host")
	if err := lockMgr.EnsureDir(); err != nil {
		t.Fatalf("Ошибка создания директории lock-файлов: %v", err)
	}

	return dir, store, idx, lockMgr
}

// createTestFile создаёт тестовый файл и attr.json, добавляет в индекс.
// StoragePath может содержать иерархический путь YYYY/MM/DD/filename —
// промежуточные каталоги создаются автоматически.
func createTestFile(t *testing.T, dir string, meta *model.FileMetadata) {
	t.Helper()

	// Создаём промежуточные каталоги (YYYY/MM/DD/) если нужно
	filePath := filepath.Join(dir, meta.StoragePath)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o750); err != nil {
		t.Fatalf("Ошибка создания каталога для тестового файла: %v", err)
	}

	// Создаём файл данных
	if err := os.WriteFile(filePath, []byte("test data"), 0o640); err != nil {
		t.Fatalf("Ошибка создания тестового файла: %v", err)
	}

	// Создаём attr.json (attr.Write уже содержит MkdirAll)
	attrPath := attr.AttrFilePath(filePath)
	if err := attr.Write(attrPath, meta); err != nil {
		t.Fatalf("Ошибка создания attr.json: %v", err)
	}
}

func TestGCRunOnce_NoFilesToProcess(t *testing.T) {
	_, store, idx, lockMgr := setupGCTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	gc := NewGCService(store, idx, lockMgr, time.Hour, logger)
	result := gc.RunOnce()

	if result.ExpiredCount != 0 {
		t.Errorf("ExpiredCount: хотели 0, получили %d", result.ExpiredCount)
	}
	if result.DeletedCount != 0 {
		t.Errorf("DeletedCount: хотели 0, получили %d", result.DeletedCount)
	}
	if result.Errors != 0 {
		t.Errorf("Errors: хотели 0, получили %d", result.Errors)
	}
}

func TestGCRunOnce_MarkExpired(t *testing.T) {
	dir, store, idx, lockMgr := setupGCTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Создаём файл с истёкшим TTL
	expiredAt := time.Now().UTC().Add(-24 * time.Hour)
	ttlDays := 1
	meta := &model.FileMetadata{
		FileID:           "expired-1",
		OriginalFilename: "expired.txt",
		StoragePath:      "2026/01/15/expired.txt",
		ContentType:      "text/plain",
		Size:             9,
		Checksum:         "abc",
		UploadedBy:       "test",
		UploadedAt:       expiredAt.Add(-48 * time.Hour),
		Status:           model.StatusActive,
		RetentionPolicy:  model.RetentionTemporary,
		TTLDays:          &ttlDays,
		ExpiresAt:        &expiredAt,
	}

	createTestFile(t, dir, meta)
	idx.Add(meta)

	// Создаём permanent файл (не должен быть затронут)
	permanentMeta := &model.FileMetadata{
		FileID:           "permanent-1",
		OriginalFilename: "permanent.txt",
		StoragePath:      "2026/02/20/permanent.txt",
		ContentType:      "text/plain",
		Size:             9,
		Checksum:         "def",
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC(),
		Status:           model.StatusActive,
		RetentionPolicy:  model.RetentionPermanent,
	}

	createTestFile(t, dir, permanentMeta)
	idx.Add(permanentMeta)

	gc := NewGCService(store, idx, lockMgr, time.Hour, logger)
	result := gc.RunOnce()

	if result.ExpiredCount != 1 {
		t.Errorf("ExpiredCount: хотели 1, получили %d", result.ExpiredCount)
	}

	// Проверяем, что файл помечен как expired в индексе
	updatedMeta := idx.Get("expired-1")
	if updatedMeta == nil {
		t.Fatal("Файл expired-1 не найден в индексе")
	}
	if updatedMeta.Status != model.StatusExpired {
		t.Errorf("Статус: хотели %s, получили %s", model.StatusExpired, updatedMeta.Status)
	}

	// Permanent файл не затронут
	permMeta := idx.Get("permanent-1")
	if permMeta == nil {
		t.Fatal("Файл permanent-1 не найден в индексе")
	}
	if permMeta.Status != model.StatusActive {
		t.Errorf("Permanent файл изменён: хотели %s, получили %s", model.StatusActive, permMeta.Status)
	}
}

func TestGCRunOnce_DeleteFiles(t *testing.T) {
	dir, store, idx, lockMgr := setupGCTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Создаём файл со статусом deleted
	meta := &model.FileMetadata{
		FileID:           "deleted-1",
		OriginalFilename: "deleted.txt",
		StoragePath:      "2026/02/10/deleted.txt",
		ContentType:      "text/plain",
		Size:             9,
		Checksum:         "abc",
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC(),
		Status:           model.StatusDeleted,
		RetentionPolicy:  model.RetentionPermanent,
	}

	createTestFile(t, dir, meta)
	idx.Add(meta)

	gc := NewGCService(store, idx, lockMgr, time.Hour, logger)
	result := gc.RunOnce()

	if result.DeletedCount != 1 {
		t.Errorf("DeletedCount: хотели 1, получили %d", result.DeletedCount)
	}

	// Проверяем, что файл удалён из индекса
	if m := idx.Get("deleted-1"); m != nil {
		t.Errorf("Файл deleted-1 не удалён из индекса")
	}

	// Проверяем, что файл удалён с диска
	if store.FileExists("2026/02/10/deleted.txt") {
		t.Errorf("Файл deleted.txt не удалён с диска")
	}

	// Проверяем, что attr.json удалён
	attrPath := attr.AttrFilePath(filepath.Join(dir, "2026/02/10/deleted.txt"))
	if _, err := os.Stat(attrPath); !os.IsNotExist(err) {
		t.Errorf("attr.json не удалён: %s", attrPath)
	}
}

func TestGCRunOnce_DeleteSkipsLockedFile(t *testing.T) {
	dir, store, idx, lockMgr := setupGCTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Создаём файл со статусом deleted, но с активным lock
	meta := &model.FileMetadata{
		FileID:           "locked-del-1",
		OriginalFilename: "locked_del.txt",
		StoragePath:      "2026/02/10/locked_del.txt",
		ContentType:      "text/plain",
		Size:             9,
		Checksum:         "abc",
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC(),
		Status:           model.StatusDeleted,
		RetentionPolicy:  model.RetentionPermanent,
	}

	createTestFile(t, dir, meta)
	idx.Add(meta)

	// Захватываем lock для этого файла
	if err := lockMgr.Acquire("locked-del-1"); err != nil {
		t.Fatalf("Ошибка захвата lock: %v", err)
	}

	gc := NewGCService(store, idx, lockMgr, time.Hour, logger)
	result := gc.RunOnce()

	// Файл НЕ должен быть удалён (lock активен)
	if result.DeletedCount != 0 {
		t.Errorf("DeletedCount: хотели 0 (locked), получили %d", result.DeletedCount)
	}
	if result.SkippedLocked != 1 {
		t.Errorf("SkippedLocked: хотели 1, получили %d", result.SkippedLocked)
	}

	// Файл остался в индексе
	if m := idx.Get("locked-del-1"); m == nil {
		t.Error("Файл locked-del-1 удалён из индекса, но lock был активен")
	}

	// Файл остался на диске
	if !store.FileExists("2026/02/10/locked_del.txt") {
		t.Error("Файл locked_del.txt удалён с диска, но lock был активен")
	}

	// Освобождаем lock и повторяем GC
	if err := lockMgr.Release("locked-del-1"); err != nil {
		t.Fatalf("Ошибка освобождения lock: %v", err)
	}

	result2 := gc.RunOnce()
	if result2.DeletedCount != 1 {
		t.Errorf("DeletedCount после release: хотели 1, получили %d", result2.DeletedCount)
	}
}

func TestGCRunOnce_ActiveNotExpired_Untouched(t *testing.T) {
	dir, store, idx, lockMgr := setupGCTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Файл temporary, но TTL ещё не истёк
	futureExpiry := time.Now().UTC().Add(48 * time.Hour)
	ttlDays := 30
	meta := &model.FileMetadata{
		FileID:           "active-1",
		OriginalFilename: "active.txt",
		StoragePath:      "2026/03/01/active.txt",
		ContentType:      "text/plain",
		Size:             9,
		Checksum:         "abc",
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC(),
		Status:           model.StatusActive,
		RetentionPolicy:  model.RetentionTemporary,
		TTLDays:          &ttlDays,
		ExpiresAt:        &futureExpiry,
	}

	createTestFile(t, dir, meta)
	idx.Add(meta)

	gc := NewGCService(store, idx, lockMgr, time.Hour, logger)
	result := gc.RunOnce()

	if result.ExpiredCount != 0 {
		t.Errorf("ExpiredCount: хотели 0, получили %d", result.ExpiredCount)
	}

	// Файл остался active
	m := idx.Get("active-1")
	if m == nil {
		t.Fatal("Файл active-1 не найден в индексе")
	}
	if m.Status != model.StatusActive {
		t.Errorf("Статус: хотели %s, получили %s", model.StatusActive, m.Status)
	}
}

func TestGCRunOnce_CombinedExpiredAndDeleted(t *testing.T) {
	dir, store, idx, lockMgr := setupGCTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// 1. Expired файл
	expiredAt := time.Now().UTC().Add(-1 * time.Hour)
	ttlDays := 1
	expiredMeta := &model.FileMetadata{
		FileID:           "exp-1",
		OriginalFilename: "exp.txt",
		StoragePath:      "2026/01/15/exp.txt",
		ContentType:      "text/plain",
		Size:             9,
		Checksum:         "abc",
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC().Add(-48 * time.Hour),
		Status:           model.StatusActive,
		RetentionPolicy:  model.RetentionTemporary,
		TTLDays:          &ttlDays,
		ExpiresAt:        &expiredAt,
	}
	createTestFile(t, dir, expiredMeta)
	idx.Add(expiredMeta)

	// 2. Deleted файл
	deletedMeta := &model.FileMetadata{
		FileID:           "del-1",
		OriginalFilename: "del.txt",
		StoragePath:      "2026/02/10/del.txt",
		ContentType:      "text/plain",
		Size:             9,
		Checksum:         "def",
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC(),
		Status:           model.StatusDeleted,
		RetentionPolicy:  model.RetentionPermanent,
	}
	createTestFile(t, dir, deletedMeta)
	idx.Add(deletedMeta)

	// 3. Active permanent файл (не затрагивается)
	activeMeta := &model.FileMetadata{
		FileID:           "act-1",
		OriginalFilename: "active.txt",
		StoragePath:      "2026/03/01/active.txt",
		ContentType:      "text/plain",
		Size:             9,
		Checksum:         "ghi",
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC(),
		Status:           model.StatusActive,
		RetentionPolicy:  model.RetentionPermanent,
	}
	createTestFile(t, dir, activeMeta)
	idx.Add(activeMeta)

	gc := NewGCService(store, idx, lockMgr, time.Hour, logger)
	result := gc.RunOnce()

	if result.ExpiredCount != 1 {
		t.Errorf("ExpiredCount: хотели 1, получили %d", result.ExpiredCount)
	}
	if result.DeletedCount != 1 {
		t.Errorf("DeletedCount: хотели 1, получили %d", result.DeletedCount)
	}
	if result.Errors != 0 {
		t.Errorf("Errors: хотели 0, получили %d", result.Errors)
	}

	// exp-1 помечен как expired
	m := idx.Get("exp-1")
	if m == nil {
		t.Fatal("Файл exp-1 не найден в индексе")
	}
	if m.Status != model.StatusExpired {
		t.Errorf("exp-1 статус: хотели %s, получили %s", model.StatusExpired, m.Status)
	}

	// del-1 удалён
	if idx.Get("del-1") != nil {
		t.Error("Файл del-1 не удалён из индекса")
	}

	// act-1 не затронут
	m = idx.Get("act-1")
	if m == nil {
		t.Fatal("Файл act-1 не найден в индексе")
	}
	if m.Status != model.StatusActive {
		t.Errorf("act-1 статус: хотели %s, получили %s", model.StatusActive, m.Status)
	}
}

func TestGCRunOnce_DeleteMissingFile_NoError(t *testing.T) {
	_, store, idx, lockMgr := setupGCTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Файл deleted, но физически не существует на диске
	meta := &model.FileMetadata{
		FileID:           "ghost-1",
		OriginalFilename: "ghost.txt",
		StoragePath:      "2026/01/01/nonexistent.txt",
		ContentType:      "text/plain",
		Size:             100,
		Checksum:         "abc",
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC(),
		Status:           model.StatusDeleted,
		RetentionPolicy:  model.RetentionPermanent,
	}
	idx.Add(meta)

	gc := NewGCService(store, idx, lockMgr, time.Hour, logger)
	result := gc.RunOnce()

	// DeleteFile возвращает nil для несуществующих файлов, поэтому удаление успешно
	if result.DeletedCount != 1 {
		t.Errorf("DeletedCount: хотели 1, получили %d", result.DeletedCount)
	}
	if result.Errors != 0 {
		t.Errorf("Errors: хотели 0, получили %d", result.Errors)
	}
}

func TestGCRunOnce_ConcurrentSafety(t *testing.T) {
	dir, store, idx, lockMgr := setupGCTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Создаём несколько deleted файлов в иерархической структуре
	for i := 0; i < 5; i++ {
		id := "del-" + string(rune('a'+i))
		sp := fmt.Sprintf("2026/02/%02d/delfile_%s.txt", 10+i, string(rune('a'+i)))
		meta := &model.FileMetadata{
			FileID:           id,
			OriginalFilename: sp,
			StoragePath:      sp,
			ContentType:      "text/plain",
			Size:             9,
			Checksum:         "abc",
			UploadedBy:       "test",
			UploadedAt:       time.Now().UTC(),
			Status:           model.StatusDeleted,
			RetentionPolicy:  model.RetentionPermanent,
		}
		createTestFile(t, dir, meta)
		idx.Add(meta)
	}

	gc := NewGCService(store, idx, lockMgr, time.Hour, logger)

	// Запускаем RunOnce из нескольких горутин — не должно быть паники
	done := make(chan struct{}, 3)
	for i := 0; i < 3; i++ {
		go func() {
			gc.RunOnce()
			done <- struct{}{}
		}()
	}

	for i := 0; i < 3; i++ {
		<-done
	}

	// Все deleted файлы должны быть удалены
	if idx.Count() != 0 {
		t.Errorf("В индексе осталось %d файлов, ожидалось 0", idx.Count())
	}
}
