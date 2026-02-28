package index

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bigkaa/goartstore/storage-element/internal/domain/model"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/attr"
)

// testLogger возвращает логгер для тестов.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
}

// createTestMetadata создаёт тестовые метаданные с уникальным ID.
func createTestMetadata(id string, status model.FileStatus, uploadedAt time.Time) *model.FileMetadata {
	return &model.FileMetadata{
		FileID:           id,
		OriginalFilename: fmt.Sprintf("file_%s.txt", id),
		StoragePath:      fmt.Sprintf("file_%s.txt", id),
		ContentType:      "text/plain",
		Size:             1024,
		Checksum:         "abc123",
		UploadedBy:       "admin",
		UploadedAt:       uploadedAt,
		Status:           status,
		RetentionPolicy:  model.RetentionPermanent,
	}
}

// TestNew проверяет создание пустого индекса.
func TestNew(t *testing.T) {
	idx := New(testLogger())

	if idx.Count() != 0 {
		t.Errorf("ожидалось 0 файлов, получено %d", idx.Count())
	}
	if idx.IsReady() {
		t.Error("новый индекс не должен быть ready")
	}
}

// TestAdd проверяет добавление файлов в индекс.
func TestAdd(t *testing.T) {
	idx := New(testLogger())

	meta := createTestMetadata("file-1", model.StatusActive, time.Now())
	idx.Add(meta)

	if idx.Count() != 1 {
		t.Errorf("ожидался 1 файл, получено %d", idx.Count())
	}

	// Проверяем, что файл доступен
	got := idx.Get("file-1")
	if got == nil {
		t.Fatal("файл не найден в индексе")
	}
	if got.FileID != "file-1" {
		t.Errorf("ожидался FileID 'file-1', получен %q", got.FileID)
	}
}

// TestAdd_Overwrite проверяет перезапись существующего файла.
func TestAdd_Overwrite(t *testing.T) {
	idx := New(testLogger())

	meta1 := createTestMetadata("file-1", model.StatusActive, time.Now())
	meta1.Size = 100
	idx.Add(meta1)

	meta2 := createTestMetadata("file-1", model.StatusActive, time.Now())
	meta2.Size = 200
	idx.Add(meta2)

	if idx.Count() != 1 {
		t.Errorf("ожидался 1 файл после перезаписи, получено %d", idx.Count())
	}

	got := idx.Get("file-1")
	if got.Size != 200 {
		t.Errorf("ожидался размер 200, получен %d", got.Size)
	}
}

// TestAdd_CopiesData проверяет, что Add создаёт копию метаданных.
func TestAdd_CopiesData(t *testing.T) {
	idx := New(testLogger())

	meta := createTestMetadata("file-1", model.StatusActive, time.Now())
	idx.Add(meta)

	// Изменяем оригинал
	meta.Size = 999

	// Индекс не должен быть затронут
	got := idx.Get("file-1")
	if got.Size == 999 {
		t.Error("Add должен копировать данные, а не хранить ссылку")
	}
}

// TestGet_NotFound проверяет поиск несуществующего файла.
func TestGet_NotFound(t *testing.T) {
	idx := New(testLogger())

	got := idx.Get("nonexistent")
	if got != nil {
		t.Error("Get для несуществующего файла должен возвращать nil")
	}
}

// TestGet_ReturnsCopy проверяет, что Get возвращает копию.
func TestGet_ReturnsCopy(t *testing.T) {
	idx := New(testLogger())

	idx.Add(createTestMetadata("file-1", model.StatusActive, time.Now()))

	got := idx.Get("file-1")
	got.Size = 999

	// Индекс не должен быть затронут
	got2 := idx.Get("file-1")
	if got2.Size == 999 {
		t.Error("Get должен возвращать копию, а не ссылку")
	}
}

// TestUpdate проверяет обновление файла в индексе.
func TestUpdate(t *testing.T) {
	idx := New(testLogger())

	meta := createTestMetadata("file-1", model.StatusActive, time.Now())
	idx.Add(meta)

	// Обновляем
	meta.Description = "Обновлённое описание"
	meta.Tags = []string{"updated"}
	err := idx.Update(meta)
	if err != nil {
		t.Fatalf("ошибка обновления: %v", err)
	}

	got := idx.Get("file-1")
	if got.Description != "Обновлённое описание" {
		t.Errorf("описание не обновлено: %q", got.Description)
	}
}

