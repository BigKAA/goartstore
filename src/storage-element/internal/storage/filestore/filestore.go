// Пакет filestore — операции с физическими файлами на диске.
// Обеспечивает streaming-запись с подсчётом SHA-256 на лету,
// чтение, удаление и получение информации о ёмкости диска.
package filestore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/bigkaa/goartstore/storage-element/internal/backend"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/attr"
)

// Compile-time check: FileStore реализует backend.FileStore.
var _ backend.FileStore = (*FileStore)(nil)

// FileStore — управление физическими файлами на диске.
type FileStore struct {
	// dataDir — корневая директория хранения файлов (SE_DATA_DIR)
	dataDir string
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
// Формат пути: YYYY/MM/DD/{name}_{user}_{timestamp}_{uuid}.{ext}
// Возвращает путь, размер и checksum записанного файла.
//
// Паттерн: MkdirAll → temp файл → запись + SHA-256 → fsync → atomic rename.
// При ошибке temp файл удаляется.
func (fs *FileStore) SaveFile(_ context.Context, reader io.Reader, originalFilename, uploadedBy string) (*backend.SaveResult, error) {
	// Генерируем относительный путь с date-based иерархией
	storagePath := generateStoragePath(originalFilename, uploadedBy)
	fullPath := filepath.Join(fs.dataDir, storagePath)
	tmpPath := fullPath + ".tmp"

	// Создаём промежуточные каталоги (YYYY/MM/DD/) если не существуют
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
		return nil, fmt.Errorf("ошибка создания каталога для файла: %w", err)
	}

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
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("ошибка записи данных: %w", err)
	}

	// fsync для гарантии записи на диск
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("ошибка fsync: %w", err)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("ошибка закрытия файла: %w", err)
	}

	// Атомарный rename
	if err := os.Rename(tmpPath, fullPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("ошибка атомарного переименования: %w", err)
	}

	return &backend.SaveResult{
		StoragePath: storagePath,
		FullPath:    fullPath,
		Size:        size,
		Checksum:    hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

// ReadFile открывает файл для чтения и возвращает io.ReadCloser.
// storagePath — относительный путь файла в dataDir.
// Возвращённый объект также реализует io.ReadSeeker (т.к. это *os.File).
// Вызывающий код обязан закрыть ReadCloser.
func (fs *FileStore) ReadFile(_ context.Context, storagePath string) (io.ReadCloser, error) {
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
func (fs *FileStore) DeleteFile(_ context.Context, storagePath string) error {
	fullPath := filepath.Join(fs.dataDir, storagePath)

	err := os.Remove(fullPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("ошибка удаления файла %s: %w", storagePath, err)
	}
	return nil
}

// FileExists проверяет существование файла на диске.
func (fs *FileStore) FileExists(_ context.Context, storagePath string) (bool, error) {
	fullPath := filepath.Join(fs.dataDir, storagePath)
	_, err := os.Stat(fullPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("ошибка проверки файла %s: %w", storagePath, err)
}

// FileSize возвращает размер файла на диске.
func (fs *FileStore) FileSize(_ context.Context, storagePath string) (int64, error) {
	fullPath := filepath.Join(fs.dataDir, storagePath)
	info, err := os.Stat(fullPath)
	if err != nil {
		return 0, fmt.Errorf("ошибка получения информации о файле %s: %w", storagePath, err)
	}
	return info.Size(), nil
}

// ComputeChecksum вычисляет SHA-256 хэш существующего файла.
// Используется при reconciliation для проверки целостности.
func (fs *FileStore) ComputeChecksum(_ context.Context, storagePath string) (string, error) {
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

// AvailableSpace возвращает доступное место на файловой системе в байтах.
func (fs *FileStore) AvailableSpace(_ context.Context) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(fs.dataDir, &stat); err != nil {
		return 0, fmt.Errorf("ошибка получения информации о FS %s: %w", fs.dataDir, err)
	}
	// Доступное место для непривилегированного пользователя
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}

// ListDataPaths возвращает все relative paths data-файлов в хранилище.
// Пропускает .attr.json, .locks/, mode.json, скрытые каталоги/файлы, .tmp.
func (fs *FileStore) ListDataPaths(_ context.Context) ([]string, error) {
	var paths []string

	err := filepath.WalkDir(fs.dataDir, func(path string, d iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Пропускаем скрытые каталоги (.locks/ и пр.)
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") && path != fs.dataDir {
				return filepath.SkipDir
			}
			// Не следуем за symlink-каталогами
			if d.Type()&iofs.ModeSymlink != 0 {
				return filepath.SkipDir
			}
			return nil
		}

		name := d.Name()

		// Пропускаем mode.json
		if name == "mode.json" {
			return nil
		}
		// Пропускаем скрытые файлы
		if strings.HasPrefix(name, ".") {
			return nil
		}
		// Пропускаем temp файлы
		if strings.HasSuffix(name, ".tmp") {
			return nil
		}
		// Пропускаем attr.json файлы
		if attr.IsAttrFile(name) {
			return nil
		}

		relPath, relErr := filepath.Rel(fs.dataDir, path)
		if relErr != nil {
			return nil
		}

		paths = append(paths, relPath)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ошибка обхода директории %s: %w", fs.dataDir, err)
	}

	return paths, nil
}

// DataDir возвращает путь к директории данных.
func (fs *FileStore) DataDir() string {
	return fs.dataDir
}

// generateStoragePath генерирует относительный путь файла для хранения на диске.
// Формат: YYYY/MM/DD/{name}_{user}_{timestamp}_{uuid}.{ext}
// Пример: 2026/02/21/photo_admin_20260221150405_a1b2c3d4.jpg
//
// Иерархическая структура YYYY/MM/DD/ необходима для:
//   - Масштабируемости FS (ext4 деградирует при >100K файлов в одном каталоге)
//   - Администрирования (файлы организованы по датам)
//   - Подготовки к S3 (единая date-based структура для LocalFS и S3)
func generateStoragePath(originalFilename, uploadedBy string) string {
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

	now := time.Now().UTC()
	ts := now.Format("20060102150405")
	uid := uuid.New().String()[:8] // Короткий UUID для уникальности

	// Дата-каталог: YYYY/MM/DD
	datePrefix := now.Format("2006/01/02")

	var filename string
	if ext != "" {
		filename = fmt.Sprintf("%s_%s_%s_%s%s", name, user, ts, uid, ext)
	} else {
		filename = fmt.Sprintf("%s_%s_%s_%s", name, user, ts, uid)
	}

	return filepath.Join(datePrefix, filename)
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
