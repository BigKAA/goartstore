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
func createTestMetadata(id string, uploadedAt time.Time) *model.FileMetadata {
	return &model.FileMetadata{
		FileID:           id,
		OriginalFilename: fmt.Sprintf("file_%s.txt", id),
		StoragePath:      fmt.Sprintf("file_%s.txt", id),
		ContentType:      "text/plain",
		Size:             1024,
		Checksum:         "abc123",
		UploadedBy:       "admin",
		UploadedAt:       uploadedAt,
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

	meta := createTestMetadata("file-1", time.Now())
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

	meta1 := createTestMetadata("file-1", time.Now())
	meta1.Size = 100
	idx.Add(meta1)

	meta2 := createTestMetadata("file-1", time.Now())
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

	meta := createTestMetadata("file-1", time.Now())
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

	idx.Add(createTestMetadata("file-1", time.Now()))

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

	meta := createTestMetadata("file-1", time.Now())
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

	meta := createTestMetadata("nonexistent", time.Now())
	err := idx.Update(meta)
	if err == nil {
		t.Error("ожидалась ошибка при обновлении несуществующего файла")
	}
}

// TestRemove проверяет удаление файла из индекса.
func TestRemove(t *testing.T) {
	idx := New(testLogger())

	idx.Add(createTestMetadata("file-1", time.Now()))
	idx.Add(createTestMetadata("file-2", time.Now()))

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
	idx.Add(createTestMetadata("file-1", now.Add(-2*time.Hour)))
	idx.Add(createTestMetadata("file-2", now.Add(-1*time.Hour)))
	idx.Add(createTestMetadata("file-3", now))

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
		idx.Add(createTestMetadata(id, now.Add(time.Duration(i)*time.Minute)))
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

// TestList_WithRetentionFilter проверяет фильтрацию по политике хранения.
func TestList_WithRetentionFilter(t *testing.T) {
	idx := New(testLogger())

	now := time.Now()
	ttl := 30
	expiresAt := now.Add(30 * 24 * time.Hour)

	// 2 permanent файла
	idx.Add(createTestMetadata("perm-1", now))
	idx.Add(createTestMetadata("perm-2", now))

	// 2 temporary файла с TTL
	tmp1 := createTestMetadata("tmp-1", now)
	tmp1.RetentionPolicy = model.RetentionTemporary
	tmp1.TTLDays = &ttl
	tmp1.ExpiresAt = &expiresAt
	idx.Add(tmp1)

	tmp2 := createTestMetadata("tmp-2", now)
	tmp2.RetentionPolicy = model.RetentionTemporary
	tmp2.TTLDays = &ttl
	tmp2.ExpiresAt = &expiresAt
	idx.Add(tmp2)

	// Только permanent
	items, total := idx.List(0, 0, model.RetentionPermanent)
	if total != 2 {
		t.Errorf("permanent total: ожидалось 2, получено %d", total)
	}
	if len(items) != 2 {
		t.Errorf("permanent items: ожидалось 2, получено %d", len(items))
	}

	// Только temporary
	items, total = idx.List(0, 0, model.RetentionTemporary)
	if total != 2 {
		t.Errorf("temporary total: ожидалось 2, получено %d", total)
	}
	if len(items) != 2 {
		t.Errorf("temporary items: ожидалось 2, получено %d", len(items))
	}

	// Без фильтра — все файлы
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

	idx.Add(createTestMetadata("f1", time.Now()))
	idx.Add(createTestMetadata("f2", time.Now()))
	idx.Add(createTestMetadata("f3", time.Now()))

	if idx.Count() != 3 {
		t.Errorf("ожидалось 3, получено %d", idx.Count())
	}
}

// TestBuildFromDir проверяет построение индекса из attr.json в иерархической структуре YYYY/MM/DD/.
func TestBuildFromDir(t *testing.T) {
	dir := t.TempDir()

	// Создаём attr.json файлы в иерархической структуре
	dateDirs := []string{"2026/02/20", "2026/02/21", "2026/03/01"}
	names := []string{"file1.txt", "file2.jpg", "file3.pdf"}
	for i, name := range names {
		storagePath := filepath.Join(dateDirs[i], name)
		meta := &model.FileMetadata{
			FileID:           fmt.Sprintf("id-%d", i),
			OriginalFilename: name,
			StoragePath:      storagePath,
			ContentType:      "application/octet-stream",
			Size:             int64(i * 100),
			Checksum:         "abc",
			UploadedBy:       "admin",
			UploadedAt:       time.Now().UTC(),
			RetentionPolicy:  model.RetentionPermanent,
		}
		path := filepath.Join(dir, storagePath+attr.AttrSuffix)
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

// TestRebuildFromDir проверяет пересборку индекса из иерархической структуры.
func TestRebuildFromDir(t *testing.T) {
	dir := t.TempDir()
	idx := New(testLogger())

	// Добавляем файл вручную
	idx.Add(createTestMetadata("old-file", time.Now()))

	// Создаём attr.json на диске в иерархической структуре
	storagePath := "2026/03/01/new.txt"
	meta := &model.FileMetadata{
		FileID:          "new-file",
		StoragePath:     storagePath,
		ContentType:     "text/plain",
		UploadedAt:      time.Now().UTC(),
		RetentionPolicy: model.RetentionPermanent,
	}
	attr.Write(filepath.Join(dir, storagePath+attr.AttrSuffix), meta)

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
		idx.Add(createTestMetadata(fmt.Sprintf("init-%d", i), time.Now()))
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
			}
		}()
	}

	// Писатели
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			fileID := fmt.Sprintf("concurrent-%d", id)
			idx.Add(createTestMetadata(fileID, time.Now()))
			idx.Get(fileID)
			idx.Remove(fileID)
		}(i)
	}

	wg.Wait()
}

