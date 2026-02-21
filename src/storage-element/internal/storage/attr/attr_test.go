package attr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arturkryukov/artsore/storage-element/internal/domain/model"
)

// testMetadata создаёт тестовые метаданные.
func testMetadata() *model.FileMetadata {
	ttl := 30
	expiresAt := time.Now().UTC().Add(30 * 24 * time.Hour)
	return &model.FileMetadata{
		FileID:           "test-file-id-001",
		OriginalFilename: "test-photo.jpg",
		StoragePath:      "test-photo_admin_20260221_001.jpg",
		ContentType:      "image/jpeg",
		Size:             1024,
		Checksum:         "abc123def456",
		UploadedBy:       "admin",
		UploadedAt:       time.Now().UTC().Truncate(time.Second),
		Status:           model.StatusActive,
		RetentionPolicy:  model.RetentionTemporary,
		TtlDays:          &ttl,
		ExpiresAt:        &expiresAt,
		Tags:             []string{"test", "photo"},
		Description:      "Тестовый файл",
	}
}

// TestWriteAndRead проверяет запись и чтение attr.json.
func TestWriteAndRead(t *testing.T) {
	dir := t.TempDir()
	meta := testMetadata()
	path := filepath.Join(dir, "test.jpg"+AttrSuffix)

	// Запись
	if err := Write(path, meta); err != nil {
		t.Fatalf("ошибка записи: %v", err)
	}

	// Проверяем, что файл существует
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("attr.json файл не создан")
	}

	// Чтение
	readMeta, err := Read(path)
	if err != nil {
		t.Fatalf("ошибка чтения: %v", err)
	}

	// Сравнение полей
	if readMeta.FileID != meta.FileID {
		t.Errorf("FileID: ожидалось %q, получено %q", meta.FileID, readMeta.FileID)
	}
	if readMeta.OriginalFilename != meta.OriginalFilename {
		t.Errorf("OriginalFilename: ожидалось %q, получено %q", meta.OriginalFilename, readMeta.OriginalFilename)
	}
	if readMeta.StoragePath != meta.StoragePath {
		t.Errorf("StoragePath: ожидалось %q, получено %q", meta.StoragePath, readMeta.StoragePath)
	}
	if readMeta.ContentType != meta.ContentType {
		t.Errorf("ContentType: ожидалось %q, получено %q", meta.ContentType, readMeta.ContentType)
	}
	if readMeta.Size != meta.Size {
		t.Errorf("Size: ожидалось %d, получено %d", meta.Size, readMeta.Size)
	}
	if readMeta.Checksum != meta.Checksum {
		t.Errorf("Checksum: ожидалось %q, получено %q", meta.Checksum, readMeta.Checksum)
	}
	if readMeta.Status != meta.Status {
		t.Errorf("Status: ожидалось %q, получено %q", meta.Status, readMeta.Status)
	}
	if readMeta.RetentionPolicy != meta.RetentionPolicy {
		t.Errorf("RetentionPolicy: ожидалось %q, получено %q", meta.RetentionPolicy, readMeta.RetentionPolicy)
	}
	if *readMeta.TtlDays != *meta.TtlDays {
		t.Errorf("TtlDays: ожидалось %d, получено %d", *meta.TtlDays, *readMeta.TtlDays)
	}
	if len(readMeta.Tags) != len(meta.Tags) {
		t.Errorf("Tags: ожидалось %d тегов, получено %d", len(meta.Tags), len(readMeta.Tags))
	}
	if readMeta.Description != meta.Description {
		t.Errorf("Description: ожидалось %q, получено %q", meta.Description, readMeta.Description)
	}
}

// TestWrite_AtomicNoTmpFile проверяет, что temp файл не остаётся после записи.
func TestWrite_AtomicNoTmpFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jpg"+AttrSuffix)

	if err := Write(path, testMetadata()); err != nil {
		t.Fatalf("ошибка записи: %v", err)
	}

	// Проверяем отсутствие .tmp файла
	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("временный файл не должен существовать после атомарной записи")
	}
}

// TestWrite_OverwriteExisting проверяет перезапись существующего attr.json.
func TestWrite_OverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jpg"+AttrSuffix)
	meta := testMetadata()

	// Первая запись
	if err := Write(path, meta); err != nil {
		t.Fatalf("ошибка первой записи: %v", err)
	}

	// Обновление
	meta.Description = "Обновлённое описание"
	meta.Tags = []string{"updated"}

	if err := Write(path, meta); err != nil {
		t.Fatalf("ошибка перезаписи: %v", err)
	}

	// Проверяем обновлённые данные
	readMeta, err := Read(path)
	if err != nil {
		t.Fatalf("ошибка чтения: %v", err)
	}

	if readMeta.Description != "Обновлённое описание" {
		t.Errorf("описание не обновлено: %q", readMeta.Description)
	}
	if len(readMeta.Tags) != 1 || readMeta.Tags[0] != "updated" {
		t.Errorf("теги не обновлены: %v", readMeta.Tags)
	}
}

// TestWrite_PermanentRetention проверяет запись метаданных permanent файла.
func TestWrite_PermanentRetention(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jpg"+AttrSuffix)

	meta := &model.FileMetadata{
		FileID:           "perm-file-001",
		OriginalFilename: "document.pdf",
		StoragePath:      "document_admin_20260221_001.pdf",
		ContentType:      "application/pdf",
		Size:             2048,
		Checksum:         "sha256hash",
		UploadedBy:       "user1",
		UploadedAt:       time.Now().UTC(),
		Status:           model.StatusActive,
		RetentionPolicy:  model.RetentionPermanent,
	}

	if err := Write(path, meta); err != nil {
		t.Fatalf("ошибка записи: %v", err)
	}

	readMeta, err := Read(path)
	if err != nil {
		t.Fatalf("ошибка чтения: %v", err)
	}

	if readMeta.TtlDays != nil {
		t.Error("TtlDays должен быть nil для permanent")
	}
	if readMeta.ExpiresAt != nil {
		t.Error("ExpiresAt должен быть nil для permanent")
	}
}

