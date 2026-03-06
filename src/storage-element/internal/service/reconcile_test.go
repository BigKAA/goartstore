package service

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bigkaa/goartstore/storage-element/internal/api/generated"
	"github.com/bigkaa/goartstore/storage-element/internal/domain/model"
	"github.com/bigkaa/goartstore/storage-element/internal/lockfile"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/attr"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/filestore"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/index"
)

// ctx — общий контекст для тестов.
var ctx = context.Background()

// setupReconcileTestEnv создаёт тестовое окружение для reconciliation тестов.
// Возвращает: dir, store, attrStore, idx, lockMgr.
func setupReconcileTestEnv(t *testing.T) (string, *filestore.FileStore, *attr.Store, *index.Index, *lockfile.LockManager) {
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

func TestReconcileRunOnce_NoIssues(t *testing.T) {
	dir, store, attrStore, idx, lockMgr := setupReconcileTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Создаём корректную пару файл + attr.json в иерархической структуре
	storagePath := "2026/03/01/good.txt"
	meta := &model.FileMetadata{
		FileID:           "good-1",
		OriginalFilename: "good.txt",
		StoragePath:      storagePath,
		ContentType:      "text/plain",
		Size:             9,
		Checksum:         "", // Будет вычислен ниже
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC(),
		RetentionPolicy:  model.RetentionPermanent,
	}

	// Создаём каталог и записываем файл данных
	filePath := filepath.Join(dir, storagePath)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o750); err != nil {
		t.Fatalf("Ошибка создания каталога: %v", err)
	}
	content := []byte("test data")
	if err := os.WriteFile(filePath, content, 0o640); err != nil {
		t.Fatalf("Ошибка записи файла: %v", err)
	}

	// Вычисляем checksum
	checksum, err := store.ComputeChecksum(ctx, storagePath)
	if err != nil {
		t.Fatalf("Ошибка вычисления checksum: %v", err)
	}
	meta.Checksum = checksum

	// Записываем attr.json
	attrPath := attr.AttrFilePath(filePath)
	if err := attr.Write(attrPath, meta); err != nil {
		t.Fatalf("Ошибка записи attr.json: %v", err)
	}

	// Строим индекс
	if err := idx.BuildFromDir(dir); err != nil {
		t.Fatalf("Ошибка построения индекса: %v", err)
	}

	rs := NewReconcileService(store, attrStore, idx, lockMgr, time.Hour, logger)
	result := rs.RunOnce()

	if result == nil {
		t.Fatal("Результат nil")
	}
	if len(result.Issues) != 0 {
		t.Errorf("Найдено %d проблем, ожидалось 0", len(result.Issues))
		for _, issue := range result.Issues {
			t.Logf("  %s: %s (path=%v)", issue.Type, issue.Description, issue.Path)
		}
	}
	if result.Summary.Ok != 1 {
		t.Errorf("Ok: хотели 1, получили %d", result.Summary.Ok)
	}
}