// --- Тесты TotalSize ---

// TestTotalSize_Empty проверяет, что пустой индекс возвращает 0.
func TestTotalSize_Empty(t *testing.T) {
	idx := New(testLogger())

	if got := idx.TotalActiveSize(); got != 0 {
		t.Errorf("пустой индекс: ожидалось 0, получено %d", got)
	}
}

// TestTotalSize_AddFiles проверяет увеличение счётчика при добавлении файлов.
func TestTotalSize_AddFiles(t *testing.T) {
	idx := New(testLogger())

	meta := createTestMetadata("f1", time.Now())
	meta.Size = 5000
	idx.Add(meta)

	if got := idx.TotalActiveSize(); got != 5000 {
		t.Errorf("после добавления файла: ожидалось 5000, получено %d", got)
	}

	meta2 := createTestMetadata("f2", time.Now())
	meta2.Size = 3000
	idx.Add(meta2)

	if got := idx.TotalActiveSize(); got != 8000 {
		t.Errorf("после добавления второго файла: ожидалось 8000, получено %d", got)
	}
}

// TestTotalSize_RemoveFile проверяет уменьшение счётчика при удалении файла.
func TestTotalSize_RemoveFile(t *testing.T) {
	idx := New(testLogger())

	meta1 := createTestMetadata("f1", time.Now())
	meta1.Size = 5000
	idx.Add(meta1)

	meta2 := createTestMetadata("f2", time.Now())
	meta2.Size = 3000
	idx.Add(meta2)

	idx.Remove("f1")

	if got := idx.TotalActiveSize(); got != 3000 {
		t.Errorf("после удаления файла: ожидалось 3000, получено %d", got)
	}
}

// TestTotalSize_AddOverwrite проверяет корректность счётчика при перезаписи файла.
func TestTotalSize_AddOverwrite(t *testing.T) {
	idx := New(testLogger())

	// Добавляем файл с размером 5000
	meta1 := createTestMetadata("f1", time.Now())
	meta1.Size = 5000
	idx.Add(meta1)

	// Перезаписываем тот же файл с новым размером
	meta2 := createTestMetadata("f1", time.Now())
	meta2.Size = 8000
	idx.Add(meta2)

	if got := idx.TotalActiveSize(); got != 8000 {
		t.Errorf("после перезаписи: ожидалось 8000, получено %d", got)
	}
}

// TestTotalSize_BuildFromDir проверяет пересчёт счётчика при BuildFromDir
// с иерархической структурой YYYY/MM/DD/.
func TestTotalSize_BuildFromDir(t *testing.T) {
	dir := t.TempDir()

	// Создаём 3 файла с разными размерами (100 + 200 + 300 = 600)
	files := []struct {
		dateDir string
		name    string
		id      string
		size    int64
	}{
		{"2026/02/20", "file1.txt", "id-1", 100},
		{"2026/02/21", "file2.txt", "id-2", 200},
		{"2026/03/01", "file3.txt", "id-3", 300},
	}
	for _, f := range files {
		storagePath := filepath.Join(f.dateDir, f.name)
		meta := &model.FileMetadata{
			FileID:          f.id,
			StoragePath:     storagePath,
			ContentType:     "text/plain",
			Size:            f.size,
			UploadedAt:      time.Now().UTC(),
			RetentionPolicy: model.RetentionPermanent,
		}
		path := filepath.Join(dir, storagePath+attr.AttrSuffix)
		if err := attr.Write(path, meta); err != nil {
			t.Fatalf("ошибка создания attr.json: %v", err)
		}
	}

	idx := New(testLogger())

	// Добавляем «мусорные» данные, которые должны быть затёрты BuildFromDir
	old := createTestMetadata("old", time.Now())
	old.Size = 9999
	idx.Add(old)

	if err := idx.BuildFromDir(dir); err != nil {
		t.Fatalf("ошибка BuildFromDir: %v", err)
	}

	// Ожидаем 100 + 200 + 300 = 600 (все файлы учитываются)
	if got := idx.TotalActiveSize(); got != 600 {
		t.Errorf("BuildFromDir: ожидалось 600, получено %d", got)
	}
}

// TestList_PaginationWithFilter проверяет пагинацию с фильтром по политике хранения.
func TestList_PaginationWithFilter(t *testing.T) {
	idx := New(testLogger())

	now := time.Now()
	ttl := 30
	expiresAt := now.Add(30 * 24 * time.Hour)

	// 5 permanent файлов
	for i := range 5 {
		idx.Add(createTestMetadata(
			fmt.Sprintf("perm-%d", i),
			now.Add(time.Duration(i)*time.Minute),
		))
	}

	// 3 temporary файла с TTL
	for i := range 3 {
		tmp := createTestMetadata(
			fmt.Sprintf("tmp-%d", i),
			now.Add(time.Duration(i)*time.Minute),
		)
		tmp.RetentionPolicy = model.RetentionTemporary
		tmp.TTLDays = &ttl
		tmp.ExpiresAt = &expiresAt
		idx.Add(tmp)
	}

	// Страница permanent: limit=2, offset=0
	items, total := idx.List(2, 0, model.RetentionPermanent)
	if total != 5 {
		t.Errorf("total permanent: ожидалось 5, получено %d", total)
	}
	if len(items) != 2 {
		t.Errorf("items: ожидалось 2, получено %d", len(items))
	}

	// Страница permanent: limit=2, offset=4
	items, _ = idx.List(2, 4, model.RetentionPermanent)
	if len(items) != 1 {
		t.Errorf("last page: ожидалось 1, получено %d", len(items))
	}
}
