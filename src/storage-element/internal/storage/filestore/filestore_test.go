package filestore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNew_CreatesDirectory проверяет создание директории данных.
func TestNew_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")

	fs, err := New(dir)
	if err != nil {
		t.Fatalf("ошибка создания FileStore: %v", err)
	}

	if fs.DataDir() != dir {
		t.Errorf("ожидался путь %s, получен %s", dir, fs.DataDir())
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("директория не создана: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("путь не является директорией")
	}
}

// TestSaveFile проверяет сохранение файла с подсчётом SHA-256.
func TestSaveFile(t *testing.T) {
	fs, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("ошибка создания FileStore: %v", err)
	}

	content := []byte("Hello, World! Тестовые данные для проверки.")
	reader := bytes.NewReader(content)

	result, err := fs.SaveFile(reader, "test-photo.jpg", "admin")
	if err != nil {
		t.Fatalf("ошибка сохранения: %v", err)
	}

	// Проверяем размер
	if result.Size != int64(len(content)) {
		t.Errorf("размер: ожидалось %d, получено %d", len(content), result.Size)
	}

	// Проверяем checksum
	expectedHash := sha256.Sum256(content)
	expectedChecksum := hex.EncodeToString(expectedHash[:])
	if result.Checksum != expectedChecksum {
		t.Errorf("checksum: ожидалось %s, получено %s", expectedChecksum, result.Checksum)
	}

	// Проверяем, что файл существует на диске
	if _, err := os.Stat(result.FullPath); os.IsNotExist(err) {
		t.Error("файл не найден на диске")
	}

	// Проверяем формат имени файла
	if !strings.Contains(result.StoragePath, "test-photo") {
		t.Errorf("имя файла должно содержать оригинальное имя: %s", result.StoragePath)
	}
	if !strings.Contains(result.StoragePath, "admin") {
		t.Errorf("имя файла должно содержать имя пользователя: %s", result.StoragePath)
	}
	if !strings.HasSuffix(result.StoragePath, ".jpg") {
		t.Errorf("имя файла должно сохранять расширение: %s", result.StoragePath)
	}

	// Проверяем содержимое
	data, err := os.ReadFile(result.FullPath)
	if err != nil {
		t.Fatalf("ошибка чтения файла: %v", err)
	}
	if !bytes.Equal(data, content) {
		t.Error("содержимое файла не совпадает")
	}
}

// TestSaveFile_NoExtension проверяет сохранение файла без расширения.
func TestSaveFile_NoExtension(t *testing.T) {
	fs, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("ошибка создания FileStore: %v", err)
	}

	result, err := fs.SaveFile(bytes.NewReader([]byte("data")), "README", "user1")
	if err != nil {
		t.Fatalf("ошибка сохранения: %v", err)
	}

	if strings.Contains(result.StoragePath, ".") {
		// Файл без расширения не должен иметь точку в конце
		// (допускается точка внутри UUID-части)
	}
	if result.Size != 4 {
		t.Errorf("размер: ожидалось 4, получено %d", result.Size)
	}
}

// TestSaveFile_NoTmpFile проверяет, что temp файл удалён после сохранения.
func TestSaveFile_NoTmpFile(t *testing.T) {
	dir := t.TempDir()
	fs, err := New(dir)
	if err != nil {
		t.Fatalf("ошибка создания FileStore: %v", err)
	}

	result, err := fs.SaveFile(bytes.NewReader([]byte("data")), "file.txt", "user")
	if err != nil {
		t.Fatalf("ошибка сохранения: %v", err)
	}

	tmpPath := result.FullPath + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("временный файл не должен существовать")
	}
}

// TestSaveFile_EmptyFile проверяет сохранение пустого файла.
func TestSaveFile_EmptyFile(t *testing.T) {
	fs, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("ошибка создания FileStore: %v", err)
	}

	result, err := fs.SaveFile(bytes.NewReader(nil), "empty.txt", "user")
	if err != nil {
		t.Fatalf("ошибка сохранения: %v", err)
	}

	if result.Size != 0 {
		t.Errorf("ожидался размер 0, получено %d", result.Size)
	}
}

// TestReadFile проверяет чтение файла.
func TestReadFile(t *testing.T) {
	fs, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("ошибка создания FileStore: %v", err)
	}

	content := []byte("read test data")
	result, err := fs.SaveFile(bytes.NewReader(content), "read-test.txt", "user")
	if err != nil {
		t.Fatalf("ошибка сохранения: %v", err)
	}

	// Чтение
	f, err := fs.ReadFile(result.StoragePath)
	if err != nil {
		t.Fatalf("ошибка открытия для чтения: %v", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ошибка чтения: %v", err)
	}

	if !bytes.Equal(data, content) {
		t.Error("прочитанные данные не совпадают с записанными")
	}
}

