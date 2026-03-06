package service

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bigkaa/goartstore/storage-element/internal/domain/model"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/attr"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/index"
)

// setupIndexSyncTestEnv создаёт тестовое окружение для IndexSyncService тестов.
// Возвращает: dataDir, attrStore, index (пустой, ready=false).
func setupIndexSyncTestEnv(t *testing.T) (string, *attr.Store, *index.Index) {
	t.Helper()

	dir := t.TempDir()
	attrStore := attr.NewStore(dir)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	idx := index.New(logger)

	// Строим пустой индекс, чтобы он стал ready
	if err := idx.BuildFromDir(dir); err != nil {
		t.Fatalf("Ошибка построения индекса: %v", err)
	}

	return dir, attrStore, idx
}

// createTestAttrFile создаёт файл данных и attr.json на диске (без добавления в индекс).
// Используется для имитации файла, записанного другим pod-ом.
// StoragePath может содержать иерархический путь YYYY/MM/DD/filename.
func createTestAttrFile(t *testing.T, dir string, meta *model.FileMetadata) {
	t.Helper()

	// Создаём промежуточные каталоги если нужно
	filePath := filepath.Join(dir, meta.StoragePath)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o750); err != nil {
		t.Fatalf("Ошибка создания каталога для тестового файла: %v", err)
	}

	// Создаём файл данных
	if err := os.WriteFile(filePath, []byte("test data from another pod"), 0o640); err != nil {
		t.Fatalf("Ошибка создания тестового файла: %v", err)
	}

	// Создаём attr.json (attr.Write содержит MkdirAll)
	attrPath := attr.AttrFilePath(filePath)
	if err := attr.Write(attrPath, meta); err != nil {
		t.Fatalf("Ошибка создания attr.json: %v", err)
	}
}

func TestIndexSyncService_SyncOnce_EmptyDir(t *testing.T) {
	// Пустая директория — SyncOnce не должен паниковать
	_, attrStore, idx := setupIndexSyncTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	svc := NewIndexSyncService(idx, attrStore, 30*time.Second, logger)
	svc.SyncOnce()

	if idx.Count() != 0 {
		t.Errorf("Индекс не пуст: %d файлов", idx.Count())
	}
}

func TestIndexSyncService_SyncOnce_DetectsNewFiles(t *testing.T) {
	// Файлы добавлены на диск другим pod-ом — SyncOnce должен их обнаружить
	dir, attrStore, idx := setupIndexSyncTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Имитируем файлы, записанные другим pod-ом (на диске, но не в индексе)
	meta1 := &model.FileMetadata{
		FileID:           "remote-1",
		OriginalFilename: "remote1.txt",
		StoragePath:      "2026/03/01/remote1.txt",
		ContentType:      "text/plain",
		Size:             26,
		Checksum:         "abc",
		UploadedBy:       "other-pod",
		UploadedAt:       time.Now().UTC(),
		RetentionPolicy:  model.RetentionPermanent,
	}
	meta2 := &model.FileMetadata{
		FileID:           "remote-2",
		OriginalFilename: "remote2.txt",
		StoragePath:      "2026/03/01/remote2.txt",
		ContentType:      "text/plain",
		Size:             26,
		Checksum:         "def",
		UploadedBy:       "other-pod",
		UploadedAt:       time.Now().UTC(),
		RetentionPolicy:  model.RetentionPermanent,
	}

	createTestAttrFile(t, dir, meta1)
	createTestAttrFile(t, dir, meta2)

	// До sync — 0 файлов в индексе
	if idx.Count() != 0 {
		t.Fatalf("Индекс должен быть пуст: %d", idx.Count())
	}

	svc := NewIndexSyncService(idx, attrStore, 30*time.Second, logger)
	svc.SyncOnce()

	// После sync — 2 файла
	if idx.Count() != 2 {
		t.Errorf("Индекс: хотели 2 файла, получили %d", idx.Count())
	}

	// Проверяем что файлы найдены
	if idx.Get("remote-1") == nil {
		t.Error("Файл remote-1 не найден в индексе после sync")
	}
	if idx.Get("remote-2") == nil {
		t.Error("Файл remote-2 не найден в индексе после sync")
	}
}

