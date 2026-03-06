package service

import (
	"context"
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
// Возвращает: dir, store, attrStore, idx, lockMgr.
func setupGCTestEnv(t *testing.T) (string, *filestore.FileStore, *attr.Store, *index.Index, *lockfile.LockManager) {
	t.Helper()

	dir := t.TempDir()
	store, err := filestore.New(dir)
	if err != nil {
		t.Fatalf("Ошибка создания FileStore: %v", err)
	}

	attrStore := attr.NewStore(dir)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	idx := index.New(logger)

	lockMgr := lockfile.NewLockManager(dir, 120*time.Second, "test-host")
	if err := lockMgr.EnsureDir(); err != nil {
		t.Fatalf("Ошибка создания директории lock-файлов: %v", err)
	}

	return dir, store, attrStore, idx, lockMgr
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
	_, store, attrStore, idx, lockMgr := setupGCTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	gc := NewGCService(store, attrStore, idx, lockMgr, time.Hour, logger)
	result := gc.RunOnce()

	if result.DeletedCount != 0 {
		t.Errorf("DeletedCount: хотели 0, получили %d", result.DeletedCount)
	}
	if result.Errors != 0 {
		t.Errorf("Errors: хотели 0, получили %d", result.Errors)
	}
}

func TestGCRunOnce_DeleteExpiredTemporary(t *testing.T) {
	dir, store, attrStore, idx, lockMgr := setupGCTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Создаём temporary файл с истёкшим TTL
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
		UploadedAt:       time.Now().UTC().Add(-48 * time.Hour),
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
		RetentionPolicy:  model.RetentionPermanent,
	}

	createTestFile(t, dir, permanentMeta)
	idx.Add(permanentMeta)

	gc := NewGCService(store, attrStore, idx, lockMgr, time.Hour, logger)
	result := gc.RunOnce()

	if result.DeletedCount != 1 {
		t.Errorf("DeletedCount: хотели 1, получили %d", result.DeletedCount)
	}

	// Проверяем, что expired файл удалён из индекса
	if idx.Get("expired-1") != nil {
		t.Error("Файл expired-1 не удалён из индекса после GC")
	}

	// Permanent файл не затронут
	permMeta := idx.Get("permanent-1")
	if permMeta == nil {
		t.Fatal("Файл permanent-1 не найден в индексе")
	}
}

func TestGCRunOnce_ExpiredTTL_PhysicalDelete(t *testing.T) {
	dir, store, attrStore, idx, lockMgr := setupGCTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx := context.Background()

	// Создаём temporary файл с истёкшим TTL
	expiredAt := time.Now().UTC().Add(-24 * time.Hour)
	ttlDays := 1
	meta := &model.FileMetadata{
		FileID:           "exp-phys-1",
		OriginalFilename: "exp_phys.txt",
		StoragePath:      "2026/02/10/exp_phys.txt",
		ContentType:      "text/plain",
		Size:             9,
		Checksum:         "abc",
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC().Add(-48 * time.Hour),
		RetentionPolicy:  model.RetentionTemporary,
		TTLDays:          &ttlDays,
		ExpiresAt:        &expiredAt,
	}

	createTestFile(t, dir, meta)
	idx.Add(meta)

	gc := NewGCService(store, attrStore, idx, lockMgr, time.Hour, logger)
	result := gc.RunOnce()

	if result.DeletedCount != 1 {
		t.Errorf("DeletedCount: хотели 1, получили %d", result.DeletedCount)
	}

	// Проверяем, что файл удалён из индекса
	if m := idx.Get("exp-phys-1"); m != nil {
		t.Errorf("Файл exp-phys-1 не удалён из индекса")
	}

	// Проверяем, что файл удалён с диска
	exists, err := store.FileExists(ctx, "2026/02/10/exp_phys.txt")
	if err != nil {
		t.Fatalf("Ошибка проверки существования: %v", err)
	}
	if exists {
		t.Errorf("Файл exp_phys.txt не удалён с диска")
	}

	// Проверяем, что attr.json удалён
	attrPath := attr.AttrFilePath(filepath.Join(dir, "2026/02/10/exp_phys.txt"))
	if _, statErr := os.Stat(attrPath); !os.IsNotExist(statErr) {
		t.Errorf("attr.json не удалён: %s", attrPath)
	}
}