// TestRead_NotFound проверяет ошибку при чтении несуществующего файла.
func TestRead_NotFound(t *testing.T) {
	_, err := Read("/nonexistent/path/file.attr.json")
	if err == nil {
		t.Error("ожидалась ошибка для несуществующего файла")
	}
}

// TestRead_InvalidJSON проверяет ошибку при невалидном JSON.
func TestRead_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.attr.json")

	if err := os.WriteFile(path, []byte("invalid json"), 0o640); err != nil {
		t.Fatalf("ошибка создания файла: %v", err)
	}

	_, err := Read(path)
	if err == nil {
		t.Error("ожидалась ошибка для невалидного JSON")
	}
}

// TestDelete проверяет удаление attr.json.
func TestDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.attr.json")

	// Создаём файл
	if err := Write(path, testMetadata()); err != nil {
		t.Fatalf("ошибка записи: %v", err)
	}

	// Удаляем
	if err := Delete(path); err != nil {
		t.Fatalf("ошибка удаления: %v", err)
	}

	// Проверяем, что удалён
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("файл должен быть удалён")
	}
}

// TestDelete_NotFound проверяет, что удаление несуществующего файла не ошибка.
func TestDelete_NotFound(t *testing.T) {
	err := Delete("/nonexistent/path/file.attr.json")
	if err != nil {
		t.Errorf("удаление несуществующего файла не должно возвращать ошибку: %v", err)
	}
}

// TestAttrFilePath проверяет формирование пути к attr.json.
func TestAttrFilePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/data/photo.jpg", "/data/photo.jpg.attr.json"},
		{"/data/doc.pdf", "/data/doc.pdf.attr.json"},
		{"file.txt", "file.txt.attr.json"},
	}

	for _, tt := range tests {
		result := AttrFilePath(tt.input)
		if result != tt.expected {
			t.Errorf("AttrFilePath(%q): ожидалось %q, получено %q", tt.input, tt.expected, result)
		}
	}
}

// TestDataFilePathFromAttr проверяет извлечение пути файла данных из attr.json.
func TestDataFilePathFromAttr(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/data/photo.jpg.attr.json", "/data/photo.jpg"},
		{"/data/doc.pdf.attr.json", "/data/doc.pdf"},
	}

	for _, tt := range tests {
		result := DataFilePathFromAttr(tt.input)
		if result != tt.expected {
			t.Errorf("DataFilePathFromAttr(%q): ожидалось %q, получено %q", tt.input, tt.expected, result)
		}
	}
}

// TestIsAttrFile проверяет определение файла метаданных по пути.
func TestIsAttrFile(t *testing.T) {
	if !IsAttrFile("photo.jpg.attr.json") {
		t.Error("photo.jpg.attr.json должен быть attr-файлом")
	}
	if IsAttrFile("photo.jpg") {
		t.Error("photo.jpg не должен быть attr-файлом")
	}
}

// TestScanDir проверяет сканирование директории на attr.json файлы.
func TestScanDir(t *testing.T) {
	dir := t.TempDir()

	// Создаём 3 attr.json файла
	for i, name := range []string{"file1.jpg", "file2.pdf", "file3.txt"} {
		meta := testMetadata()
		meta.FileID = "scan-" + name
		meta.OriginalFilename = name
		ttl := 30 + i
		meta.TtlDays = &ttl
		path := filepath.Join(dir, name+AttrSuffix)
		if err := Write(path, meta); err != nil {
			t.Fatalf("ошибка записи %s: %v", name, err)
		}
	}

	// Создаём обычный файл (не attr.json)
	os.WriteFile(filepath.Join(dir, "not-attr.txt"), []byte("data"), 0o640)

	// Сканирование
	results, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("ошибка сканирования: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("ожидалось 3 метаданных, получено %d", len(results))
	}
}

// TestScanDir_EmptyDir проверяет сканирование пустой директории.
func TestScanDir_EmptyDir(t *testing.T) {
	results, err := ScanDir(t.TempDir())
	if err != nil {
		t.Fatalf("ошибка сканирования: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("ожидалось 0 метаданных, получено %d", len(results))
	}
}

// TestScanDir_SkipInvalidJSON проверяет, что невалидные attr.json пропускаются.
func TestScanDir_SkipInvalidJSON(t *testing.T) {
	dir := t.TempDir()

	// Один валидный
	Write(filepath.Join(dir, "good.jpg"+AttrSuffix), testMetadata())

	// Один невалидный
	os.WriteFile(filepath.Join(dir, "bad.jpg"+AttrSuffix), []byte("broken"), 0o640)

	results, err := ScanDir(dir)
	if err != nil {
		t.Fatalf("ошибка сканирования: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("ожидалось 1 метаданных (невалидный пропущен), получено %d", len(results))
	}
}

// TestWrite_TooLargeAttr проверяет отклонение слишком больших attr.json.
func TestWrite_TooLargeAttr(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.attr.json")

	meta := testMetadata()
	// Создаём описание > 4 КБ
	meta.Description = strings.Repeat("A", 5000)

	err := Write(path, meta)
	if err == nil {
		t.Error("ожидалась ошибка для слишком большого attr.json")
	}
}