func TestReconcileRunOnce_OrphanedFile(t *testing.T) {
	dir, store, attrStore, idx, lockMgr := setupReconcileTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Файл на диске без attr.json в иерархической структуре
	orphanedRelPath := "2026/02/20/orphaned.txt"
	filePath := filepath.Join(dir, orphanedRelPath)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o750); err != nil {
		t.Fatalf("Ошибка создания каталога: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("data"), 0o640); err != nil {
		t.Fatalf("Ошибка создания файла: %v", err)
	}

	// Устанавливаем mtime в прошлое (за пределами lockTTL), чтобы orphaned обнаружился
	oldTime := time.Now().Add(-5 * time.Minute)
	if err := os.Chtimes(filePath, oldTime, oldTime); err != nil {
		t.Fatalf("Ошибка установки mtime: %v", err)
	}

	if err := idx.BuildFromDir(dir); err != nil {
		t.Fatalf("Ошибка построения индекса: %v", err)
	}

	rs := NewReconcileService(store, attrStore, idx, lockMgr, time.Hour, logger)
	result := rs.RunOnce()

	if result == nil {
		t.Fatal("Результат nil")
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Type == generated.OrphanedFile && issue.Path != nil && *issue.Path == orphanedRelPath {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Не обнаружен orphaned_file для %s", orphanedRelPath)
	}
	if result.Summary.OrphanedFiles != 1 {
		t.Errorf("OrphanedFiles: хотели 1, получили %d", result.Summary.OrphanedFiles)
	}
}

func TestReconcileRunOnce_OrphanedFileSkippedWhenFresh(t *testing.T) {
	dir, store, attrStore, idx, lockMgr := setupReconcileTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Файл на диске без attr.json — свежий (mtime в пределах lockTTL)
	freshRelPath := "2026/03/01/fresh_orphan.txt"
	filePath := filepath.Join(dir, freshRelPath)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o750); err != nil {
		t.Fatalf("Ошибка создания каталога: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("data"), 0o640); err != nil {
		t.Fatalf("Ошибка создания файла: %v", err)
	}
	// mtime = now (по умолчанию), lockTTL = 120s — файл свежий

	if err := idx.BuildFromDir(dir); err != nil {
		t.Fatalf("Ошибка построения индекса: %v", err)
	}

	rs := NewReconcileService(store, attrStore, idx, lockMgr, time.Hour, logger)
	result := rs.RunOnce()

	if result == nil {
		t.Fatal("Результат nil")
	}

	// Свежий orphaned файл НЕ должен быть обнаружен (возможный in-flight upload)
	for _, issue := range result.Issues {
		if issue.Type == generated.OrphanedFile && issue.Path != nil && *issue.Path == freshRelPath {
			t.Error("Свежий orphaned файл не должен был быть обнаружен (in-flight upload)")
		}
	}
}

func TestReconcileRunOnce_MissingFile(t *testing.T) {
	dir, store, attrStore, idx, lockMgr := setupReconcileTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// attr.json без файла данных в иерархической структуре
	missingRelPath := "2026/02/21/missing.txt"
	meta := &model.FileMetadata{
		FileID:           "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		OriginalFilename: "missing.txt",
		StoragePath:      missingRelPath,
		ContentType:      "text/plain",
		Size:             100,
		Checksum:         "abc123",
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC(),
		RetentionPolicy:  model.RetentionPermanent,
	}

	attrPath := filepath.Join(dir, missingRelPath+attr.AttrSuffix)
	if err := attr.Write(attrPath, meta); err != nil {
		t.Fatalf("Ошибка записи attr.json: %v", err)
	}

	// Устанавливаем mtime attr.json в прошлое
	oldTime := time.Now().Add(-5 * time.Minute)
	if err := os.Chtimes(attrPath, oldTime, oldTime); err != nil {
		t.Fatalf("Ошибка установки mtime: %v", err)
	}

	if err := idx.BuildFromDir(dir); err != nil {
		t.Fatalf("Ошибка построения индекса: %v", err)
	}

	rs := NewReconcileService(store, attrStore, idx, lockMgr, time.Hour, logger)
	result := rs.RunOnce()

	if result == nil {
		t.Fatal("Результат nil")
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Type == generated.MissingFile && issue.Path != nil && *issue.Path == missingRelPath {
			found = true
			// Проверяем, что file_id заполнен
			if issue.FileId == nil {
				t.Error("FileId nil для missing_file")
			}
			break
		}
	}
	if !found {
		t.Errorf("Не обнаружен missing_file для %s", missingRelPath)
	}
	if result.Summary.MissingFiles != 1 {
		t.Errorf("MissingFiles: хотели 1, получили %d", result.Summary.MissingFiles)
	}
}

func TestReconcileRunOnce_SizeMismatch(t *testing.T) {
	dir, store, attrStore, idx, lockMgr := setupReconcileTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Файл с неправильным размером в attr.json
	sizeRelPath := "2026/02/20/size_mismatch.txt"
	filePath := filepath.Join(dir, sizeRelPath)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o750); err != nil {
		t.Fatalf("Ошибка создания каталога: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("actual data"), 0o640); err != nil {
		t.Fatalf("Ошибка создания файла: %v", err)
	}

	meta := &model.FileMetadata{
		FileID:           "size-1",
		OriginalFilename: "size_mismatch.txt",
		StoragePath:      sizeRelPath,
		ContentType:      "text/plain",
		Size:             999, // Неправильный размер
		Checksum:         "abc",
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC(),
		RetentionPolicy:  model.RetentionPermanent,
	}

	attrPath := attr.AttrFilePath(filePath)
	if err := attr.Write(attrPath, meta); err != nil {
		t.Fatalf("Ошибка записи attr.json: %v", err)
	}

	if err := idx.BuildFromDir(dir); err != nil {
		t.Fatalf("Ошибка построения индекса: %v", err)
	}

	rs := NewReconcileService(store, attrStore, idx, lockMgr, time.Hour, logger)
	result := rs.RunOnce()

	if result == nil {
		t.Fatal("Результат nil")
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Type == generated.SizeMismatch {
			found = true
			break
		}
	}
	if !found {
		t.Error("Не обнаружен size_mismatch")
	}
	if result.Summary.SizeMismatches != 1 {
		t.Errorf("SizeMismatches: хотели 1, получили %d", result.Summary.SizeMismatches)
	}
}

func TestReconcileRunOnce_ChecksumMismatch(t *testing.T) {
	dir, store, attrStore, idx, lockMgr := setupReconcileTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Файл с неправильным checksum в attr.json
	csRelPath := "2026/02/20/cs_mismatch.txt"
	filePath := filepath.Join(dir, csRelPath)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o750); err != nil {
		t.Fatalf("Ошибка создания каталога: %v", err)
	}
	content := []byte("actual data")
	if err := os.WriteFile(filePath, content, 0o640); err != nil {
		t.Fatalf("Ошибка создания файла: %v", err)
	}

	meta := &model.FileMetadata{
		FileID:           "cs-1",
		OriginalFilename: "cs_mismatch.txt",
		StoragePath:      csRelPath,
		ContentType:      "text/plain",
		Size:             int64(len(content)), // Правильный размер
		Checksum:         "deadbeef",          // Неправильный checksum
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC(),
		RetentionPolicy:  model.RetentionPermanent,
	}

	attrPath := attr.AttrFilePath(filePath)
	if err := attr.Write(attrPath, meta); err != nil {
		t.Fatalf("Ошибка записи attr.json: %v", err)
	}

	if err := idx.BuildFromDir(dir); err != nil {
		t.Fatalf("Ошибка построения индекса: %v", err)
	}

	rs := NewReconcileService(store, attrStore, idx, lockMgr, time.Hour, logger)
	result := rs.RunOnce()

	if result == nil {
		t.Fatal("Результат nil")
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Type == generated.ChecksumMismatch {
			found = true
			break
		}
	}
	if !found {
		t.Error("Не обнаружен checksum_mismatch")
	}
	if result.Summary.ChecksumMismatches != 1 {
		t.Errorf("ChecksumMismatches: хотели 1, получили %d", result.Summary.ChecksumMismatches)
	}
}