func TestGCRunOnce_DeleteSkipsLockedFile(t *testing.T) {
	dir, store, attrStore, idx, lockMgr := setupGCTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx := context.Background()

	// Создаём temporary файл с истёкшим TTL, но с активным lock
	expiredAt := time.Now().UTC().Add(-24 * time.Hour)
	ttlDays := 1
	meta := &model.FileMetadata{
		FileID:           "locked-exp-1",
		OriginalFilename: "locked_exp.txt",
		StoragePath:      "2026/02/10/locked_exp.txt",
		ContentType:      "text/plain",
		Size:             9,
		Checksum:         "abc",
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC().Add(-48 * time.Hour),
		RetentionPolicy:  model.RetentionTemporary,
		TTLDays:          &ttlDays,
		ExpiresAt:        &expiredAt,
	}

	createTestFile(t, dir, meta)
	idx.Add(meta)

	// Захватываем lock для этого файла
	if err := lockMgr.Acquire(ctx, "locked-exp-1"); err != nil {
		t.Fatalf("Ошибка захвата lock: %v", err)
	}

	gc := NewGCService(store, attrStore, idx, lockMgr, time.Hour, logger)
	result := gc.RunOnce()

	// Файл НЕ должен быть удалён (lock активен)
	if result.DeletedCount != 0 {
		t.Errorf("DeletedCount: хотели 0 (locked), получили %d", result.DeletedCount)
	}
	if result.SkippedLocked != 1 {
		t.Errorf("SkippedLocked: хотели 1, получили %d", result.SkippedLocked)
	}

	// Файл остался в индексе
	if m := idx.Get("locked-exp-1"); m == nil {
		t.Error("Файл locked-exp-1 удалён из индекса, но lock был активен")
	}

	// Файл остался на диске
	exists, err := store.FileExists(ctx, "2026/02/10/locked_exp.txt")
	if err != nil {
		t.Fatalf("Ошибка проверки существования: %v", err)
	}
	if !exists {
		t.Error("Файл locked_exp.txt удалён с диска, но lock был активен")
	}

	// Освобождаем lock и повторяем GC
	if err := lockMgr.Release(ctx, "locked-exp-1"); err != nil {
		t.Fatalf("Ошибка освобождения lock: %v", err)
	}

	result2 := gc.RunOnce()
	if result2.DeletedCount != 1 {
		t.Errorf("DeletedCount после release: хотели 1, получили %d", result2.DeletedCount)
	}
}

func TestGCRunOnce_TemporaryNotExpired_Untouched(t *testing.T) {
	dir, store, attrStore, idx, lockMgr := setupGCTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Файл temporary, но TTL ещё не истёк
	futureExpiry := time.Now().UTC().Add(48 * time.Hour)
	ttlDays := 30
	meta := &model.FileMetadata{
		FileID:           "temp-fresh-1",
		OriginalFilename: "temp_fresh.txt",
		StoragePath:      "2026/03/01/temp_fresh.txt",
		ContentType:      "text/plain",
		Size:             9,
		Checksum:         "abc",
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC(),
		RetentionPolicy:  model.RetentionTemporary,
		TTLDays:          &ttlDays,
		ExpiresAt:        &futureExpiry,
	}

	createTestFile(t, dir, meta)
	idx.Add(meta)

	gc := NewGCService(store, attrStore, idx, lockMgr, time.Hour, logger)
	result := gc.RunOnce()

	if result.DeletedCount != 0 {
		t.Errorf("DeletedCount: хотели 0, получили %d", result.DeletedCount)
	}

	// Файл остался в индексе
	m := idx.Get("temp-fresh-1")
	if m == nil {
		t.Fatal("Файл temp-fresh-1 не найден в индексе")
	}
}