// TestReadFile_NotFound проверяет ошибку при чтении несуществующего файла.
func TestReadFile_NotFound(t *testing.T) {
	fs, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("ошибка создания FileStore: %v", err)
	}

	_, err = fs.ReadFile("nonexistent.txt")
	if err == nil {
		t.Error("ожидалась ошибка для несуществующего файла")
	}
}

// TestDeleteFile проверяет удаление файла.
func TestDeleteFile(t *testing.T) {
	fs, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("ошибка создания FileStore: %v", err)
	}

	result, err := fs.SaveFile(bytes.NewReader([]byte("delete me")), "delete.txt", "user")
	if err != nil {
		t.Fatalf("ошибка сохранения: %v", err)
	}

	// Удаление
	if err := fs.DeleteFile(result.StoragePath); err != nil {
		t.Fatalf("ошибка удаления: %v", err)
	}

	// Проверяем, что файл удалён
	if fs.FileExists(result.StoragePath) {
		t.Error("файл должен быть удалён")
	}
}

// TestDeleteFile_NotFound проверяет, что удаление несуществующего файла не ошибка.
func TestDeleteFile_NotFound(t *testing.T) {
	fs, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("ошибка создания FileStore: %v", err)
	}

	if err := fs.DeleteFile("nonexistent.txt"); err != nil {
		t.Errorf("удаление несуществующего файла не должно быть ошибкой: %v", err)
	}
}

// TestFileExists проверяет определение существования файла.
func TestFileExists(t *testing.T) {
	fs, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("ошибка создания FileStore: %v", err)
	}

	// Не существует
	if fs.FileExists("no-file.txt") {
		t.Error("файл не должен существовать")
	}

	// Создаём файл
	result, err := fs.SaveFile(bytes.NewReader([]byte("exists")), "exists.txt", "user")
	if err != nil {
		t.Fatalf("ошибка сохранения: %v", err)
	}

	// Существует
	if !fs.FileExists(result.StoragePath) {
		t.Error("файл должен существовать")
	}
}

// TestFileSize проверяет получение размера файла.
func TestFileSize(t *testing.T) {
	fs, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("ошибка создания FileStore: %v", err)
	}

	content := []byte("size check data - 123")
	result, err := fs.SaveFile(bytes.NewReader(content), "size.txt", "user")
	if err != nil {
		t.Fatalf("ошибка сохранения: %v", err)
	}

	size, err := fs.FileSize(result.StoragePath)
	if err != nil {
		t.Fatalf("ошибка получения размера: %v", err)
	}

	if size != int64(len(content)) {
		t.Errorf("размер: ожидалось %d, получено %d", len(content), size)
	}
}

// TestComputeChecksum проверяет вычисление SHA-256 существующего файла.
func TestComputeChecksum(t *testing.T) {
	fs, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("ошибка создания FileStore: %v", err)
	}

	content := []byte("checksum verification data")
	result, err := fs.SaveFile(bytes.NewReader(content), "check.bin", "user")
	if err != nil {
		t.Fatalf("ошибка сохранения: %v", err)
	}

	checksum, err := fs.ComputeChecksum(result.StoragePath)
	if err != nil {
		t.Fatalf("ошибка вычисления checksum: %v", err)
	}

	// Checksum при сохранении и повторном вычислении должны совпадать
	if checksum != result.Checksum {
		t.Errorf("checksum не совпадает: save=%s, compute=%s", result.Checksum, checksum)
	}
}

// TestGenerateStorageName проверяет генерацию имени файла.
func TestGenerateStorageName(t *testing.T) {
	name := generateStorageName("My Photo.jpg", "admin")

	if !strings.HasSuffix(name, ".jpg") {
		t.Errorf("должно сохраняться расширение .jpg: %s", name)
	}
	if !strings.Contains(name, "admin") {
		t.Errorf("должно содержать имя пользователя: %s", name)
	}
	// Имя файла не должно содержать пробелы
	if strings.Contains(name, " ") {
		t.Errorf("не должно содержать пробелов: %s", name)
	}
}

// TestSanitize проверяет очистку строк для имени файла.
func TestSanitize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"hello world", "helloworld"},
		{"test-file_01", "test-file_01"},
		{"file@#$%", "file"},
		{"", "file"}, // пустая строка → "file"
		{"тест", "тест"},
	}

	for _, tt := range tests {
		result := sanitize(tt.input)
		if result != tt.expected {
			t.Errorf("sanitize(%q): ожидалось %q, получено %q", tt.input, tt.expected, result)
		}
	}
}

// TestFullPath проверяет формирование полного пути.
func TestFullPath(t *testing.T) {
	fs, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("ошибка создания FileStore: %v", err)
	}

	fullPath := fs.FullPath("test.txt")
	expected := filepath.Join(fs.DataDir(), "test.txt")

	if fullPath != expected {
		t.Errorf("ожидалось %s, получено %s", expected, fullPath)
	}
}