func TestIndexSyncService_SyncOnce_DetectsDeletedFiles(t *testing.T) {
	// Файл удалён с диска другим pod-ом — SyncOnce должен убрать из индекса
	dir, attrStore, idx := setupIndexSyncTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Создаём файл на диске
	meta := &model.FileMetadata{
		FileID:           "to-remove",
		OriginalFilename: "toremove.txt",
		StoragePath:      "2026/02/20/toremove.txt",
		ContentType:      "text/plain",
		Size:             26,
		Checksum:         "abc",
		UploadedBy:       "this-pod",
		UploadedAt:       time.Now().UTC(),
		RetentionPolicy:  model.RetentionPermanent,
	}
	createTestAttrFile(t, dir, meta)
	idx.Add(meta)

	if idx.Count() != 1 {
		t.Fatalf("Индекс: хотели 1 файл, получили %d", idx.Count())
	}

	// Удаляем файл и attr.json с диска (имитируем GC другого pod-а)
	os.Remove(filepath.Join(dir, "2026/02/20/toremove.txt"))
	os.Remove(attr.AttrFilePath(filepath.Join(dir, "2026/02/20/toremove.txt")))

	svc := NewIndexSyncService(idx, attrStore, 30*time.Second, logger)
	svc.SyncOnce()

	// После sync — 0 файлов (файл удалён с диска)
	if idx.Count() != 0 {
		t.Errorf("Индекс: хотели 0 файлов, получили %d", idx.Count())
	}
}

func TestIndexSyncService_SyncOnce_PreservesExistingFiles(t *testing.T) {
	// Существующие файлы на диске — SyncOnce не теряет их
	dir, attrStore, idx := setupIndexSyncTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Создаём файлы на диске и в индексе
	meta := &model.FileMetadata{
		FileID:           "existing-1",
		OriginalFilename: "existing.txt",
		StoragePath:      "2026/02/20/existing.txt",
		ContentType:      "text/plain",
		Size:             26,
		Checksum:         "abc",
		UploadedBy:       "this-pod",
		UploadedAt:       time.Now().UTC(),
		RetentionPolicy:  model.RetentionPermanent,
	}
	createTestAttrFile(t, dir, meta)
	idx.Add(meta)

	svc := NewIndexSyncService(idx, attrStore, 30*time.Second, logger)
	svc.SyncOnce()

	// Файл остался в индексе
	if idx.Count() != 1 {
		t.Errorf("Индекс: хотели 1 файл, получили %d", idx.Count())
	}
	if idx.Get("existing-1") == nil {
		t.Error("Файл existing-1 потерян после sync")
	}
}

func TestIndexSyncService_StartStop(t *testing.T) {
	// Проверяем жизненный цикл: Start → Stop без паники
	_, attrStore, idx := setupIndexSyncTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	svc := NewIndexSyncService(idx, attrStore, 50*time.Millisecond, logger)
	ctx := context.Background()

	svc.Start(ctx)

	// Даём сервису время выполнить несколько итераций
	time.Sleep(200 * time.Millisecond)

	svc.Stop()
	// Повторный Stop не должен паниковать
	svc.Stop()
}

func TestIndexSyncService_DetectsNewFiles_Background(t *testing.T) {
	// Проверяем обнаружение файлов фоновым процессом
	dir, attrStore, idx := setupIndexSyncTestEnv(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	svc := NewIndexSyncService(idx, attrStore, 50*time.Millisecond, logger)
	ctx := context.Background()

	svc.Start(ctx)
	defer svc.Stop()

	// Добавляем файл после запуска сервиса
	time.Sleep(30 * time.Millisecond)
	meta := &model.FileMetadata{
		FileID:           "bg-new-1",
		OriginalFilename: "bgnew.txt",
		StoragePath:      "2026/03/01/bgnew.txt",
		ContentType:      "text/plain",
		Size:             26,
		Checksum:         "abc",
		UploadedBy:       "other-pod",
		UploadedAt:       time.Now().UTC(),
		RetentionPolicy:  model.RetentionPermanent,
	}
	createTestAttrFile(t, dir, meta)

	// Ждём пока сервис обнаружит файл (2+ интервала)
	time.Sleep(200 * time.Millisecond)

	if idx.Get("bg-new-1") == nil {
		t.Error("Фоновый процесс не обнаружил новый файл bg-new-1")
	}
}