func TestGCRunOnce_MultipleExpiredFiles(t *testing.T) {
	dir, store, attrStore, idx, lockMgr := setupGCTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	expiredAt := time.Now().UTC().Add(-1 * time.Hour)
	ttlDays := 1

	// 1. Первый temporary файл с истёкшим TTL
	expMeta1 := &model.FileMetadata{
		FileID:           "exp-1",
		OriginalFilename: "exp1.txt",
		StoragePath:      "2026/01/15/exp1.txt",
		ContentType:      "text/plain",
		Size:             9,
		Checksum:         "abc",
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC().Add(-48 * time.Hour),
		RetentionPolicy:  model.RetentionTemporary,
		TTLDays:          &ttlDays,
		ExpiresAt:        &expiredAt,
	}
	createTestFile(t, dir, expMeta1)
	idx.Add(expMeta1)

	// 2. Второй temporary файл с истёкшим TTL
	expMeta2 := &model.FileMetadata{
		FileID:           "exp-2",
		OriginalFilename: "exp2.txt",
		StoragePath:      "2026/02/10/exp2.txt",
		ContentType:      "text/plain",
		Size:             9,
		Checksum:         "def",
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC().Add(-48 * time.Hour),
		RetentionPolicy:  model.RetentionTemporary,
		TTLDays:          &ttlDays,
		ExpiresAt:        &expiredAt,
	}
	createTestFile(t, dir, expMeta2)
	idx.Add(expMeta2)

	// 3. Permanent файл (не затрагивается GC)
	permMeta := &model.FileMetadata{
		FileID:           "perm-1",
		OriginalFilename: "permanent.txt",
		StoragePath:      "2026/03/01/permanent.txt",
		ContentType:      "text/plain",
		Size:             9,
		Checksum:         "ghi",
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC(),
		RetentionPolicy:  model.RetentionPermanent,
	}
	createTestFile(t, dir, permMeta)
	idx.Add(permMeta)

	gc := NewGCService(store, attrStore, idx, lockMgr, time.Hour, logger)
	result := gc.RunOnce()

	if result.DeletedCount != 2 {
		t.Errorf("DeletedCount: хотели 2, получили %d", result.DeletedCount)
	}
	if result.Errors != 0 {
		t.Errorf("Errors: хотели 0, получили %d", result.Errors)
	}

	// Оба expired файла удалены из индекса
	if idx.Get("exp-1") != nil {
		t.Error("Файл exp-1 не удалён из индекса")
	}
	if idx.Get("exp-2") != nil {
		t.Error("Файл exp-2 не удалён из индекса")
	}

	// Permanent файл не затронут
	m := idx.Get("perm-1")
	if m == nil {
		t.Fatal("Файл perm-1 не найден в индексе")
	}
}

func TestGCRunOnce_DeleteMissingFile_NoError(t *testing.T) {
	_, store, attrStore, idx, lockMgr := setupGCTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Temporary файл с истёкшим TTL, но физически не существует на диске
	expiredAt := time.Now().UTC().Add(-24 * time.Hour)
	ttlDays := 1
	meta := &model.FileMetadata{
		FileID:           "ghost-1",
		OriginalFilename: "ghost.txt",
		StoragePath:      "2026/01/01/nonexistent.txt",
		ContentType:      "text/plain",
		Size:             100,
		Checksum:         "abc",
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC().Add(-48 * time.Hour),
		RetentionPolicy:  model.RetentionTemporary,
		TTLDays:          &ttlDays,
		ExpiresAt:        &expiredAt,
	}
	idx.Add(meta)

	gc := NewGCService(store, attrStore, idx, lockMgr, time.Hour, logger)
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
	dir, store, attrStore, idx, lockMgr := setupGCTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Создаём несколько temporary файлов с истёкшим TTL
	expiredAt := time.Now().UTC().Add(-24 * time.Hour)
	ttlDays := 1
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("exp-%c", rune('a'+i))
		sp := fmt.Sprintf("2026/02/%02d/expfile_%c.txt", 10+i, rune('a'+i))
		meta := &model.FileMetadata{
			FileID:           id,
			OriginalFilename: sp,
			StoragePath:      sp,
			ContentType:      "text/plain",
			Size:             9,
			Checksum:         "abc",
			UploadedBy:       "test",
			UploadedAt:       time.Now().UTC().Add(-48 * time.Hour),
			RetentionPolicy:  model.RetentionTemporary,
			TTLDays:          &ttlDays,
			ExpiresAt:        &expiredAt,
		}
		createTestFile(t, dir, meta)
		idx.Add(meta)
	}

	gc := NewGCService(store, attrStore, idx, lockMgr, time.Hour, logger)

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

	// Все expired temporary файлы должны быть удалены
	if idx.Count() != 0 {
		t.Errorf("В индексе осталось %d файлов, ожидалось 0", idx.Count())
	}
}
