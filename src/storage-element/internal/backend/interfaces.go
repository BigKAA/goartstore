// Пакет backend — абстракция Storage Backend.
//
// Определяет интерфейсы FileStore, AttrStore, LockStore,
// позволяя бизнес-логике не зависеть от типа хранилища (LocalFS, S3 и т.д.).
// Конкретные реализации создаются через фабрику New().
package backend

import (
	"context"
	"io"
	"time"

	"github.com/bigkaa/goartstore/storage-element/internal/domain/model"
)

// FileStore — интерфейс физического хранения файлов.
// Реализации: filestore.FileStore (LocalFS), s3.FileStore (S3).
type FileStore interface {
	// SaveFile записывает данные из reader на хранилище.
	// Возвращает результат с relative path, size, checksum.
	SaveFile(ctx context.Context, reader io.Reader, originalFilename, uploadedBy string) (*SaveResult, error)

	// ReadFile открывает файл для чтения.
	// Возвращает io.ReadCloser — вызывающий код обязан закрыть.
	// Для LocalFS возвращённый объект также реализует io.ReadSeeker.
	ReadFile(ctx context.Context, storagePath string) (io.ReadCloser, error)

	// DeleteFile удаляет файл. Идемпотентно: не ошибка, если файл не существует.
	DeleteFile(ctx context.Context, storagePath string) error

	// FileExists проверяет существование файла.
	FileExists(ctx context.Context, storagePath string) (bool, error)

	// FileSize возвращает размер файла в байтах.
	FileSize(ctx context.Context, storagePath string) (int64, error)

	// ComputeChecksum вычисляет SHA-256 хэш файла.
	ComputeChecksum(ctx context.Context, storagePath string) (string, error)

	// AvailableSpace возвращает доступное место в байтах.
	// LocalFS: syscall.Statfs. S3: quota - cachedTotalSize.
	AvailableSpace(ctx context.Context) (int64, error)

	// FullPath возвращает абсолютный/полный путь к файлу.
	// Для LocalFS: filepath.Join(dataDir, storagePath).
	// Для S3: bucket key.
	FullPath(storagePath string) string

	// ListDataPaths возвращает все relative paths data-файлов.
	// Пропускает .attr.json, .locks/, mode.json, скрытые, .tmp.
	ListDataPaths(ctx context.Context) ([]string, error)
}

// AttrStore — интерфейс хранения метаданных (attr.json).
// Реализации: attr.Store (LocalFS), s3.AttrStore (S3).
//
// Все методы принимают storagePath — относительный путь DATA-файла.
// Реализация сама вычисляет путь к .attr.json.
type AttrStore interface {
	// Write атомарно записывает метаданные для data-файла.
	Write(ctx context.Context, storagePath string, meta *model.FileMetadata) error

	// Read читает метаданные для data-файла.
	Read(ctx context.Context, storagePath string) (*model.FileMetadata, error)

	// Delete удаляет метаданные для data-файла. Идемпотентно.
	Delete(ctx context.Context, storagePath string) error

	// ScanAll возвращает все метаданные из хранилища.
	// Используется при построении индекса и reconciliation.
	ScanAll(ctx context.Context) ([]*model.FileMetadata, error)
}

// LockStore — интерфейс управления per-file lock-ами.
// Реализации: lockfile.LockManager (LocalFS), s3.NoOpLockStore (S3).
type LockStore interface {
	// Acquire захватывает lock для файла.
	Acquire(ctx context.Context, fileID string) error

	// Release освобождает lock для файла. Идемпотентно.
	Release(ctx context.Context, fileID string) error

	// IsLocked проверяет наличие активного lock-а.
	// Возвращает: (locked, lockInfo, error).
	IsLocked(ctx context.Context, fileID string) (bool, *LockInfo, error)

	// List возвращает все lock-и (активные и expired).
	List(ctx context.Context) ([]LockInfo, error)

	// Cleanup удаляет expired (или все при force=true) lock-и.
	Cleanup(ctx context.Context, force bool) (*CleanupResult, error)

	// TTL возвращает сконфигурированное время жизни lock-а.
	TTL() time.Duration

	// EnsureDir создаёт необходимые директории для lock-ов.
	EnsureDir() error
}

// Backend — контейнер для всех store-ов.
// Создаётся через фабрику New().
type Backend struct {
	// Files — хранилище физических файлов
	Files FileStore
	// Attrs — хранилище метаданных (attr.json)
	Attrs AttrStore
	// Locks — менеджер per-file lock-ов
	Locks LockStore
}