// TestUpdate_NotFound проверяет ошибку обновления несуществующего файла.
func TestUpdate_NotFound(t *testing.T) {
	idx := New(testLogger())

	meta := createTestMetadata("nonexistent", model.StatusActive, time.Now())
	err := idx.Update(meta)
	if err == nil {
		t.Error("ожидалась ошибка при обновлении несуществующего файла")
	}
}

// TestRemove проверяет удаление файла из индекса.
func TestRemove(t *testing.T) {
	idx := New(testLogger())

	idx.Add(createTestMetadata("file-1", model.StatusActive, time.Now()))
	idx.Add(createTestMetadata("file-2", model.StatusActive, time.Now()))

	removed := idx.Remove("file-1")
	if !removed {
		t.Error("Remove должен вернуть true для существующего файла")
	}

	if idx.Count() != 1 {
		t.Errorf("ожидался 1 файл после удаления, получено %d", idx.Count())
	}

	if idx.Get("file-1") != nil {
		t.Error("удалённый файл не должен быть в индексе")
	}
}

// TestRemove_NotFound проверяет удаление несуществующего файла.
func TestRemove_NotFound(t *testing.T) {
	idx := New(testLogger())

	removed := idx.Remove("nonexistent")
	if removed {
		t.Error("Remove должен вернуть false для несуществующего файла")
	}
}

// TestList_NoPagination проверяет List без пагинации.
func TestList_NoPagination(t *testing.T) {
	idx := New(testLogger())

	now := time.Now()
	idx.Add(createTestMetadata("file-1", model.StatusActive, now.Add(-2*time.Hour)))
	idx.Add(createTestMetadata("file-2", model.StatusActive, now.Add(-1*time.Hour)))
	idx.Add(createTestMetadata("file-3", model.StatusActive, now))

	items, total := idx.List(0, 0, "")
	if total != 3 {
		t.Errorf("total: ожидалось 3, получено %d", total)
	}
	if len(items) != 3 {
		t.Errorf("items: ожидалось 3, получено %d", len(items))
	}

	// Проверяем сортировку (новые первые)
	if items[0].FileID != "file-3" {
		t.Errorf("первый файл должен быть file-3 (новейший), получен %s", items[0].FileID)
	}
	if items[2].FileID != "file-1" {
		t.Errorf("последний файл должен быть file-1 (старейший), получен %s", items[2].FileID)
	}
}

// TestList_WithPagination проверяет List с limit и offset.
func TestList_WithPagination(t *testing.T) {
	idx := New(testLogger())

	now := time.Now()
	for i := range 10 {
		id := fmt.Sprintf("file-%02d", i)
		idx.Add(createTestMetadata(id, model.StatusActive, now.Add(time.Duration(i)*time.Minute)))
	}

	// Страница 1: limit=3, offset=0
	items, total := idx.List(3, 0, "")
	if total != 10 {
		t.Errorf("total: ожидалось 10, получено %d", total)
	}
	if len(items) != 3 {
		t.Errorf("items: ожидалось 3, получено %d", len(items))
	}

	// Страница 2: limit=3, offset=3
	items2, _ := idx.List(3, 3, "")
	if len(items2) != 3 {
		t.Errorf("items page 2: ожидалось 3, получено %d", len(items2))
	}

	// Последняя страница: limit=3, offset=9
	items3, _ := idx.List(3, 9, "")
	if len(items3) != 1 {
		t.Errorf("items last page: ожидалось 1, получено %d", len(items3))
	}

	// Offset за пределами
	items4, _ := idx.List(3, 100, "")
	if len(items4) != 0 {
		t.Errorf("items beyond: ожидалось 0, получено %d", len(items4))
	}
}

