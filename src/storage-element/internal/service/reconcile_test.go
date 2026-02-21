package service

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arturkryukov/artsore/storage-element/internal/api/generated"
	"github.com/arturkryukov/artsore/storage-element/internal/domain/model"
	"github.com/arturkryukov/artsore/storage-element/internal/storage/attr"
	"github.com/arturkryukov/artsore/storage-element/internal/storage/filestore"
	"github.com/arturkryukov/artsore/storage-element/internal/storage/index"
)

// setupReconcileTestEnv создаёт тестовое окружение для reconciliation тестов.
func setupReconcileTestEnv(t *testing.T) (string, *filestore.FileStore, *index.Index) {
	t.Helper()

	dir := t.TempDir()
	store, err := filestore.New(dir)
	if err != nil {
		t.Fatalf("Ошибка создания FileStore: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	idx := index.New(logger)

	return dir, store, idx
}

func TestReconcileRunOnce_NoIssues(t *testing.T) {
	dir, store, idx := setupReconcileTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Создаём корректную пару файл + attr.json
	meta := &model.FileMetadata{
		FileID:           "good-1",
		OriginalFilename: "good.txt",
		StoragePath:      "good.txt",
		ContentType:      "text/plain",
		Size:             9,
		Checksum:         "", // Будет вычислен ниже
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC(),
		Status:           model.StatusActive,
		RetentionPolicy:  model.RetentionPermanent,
	}

	// Записываем файл данных
	filePath := filepath.Join(dir, "good.txt")
	content := []byte("test data")
	if err := os.WriteFile(filePath, content, 0o640); err != nil {
		t.Fatalf("Ошибка записи файла: %v", err)
	}

	// Вычисляем checksum
	checksum, err := store.ComputeChecksum("good.txt")
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

	rs := NewReconcileService(store, idx, dir, time.Hour, logger)
	result, skipped := rs.RunOnce()

	if skipped {
		t.Fatal("Reconciliation пропущена")
	}
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
	dir, store, idx := setupReconcileTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Файл на диске без attr.json
	filePath := filepath.Join(dir, "orphaned.txt")
	if err := os.WriteFile(filePath, []byte("data"), 0o640); err != nil {
		t.Fatalf("Ошибка создания файла: %v", err)
	}

	if err := idx.BuildFromDir(dir); err != nil {
		t.Fatalf("Ошибка построения индекса: %v", err)
	}

	rs := NewReconcileService(store, idx, dir, time.Hour, logger)
	result, _ := rs.RunOnce()

	if result == nil {
		t.Fatal("Результат nil")
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Type == generated.OrphanedFile && issue.Path != nil && *issue.Path == "orphaned.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Не обнаружен orphaned_file для orphaned.txt")
	}
	if result.Summary.OrphanedFiles != 1 {
		t.Errorf("OrphanedFiles: хотели 1, получили %d", result.Summary.OrphanedFiles)
	}
}

func TestReconcileRunOnce_MissingFile(t *testing.T) {
	dir, store, idx := setupReconcileTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// attr.json без файла данных
	meta := &model.FileMetadata{
		FileID:           "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		OriginalFilename: "missing.txt",
		StoragePath:      "missing.txt",
		ContentType:      "text/plain",
		Size:             100,
		Checksum:         "abc123",
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC(),
		Status:           model.StatusActive,
		RetentionPolicy:  model.RetentionPermanent,
	}

	attrPath := filepath.Join(dir, "missing.txt"+attr.AttrSuffix)
	if err := attr.Write(attrPath, meta); err != nil {
		t.Fatalf("Ошибка записи attr.json: %v", err)
	}

	if err := idx.BuildFromDir(dir); err != nil {
		t.Fatalf("Ошибка построения индекса: %v", err)
	}

	rs := NewReconcileService(store, idx, dir, time.Hour, logger)
	result, _ := rs.RunOnce()

	if result == nil {
		t.Fatal("Результат nil")
	}

	found := false
	for _, issue := range result.Issues {
		if issue.Type == generated.MissingFile && issue.Path != nil && *issue.Path == "missing.txt" {
			found = true
			// Проверяем, что file_id заполнен
			if issue.FileId == nil {
				t.Error("FileId nil для missing_file")
			}
			break
		}
	}
	if !found {
		t.Error("Не обнаружен missing_file для missing.txt")
	}
	if result.Summary.MissingFiles != 1 {
		t.Errorf("MissingFiles: хотели 1, получили %d", result.Summary.MissingFiles)
	}
}

func TestReconcileRunOnce_SizeMismatch(t *testing.T) {
	dir, store, idx := setupReconcileTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Файл с неправильным размером в attr.json
	filePath := filepath.Join(dir, "size_mismatch.txt")
	if err := os.WriteFile(filePath, []byte("actual data"), 0o640); err != nil {
		t.Fatalf("Ошибка создания файла: %v", err)
	}

	meta := &model.FileMetadata{
		FileID:           "size-1",
		OriginalFilename: "size_mismatch.txt",
		StoragePath:      "size_mismatch.txt",
		ContentType:      "text/plain",
		Size:             999, // Неправильный размер
		Checksum:         "abc",
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC(),
		Status:           model.StatusActive,
		RetentionPolicy:  model.RetentionPermanent,
	}

	attrPath := attr.AttrFilePath(filePath)
	if err := attr.Write(attrPath, meta); err != nil {
		t.Fatalf("Ошибка записи attr.json: %v", err)
	}

	if err := idx.BuildFromDir(dir); err != nil {
		t.Fatalf("Ошибка построения индекса: %v", err)
	}

	rs := NewReconcileService(store, idx, dir, time.Hour, logger)
	result, _ := rs.RunOnce()

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
	dir, store, idx := setupReconcileTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Файл с неправильным checksum в attr.json
	filePath := filepath.Join(dir, "cs_mismatch.txt")
	content := []byte("actual data")
	if err := os.WriteFile(filePath, content, 0o640); err != nil {
		t.Fatalf("Ошибка создания файла: %v", err)
	}

	meta := &model.FileMetadata{
		FileID:           "cs-1",
		OriginalFilename: "cs_mismatch.txt",
		StoragePath:      "cs_mismatch.txt",
		ContentType:      "text/plain",
		Size:             int64(len(content)), // Правильный размер
		Checksum:         "deadbeef",          // Неправильный checksum
		UploadedBy:       "test",
		UploadedAt:       time.Now().UTC(),
		Status:           model.StatusActive,
		RetentionPolicy:  model.RetentionPermanent,
	}

	attrPath := attr.AttrFilePath(filePath)
	if err := attr.Write(attrPath, meta); err != nil {
		t.Fatalf("Ошибка записи attr.json: %v", err)
	}

	if err := idx.BuildFromDir(dir); err != nil {
		t.Fatalf("Ошибка построения индекса: %v", err)
	}

	rs := NewReconcileService(store, idx, dir, time.Hour, logger)
	result, _ := rs.RunOnce()

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
	dir, store, idx := setupReconcileTestEnv(t)
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

	rs := NewReconcileService(store, idx, dir, time.Hour, logger)
	result, _ := rs.RunOnce()

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

func TestReconcileRunOnce_ConcurrentProtection(t *testing.T) {
	dir, store, idx := setupReconcileTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	if err := idx.BuildFromDir(dir); err != nil {
		t.Fatalf("Ошибка построения индекса: %v", err)
	}

	rs := NewReconcileService(store, idx, dir, time.Hour, logger)

	// Запускаем из нескольких горутин
	results := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func() {
			_, skipped := rs.RunOnce()
			results <- skipped
		}()
	}

	skippedCount := 0
	for i := 0; i < 5; i++ {
		if <-results {
			skippedCount++
		}
	}

	// Хотя бы одна должна пройти, остальные могут быть пропущены
	if skippedCount == 5 {
		t.Error("Все 5 запусков были пропущены — ни один не выполнился")
	}
}

func TestReconcileRunOnce_EmptyDirectory(t *testing.T) {
	dir, store, idx := setupReconcileTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	if err := idx.BuildFromDir(dir); err != nil {
		t.Fatalf("Ошибка построения индекса: %v", err)
	}

	rs := NewReconcileService(store, idx, dir, time.Hour, logger)
	result, skipped := rs.RunOnce()

	if skipped {
		t.Fatal("Reconciliation пропущена")
	}
	if result == nil {
		t.Fatal("Результат nil")
	}
	if len(result.Issues) != 0 {
		t.Errorf("Найдено %d проблем, ожидалось 0 (пустая директория)", len(result.Issues))
	}
}

func TestReconcileRunOnce_RebuildIndex(t *testing.T) {
	dir, store, idx := setupReconcileTestEnv(t)
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
		Status:           model.StatusActive,
		RetentionPolicy:  model.RetentionPermanent,
	})

	if idx.Count() != 1 {
		t.Fatalf("Индекс должен содержать 1 файл, содержит %d", idx.Count())
	}

	rs := NewReconcileService(store, idx, dir, time.Hour, logger)
	rs.RunOnce()

	// После reconciliation индекс пересобран — phantom файла нет на диске
	if idx.Count() != 0 {
		t.Errorf("После reconciliation индекс должен быть пуст, содержит %d файлов", idx.Count())
	}
}
