// Пакет filestore — операции с физическими файлами на диске.
// Обеспечивает streaming-запись с подсчётом SHA-256 на лету,
// чтение, удаление и получение информации о ёмкости диска.
package filestore

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// FileStore — управление физическими файлами на диске.
type FileStore struct {
	// dataDir — корневая директория хранения файлов (SE_DATA_DIR)
	dataDir string
}

// SaveResult — результат сохранения файла на диск.
type SaveResult struct {
	// StoragePath — относительный путь файла в dataDir
	StoragePath string
	// FullPath — абсолютный путь файла на диске
	FullPath string
	// Size — размер записанных данных в байтах
	Size int64
	// Checksum — SHA-256 хэш содержимого файла
	Checksum string
}

// New создаёт новый FileStore. Проверяет и создаёт директорию
// если она не существует.
func New(dataDir string) (*FileStore, error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return nil, fmt.Errorf("не удалось создать директорию данных %s: %w", dataDir, err)
	}

	return &FileStore{dataDir: dataDir}, nil
}

// SaveFile записывает данные из reader на диск с подсчётом SHA-256 на лету.
// Формат имени файла: {name}_{user}_{timestamp}_{uuid}.{ext}
// Возвращает путь, размер и checksum записанного файла.
//
// Паттерн: temp файл → запись + SHA-256 → fsync → atomic rename.
// При ошибке temp файл удаляется.
func (fs *FileStore) SaveFile(reader io.Reader, originalFilename, uploadedBy string) (*SaveResult, error) {
	// Генерируем имя файла для хранения
	storageName := generateStorageName(originalFilename, uploadedBy)
	fullPath := filepath.Join(fs.dataDir, storageName)
	tmpPath := fullPath + ".tmp"

	// Создаём temp файл
	f, err := os.Create(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания временного файла: %w", err)
	}

	// Streaming запись с одновременным подсчётом SHA-256
	hasher := sha256.New()
	tee := io.TeeReader(reader, hasher)

	size, err := io.Copy(f, tee)
	if err != nil {
		f.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("ошибка записи данных: %w", err)
	}

	// fsync для гарантии записи на диск
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("ошибка fsync: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("ошибка закрытия файла: %w", err)
	}

	// Атомарный rename
	if err := os.Rename(tmpPath, fullPath); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("ошибка атомарного переименования: %w", err)
	}

	return &SaveResult{
		StoragePath: storageName,
		FullPath:    fullPath,
		Size:        size,
		Checksum:    hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

// ReadFile открывает файл для чтения и возвращает io.ReadCloser.
// storagePath — относительный путь файла в dataDir.
// Вызывающий код обязан закрыть ReadCloser.
func (fs *FileStore) ReadFile(storagePath string) (*os.File, error) {
	fullPath := filepath.Join(fs.dataDir, storagePath)

	f, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("файл не найден: %s", storagePath)
		}
		return nil, fmt.Errorf("ошибка открытия файла %s: %w", storagePath, err)
	}

	return f, nil
}

// FullPath возвращает абсолютный путь к файлу на диске.
func (fs *FileStore) FullPath(storagePath string) string {
	return filepath.Join(fs.dataDir, storagePath)
}

// DeleteFile удаляет файл с диска.
// storagePath — относительный путь файла в dataDir.
// Возвращает nil если файл уже не существует.
func (fs *FileStore) DeleteFile(storagePath string) error {
	fullPath := filepath.Join(fs.dataDir, storagePath)

	err := os.Remove(fullPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("ошибка удаления файла %s: %w", storagePath, err)
	}
	return nil
}

// FileExists проверяет существование файла на диске.
func (fs *FileStore) FileExists(storagePath string) bool {
	fullPath := filepath.Join(fs.dataDir, storagePath)
	_, err := os.Stat(fullPath)
	return err == nil
}

// FileSize возвращает размер файла на диске.
func (fs *FileStore) FileSize(storagePath string) (int64, error) {
	fullPath := filepath.Join(fs.dataDir, storagePath)
	info, err := os.Stat(fullPath)
	if err != nil {
		return 0, fmt.Errorf("ошибка получения информации о файле %s: %w", storagePath, err)
	}
	return info.Size(), nil
}

// ComputeChecksum вычисляет SHA-256 хэш существующего файла.
// Используется при reconciliation для проверки целостности.
func (fs *FileStore) ComputeChecksum(storagePath string) (string, error) {
	fullPath := filepath.Join(fs.dataDir, storagePath)

	f, err := os.Open(fullPath)
	if err != nil {
		return "", fmt.Errorf("ошибка открытия файла %s: %w", storagePath, err)
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", fmt.Errorf("ошибка вычисления checksum %s: %w", storagePath, err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// DataDir возвращает путь к директории данных.
func (fs *FileStore) DataDir() string {
	return fs.dataDir
}

// generateStorageName генерирует имя файла для хранения на диске.
// Формат: {name}_{user}_{timestamp}_{uuid}.{ext}
// Пример: photo_admin_20260221150405_a1b2c3d4.jpg
func generateStorageName(originalFilename, uploadedBy string) string {
	ext := filepath.Ext(originalFilename)
	name := strings.TrimSuffix(originalFilename, ext)

	// Убираем небезопасные символы из имени и пользователя
	name = sanitize(name)
	user := sanitize(uploadedBy)

	// Ограничиваем длину имени для предотвращения проблем с FS
	if len(name) > 50 {
		name = name[:50]
	}
	if len(user) > 20 {
		user = user[:20]
	}

	ts := time.Now().UTC().Format("20060102150405")
	uid := uuid.New().String()[:8] // Короткий UUID для уникальности

	if ext != "" {
		return fmt.Sprintf("%s_%s_%s_%s%s", name, user, ts, uid, ext)
	}
	return fmt.Sprintf("%s_%s_%s_%s", name, user, ts, uid)
}

// sanitize убирает небезопасные символы из строки для использования в имени файла.
// Оставляет только буквы, цифры, дефис и подчёркивание.
func sanitize(s string) string {
	var result strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' ||
			(r >= 0x0400 && r <= 0x04FF) { // Кириллица
			result.WriteRune(r)
		}
	}
	if result.Len() == 0 {
		return "file"
	}
	return result.String()
}