// TestList_WithStatusFilter проверяет фильтрацию по статусу.
func TestList_WithStatusFilter(t *testing.T) {
	idx := New(testLogger())

	now := time.Now()
	idx.Add(createTestMetadata("active-1", model.StatusActive, now))
	idx.Add(createTestMetadata("active-2", model.StatusActive, now))
	idx.Add(createTestMetadata("deleted-1", model.StatusDeleted, now))
	idx.Add(createTestMetadata("expired-1", model.StatusExpired, now))

	// Только active
	items, total := idx.List(0, 0, model.StatusActive)
	if total != 2 {
		t.Errorf("active total: ожидалось 2, получено %d", total)
	}
	if len(items) != 2 {
		t.Errorf("active items: ожидалось 2, получено %d", len(items))
	}

	// Только deleted
	_, total = idx.List(0, 0, model.StatusDeleted)
	if total != 1 {
		t.Errorf("deleted total: ожидалось 1, получено %d", total)
	}

	// Без фильтра
	_, total = idx.List(0, 0, "")
	if total != 4 {
		t.Errorf("all total: ожидалось 4, получено %d", total)
	}
}

// TestList_EmptyIndex проверяет List на пустом индексе.
func TestList_EmptyIndex(t *testing.T) {
	idx := New(testLogger())

	items, total := idx.List(10, 0, "")
	if total != 0 {
		t.Errorf("total: ожидалось 0, получено %d", total)
	}
	if items != nil {
		t.Errorf("items: ожидалось nil, получено %v", items)
	}
}

// TestCount проверяет подсчёт файлов.
func TestCount(t *testing.T) {
	idx := New(testLogger())

	if idx.Count() != 0 {
		t.Error("пустой индекс должен вернуть 0")
	}

	idx.Add(createTestMetadata("f1", model.StatusActive, time.Now()))
	idx.Add(createTestMetadata("f2", model.StatusDeleted, time.Now()))
	idx.Add(createTestMetadata("f3", model.StatusExpired, time.Now()))

	if idx.Count() != 3 {
		t.Errorf("ожидалось 3, получено %d", idx.Count())
	}
}

// TestCountByStatus проверяет подсчёт файлов по статусу.
func TestCountByStatus(t *testing.T) {
	idx := New(testLogger())

	idx.Add(createTestMetadata("a1", model.StatusActive, time.Now()))
	idx.Add(createTestMetadata("a2", model.StatusActive, time.Now()))
	idx.Add(createTestMetadata("d1", model.StatusDeleted, time.Now()))

	if idx.CountByStatus(model.StatusActive) != 2 {
		t.Errorf("active: ожидалось 2, получено %d", idx.CountByStatus(model.StatusActive))
	}
	if idx.CountByStatus(model.StatusDeleted) != 1 {
		t.Errorf("deleted: ожидалось 1, получено %d", idx.CountByStatus(model.StatusDeleted))
	}
	if idx.CountByStatus(model.StatusExpired) != 0 {
		t.Errorf("expired: ожидалось 0, получено %d", idx.CountByStatus(model.StatusExpired))
	}
}

// TestBuildFromDir проверяет построение индекса из attr.json файлов.
func TestBuildFromDir(t *testing.T) {
	dir := t.TempDir()

	// Создаём attr.json файлы
	for i, name := range []string{"file1.txt", "file2.jpg", "file3.pdf"} {
		meta := &model.FileMetadata{
			FileID:           fmt.Sprintf("id-%d", i),
			OriginalFilename: name,
			StoragePath:      name,
			ContentType:      "application/octet-stream",
			Size:             int64(i * 100),
			Checksum:         "abc",
			UploadedBy:       "admin",
			UploadedAt:       time.Now().UTC(),
			Status:           model.StatusActive,
			RetentionPolicy:  model.RetentionPermanent,
		}
		path := filepath.Join(dir, name+attr.AttrSuffix)
		if err := attr.Write(path, meta); err != nil {
			t.Fatalf("ошибка создания attr.json: %v", err)
		}
	}

	// Строим индекс
	idx := New(testLogger())
	if err := idx.BuildFromDir(dir); err != nil {
		t.Fatalf("ошибка BuildFromDir: %v", err)
	}

	if !idx.IsReady() {
		t.Error("индекс должен быть ready после BuildFromDir")
	}

	if idx.Count() != 3 {
		t.Errorf("ожидалось 3 файла, получено %d", idx.Count())
	}

	// Проверяем доступность файлов
	for i := range 3 {
		id := fmt.Sprintf("id-%d", i)
		if idx.Get(id) == nil {
			t.Errorf("файл %s не найден в индексе", id)
		}
	}
}