func TestReconcileRunOnce_SkipsHiddenAndTmpFiles(t *testing.T) {
	dir, store, attrStore, idx, lockMgr := setupReconcileTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Скрытые и temp файлы — не должны обнаруживаться как orphaned
	for _, name := range []string{".health_check", ".leader.lock", "upload.tmp"} {
		filePath := filepath.Join(dir, name)
		if err := os.WriteFile(filePath, []byte("data"), 0o640); err != nil {
			t.Fatalf("Ошибка создания файла %s: %v", name, err)
		}
	}

	if err := idx.BuildFromDir(dir); err != nil {
		t.Fatalf("Ошибка построения индекса: %v", err)
	}

	rs := NewReconcileService(store, attrStore, idx, lockMgr, time.Hour, logger)
	result := rs.RunOnce()

	if result == nil {
		t.Fatal("Результат nil")
	}
	if len(result.Issues) != 0 {
		t.Errorf("Найдено %d проблем, ожидалось 0 (скрытые/tmp файлы)", len(result.Issues))
		for _, issue := range result.Issues {
			t.Logf("  %s: path=%v", issue.Type, issue.Path)
		}
	}
}

func TestReconcileRunOnce_ConcurrentSafety(t *testing.T) {
	dir, store, attrStore, idx, lockMgr := setupReconcileTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	if err := idx.BuildFromDir(dir); err != nil {
		t.Fatalf("Ошибка построения индекса: %v", err)
	}

	rs := NewReconcileService(store, attrStore, idx, lockMgr, time.Hour, logger)

	// Запускаем из нескольких горутин — не должно быть паники
	done := make(chan struct{}, 5)
	for i := 0; i < 5; i++ {
		go func() {
			rs.RunOnce()
			done <- struct{}{}
		}()
	}

	for i := 0; i < 5; i++ {
		<-done
	}
	// Все горутины завершились без паники — тест пройден
}

func TestReconcileRunOnce_EmptyDirectory(t *testing.T) {
	dir, store, attrStore, idx, lockMgr := setupReconcileTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	if err := idx.BuildFromDir(dir); err != nil {
		t.Fatalf("Ошибка построения индекса: %v", err)
	}

	rs := NewReconcileService(store, attrStore, idx, lockMgr, time.Hour, logger)
	result := rs.RunOnce()

	if result == nil {
		t.Fatal("Результат nil")
	}
	if len(result.Issues) != 0 {
		t.Errorf("Найдено %d проблем, ожидалось 0 (пустая директория)", len(result.Issues))
	}
}

func TestReconcileRunOnce_RebuildIndex(t *testing.T) {
	_, store, attrStore, idx, lockMgr := setupReconcileTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Добавляем файл напрямую в индекс (без диска)
	idx.Add(&model.FileMetadata{
		FileID:           "phantom-1",
		OriginalFilename: "phantom.txt",
		StoragePath:      "phantom.txt",
		ContentType:      "text/plain",
		Size:             100,
		Checksum:         "abc",
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC(),
		RetentionPolicy:  model.RetentionPermanent,
	})

	if idx.Count() != 1 {
		t.Fatalf("Индекс должен содержать 1 файл, содержит %d", idx.Count())
	}

	rs := NewReconcileService(store, attrStore, idx, lockMgr, time.Hour, logger)
	rs.RunOnce()

	// После reconciliation индекс пересобран — phantom файла нет на диске
	if idx.Count() != 0 {
		t.Errorf("После reconciliation индекс должен быть пуст, содержит %d файлов", idx.Count())
	}
}