// TestBuildFromDir_EmptyDir проверяет построение из пустой директории.
func TestBuildFromDir_EmptyDir(t *testing.T) {
	idx := New(testLogger())
	if err := idx.BuildFromDir(t.TempDir()); err != nil {
		t.Fatalf("ошибка: %v", err)
	}

	if !idx.IsReady() {
		t.Error("индекс должен быть ready даже для пустой директории")
	}
	if idx.Count() != 0 {
		t.Errorf("ожидалось 0 файлов, получено %d", idx.Count())
	}
}

// TestRebuildFromDir проверяет пересборку индекса.
func TestRebuildFromDir(t *testing.T) {
	dir := t.TempDir()
	idx := New(testLogger())

	// Добавляем файл вручную
	idx.Add(createTestMetadata("old-file", model.StatusActive, time.Now()))

	// Создаём attr.json на диске
	meta := &model.FileMetadata{
		FileID:          "new-file",
		StoragePath:     "new.txt",
		ContentType:     "text/plain",
		UploadedAt:      time.Now().UTC(),
		Status:          model.StatusActive,
		RetentionPolicy: model.RetentionPermanent,
	}
	attr.Write(filepath.Join(dir, "new.txt"+attr.AttrSuffix), meta)

	// Пересборка
	if err := idx.RebuildFromDir(dir); err != nil {
		t.Fatalf("ошибка RebuildFromDir: %v", err)
	}

	// Старый файл должен исчезнуть
	if idx.Get("old-file") != nil {
		t.Error("старый файл должен быть удалён при пересборке")
	}

	// Новый файл должен быть
	if idx.Get("new-file") == nil {
		t.Error("новый файл должен быть в индексе после пересборки")
	}

	if idx.Count() != 1 {
		t.Errorf("ожидался 1 файл, получено %d", idx.Count())
	}
}

// TestConcurrentAccess проверяет потокобезопасность индекса.
// Запускать с go test -race для обнаружения data races.
func TestConcurrentAccess(t *testing.T) {
	idx := New(testLogger())

	// Предзаполняем
	for i := range 10 {
		idx.Add(createTestMetadata(fmt.Sprintf("init-%d", i), model.StatusActive, time.Now()))
	}

	var wg sync.WaitGroup
	const goroutines = 50

	// Параллельные операции чтения и записи
	wg.Add(goroutines * 4)

	// Читатели — Get
	for range goroutines {
		go func() {
			defer wg.Done()
			for range 100 {
				idx.Get("init-5")
			}
		}()
	}

	// Читатели — List
	for range goroutines {
		go func() {
			defer wg.Done()
			for range 50 {
				idx.List(5, 0, "")
			}
		}()
	}

	// Читатели — Count
	for range goroutines {
		go func() {
			defer wg.Done()
			for range 100 {
				idx.Count()
				idx.CountByStatus(model.StatusActive)
			}
		}()
	}

	// Писатели
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			fileID := fmt.Sprintf("concurrent-%d", id)
			idx.Add(createTestMetadata(fileID, model.StatusActive, time.Now()))
			idx.Get(fileID)
			idx.Remove(fileID)
		}(i)
	}

	wg.Wait()
}

// --- Тесты TotalActiveSize ---

// TestTotalActiveSize_Empty проверяет, что пустой индекс возвращает 0.
func TestTotalActiveSize_Empty(t *testing.T) {
	idx := New(testLogger())

	if got := idx.TotalActiveSize(); got != 0 {
		t.Errorf("пустой индекс: ожидалось 0, получено %d", got)
	}
}

// TestTotalActiveSize_AddActive проверяет увеличение счётчика при добавлении active файла.
func TestTotalActiveSize_AddActive(t *testing.T) {
	idx := New(testLogger())

	meta := createTestMetadata("f1", model.StatusActive, time.Now())
	meta.Size = 5000
	idx.Add(meta)

	if got := idx.TotalActiveSize(); got != 5000 {
		t.Errorf("после добавления active файла: ожидалось 5000, получено %d", got)
	}

	meta2 := createTestMetadata("f2", model.StatusActive, time.Now())
	meta2.Size = 3000
	idx.Add(meta2)

	if got := idx.TotalActiveSize(); got != 8000 {
		t.Errorf("после добавления второго active файла: ожидалось 8000, получено %d", got)
	}
}

// TestTotalActiveSize_AddNonActive проверяет, что deleted/expired файлы не влияют на счётчик.
func TestTotalActiveSize_AddNonActive(t *testing.T) {
	idx := New(testLogger())

	meta1 := createTestMetadata("d1", model.StatusDeleted, time.Now())
	meta1.Size = 1000
	idx.Add(meta1)

	meta2 := createTestMetadata("e1", model.StatusExpired, time.Now())
	meta2.Size = 2000
	idx.Add(meta2)

	if got := idx.TotalActiveSize(); got != 0 {
		t.Errorf("после добавления deleted/expired файлов: ожидалось 0, получено %d", got)
	}
}

// TestTotalActiveSize_UpdateActiveToDeleted проверяет уменьшение счётчика
// при смене статуса active → deleted.
func TestTotalActiveSize_UpdateActiveToDeleted(t *testing.T) {
	idx := New(testLogger())

	meta := createTestMetadata("f1", model.StatusActive, time.Now())
	meta.Size = 5000
	idx.Add(meta)

	// Обновляем статус на deleted
	updated := createTestMetadata("f1", model.StatusDeleted, time.Now())
	updated.Size = 5000
	if err := idx.Update(updated); err != nil {
		t.Fatalf("ошибка обновления: %v", err)
	}

	if got := idx.TotalActiveSize(); got != 0 {
		t.Errorf("после active→deleted: ожидалось 0, получено %d", got)
	}
}

// TestTotalActiveSize_UpdateDeletedToActive проверяет увеличение счётчика
// при смене статуса deleted → active.
func TestTotalActiveSize_UpdateDeletedToActive(t *testing.T) {
	idx := New(testLogger())

	meta := createTestMetadata("f1", model.StatusDeleted, time.Now())
	meta.Size = 3000
	idx.Add(meta)

	if got := idx.TotalActiveSize(); got != 0 {
		t.Errorf("после добавления deleted файла: ожидалось 0, получено %d", got)
	}

	// Обновляем статус на active
	updated := createTestMetadata("f1", model.StatusActive, time.Now())
	updated.Size = 3000
	if err := idx.Update(updated); err != nil {
		t.Fatalf("ошибка обновления: %v", err)
	}

	if got := idx.TotalActiveSize(); got != 3000 {
		t.Errorf("после deleted→active: ожидалось 3000, получено %d", got)
	}
}

// TestTotalActiveSize_RemoveActive проверяет уменьшение счётчика при удалении active файла.
func TestTotalActiveSize_RemoveActive(t *testing.T) {
	idx := New(testLogger())

	meta1 := createTestMetadata("f1", model.StatusActive, time.Now())
	meta1.Size = 5000
	idx.Add(meta1)

	meta2 := createTestMetadata("f2", model.StatusActive, time.Now())
	meta2.Size = 3000
	idx.Add(meta2)

	idx.Remove("f1")

	if got := idx.TotalActiveSize(); got != 3000 {
		t.Errorf("после удаления active файла: ожидалось 3000, получено %d", got)
	}
}

// TestTotalActiveSize_RemoveNonActive проверяет, что удаление deleted файла
// не влияет на счётчик.
func TestTotalActiveSize_RemoveNonActive(t *testing.T) {
	idx := New(testLogger())

	meta1 := createTestMetadata("f1", model.StatusActive, time.Now())
	meta1.Size = 5000
	idx.Add(meta1)

	meta2 := createTestMetadata("f2", model.StatusDeleted, time.Now())
	meta2.Size = 3000
	idx.Add(meta2)

	idx.Remove("f2")

	if got := idx.TotalActiveSize(); got != 5000 {
		t.Errorf("после удаления deleted файла: ожидалось 5000, получено %d", got)
	}
}

// TestTotalActiveSize_AddOverwrite проверяет корректность счётчика при перезаписи файла.
func TestTotalActiveSize_AddOverwrite(t *testing.T) {
	idx := New(testLogger())

	// Добавляем active файл с размером 5000
	meta1 := createTestMetadata("f1", model.StatusActive, time.Now())
	meta1.Size = 5000
	idx.Add(meta1)

	// Перезаписываем тот же файл с новым размером
	meta2 := createTestMetadata("f1", model.StatusActive, time.Now())
	meta2.Size = 8000
	idx.Add(meta2)

	if got := idx.TotalActiveSize(); got != 8000 {
		t.Errorf("после перезаписи active→active: ожидалось 8000, получено %d", got)
	}

	// Перезаписываем active файл на deleted
	meta3 := createTestMetadata("f1", model.StatusDeleted, time.Now())
	meta3.Size = 8000
	idx.Add(meta3)

	if got := idx.TotalActiveSize(); got != 0 {
		t.Errorf("после перезаписи active→deleted: ожидалось 0, получено %d", got)
	}
}

// TestTotalActiveSize_BuildFromDir проверяет пересчёт счётчика при BuildFromDir.
func TestTotalActiveSize_BuildFromDir(t *testing.T) {
	dir := t.TempDir()

	// Создаём файлы: 2 active (100 + 200), 1 deleted (300)
	files := []struct {
		name   string
		id     string
		size   int64
		status model.FileStatus
	}{
		{"active1.txt", "id-1", 100, model.StatusActive},
		{"active2.txt", "id-2", 200, model.StatusActive},
		{"deleted1.txt", "id-3", 300, model.StatusDeleted},
	}
	for _, f := range files {
		meta := &model.FileMetadata{
			FileID:          f.id,
			StoragePath:     f.name,
			ContentType:     "text/plain",
			Size:            f.size,
			UploadedAt:      time.Now().UTC(),
			Status:          f.status,
			RetentionPolicy: model.RetentionPermanent,
		}
		path := filepath.Join(dir, f.name+attr.AttrSuffix)
		if err := attr.Write(path, meta); err != nil {
			t.Fatalf("ошибка создания attr.json: %v", err)
		}
	}

	idx := New(testLogger())

	// Добавляем «мусорные» данные, которые должны быть затёрты BuildFromDir
	old := createTestMetadata("old", model.StatusActive, time.Now())
	old.Size = 9999
	idx.Add(old)

	if err := idx.BuildFromDir(dir); err != nil {
		t.Fatalf("ошибка BuildFromDir: %v", err)
	}

	// Ожидаем 100 + 200 = 300 (deleted файл не учитывается)
	if got := idx.TotalActiveSize(); got != 300 {
		t.Errorf("BuildFromDir: ожидалось 300, получено %d", got)
	}
}

// TestList_PaginationWithFilter проверяет пагинацию с фильтром одновременно.
func TestList_PaginationWithFilter(t *testing.T) {
	idx := New(testLogger())

	now := time.Now()
	// 5 active, 3 deleted
	for i := range 5 {
		idx.Add(createTestMetadata(
			fmt.Sprintf("active-%d", i), model.StatusActive,
			now.Add(time.Duration(i)*time.Minute),
		))
	}
	for i := range 3 {
		idx.Add(createTestMetadata(
			fmt.Sprintf("deleted-%d", i), model.StatusDeleted,
			now.Add(time.Duration(i)*time.Minute),
		))
	}

	// Страница active: limit=2, offset=0
	items, total := idx.List(2, 0, model.StatusActive)
	if total != 5 {
		t.Errorf("total active: ожидалось 5, получено %d", total)
	}
	if len(items) != 2 {
		t.Errorf("items: ожидалось 2, получено %d", len(items))
	}

	// Страница active: limit=2, offset=4
	items, _ = idx.List(2, 4, model.StatusActive)
	if len(items) != 1 {
		t.Errorf("last page: ожидалось 1, получено %d", len(items))
	}
}
