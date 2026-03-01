// Пакет lockfile — управление per-file lock-файлами с TTL.
//
// Каждый lock — отдельный JSON-файл в директории {dataDir}/.locks/{fileID}.lock.
// Lock защищает файл от преждевременного GC/Reconcile/Delete во время upload-а.
// TTL гарантирует автоматическую очистку при crash pod-а.
//
// Формат lock-файла:
//
//	{
//	  "holder": "se-edit-1-7f9b4c6d8-x2k9p",
//	  "file_id": "abc-123-def-456",
//	  "acquired_at": "2026-03-01T12:00:00Z",
//	  "ttl_seconds": 120
//	}
package lockfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// lockDir — имя поддиректории для lock-файлов.
	lockDir = ".locks"
	// lockSuffix — расширение файла lock.
	lockSuffix = ".lock"
)

// LockInfo — метаданные lock-файла.
type LockInfo struct {
	// Holder — идентификатор экземпляра, захватившего lock (hostname pod-а).
	Holder string `json:"holder"`
	// FileID — идентификатор файла, защищённого lock-ом.
	FileID string `json:"file_id"`
	// AcquiredAt — время захвата lock-а (UTC).
	AcquiredAt time.Time `json:"acquired_at"`
	// TTLSeconds — время жизни lock-а в секундах.
	TTLSeconds int `json:"ttl_seconds"`
}

// ExpiresAt возвращает время истечения lock-а.
func (l *LockInfo) ExpiresAt() time.Time {
	return l.AcquiredAt.Add(time.Duration(l.TTLSeconds) * time.Second)
}

// IsExpired проверяет, истёк ли TTL lock-а.
func (l *LockInfo) IsExpired(now time.Time) bool {
	return now.After(l.ExpiresAt())
}

// CleanupResult — результат очистки expired lock-ов.
type CleanupResult struct {
	// Cleaned — количество удалённых lock-ов.
	Cleaned int `json:"cleaned"`
	// Remaining — количество оставшихся активных lock-ов.
	Remaining int `json:"remaining"`
	// Removed — информация об удалённых lock-ах.
	Removed []LockInfo `json:"removed"`
	// Active — информация об оставшихся активных lock-ах.
	Active []LockInfo `json:"active"`
}

// LockManager — менеджер per-file lock-файлов.
type LockManager struct {
	// dataDir — базовая директория хранилища (корень, не .locks/).
	dataDir string
	// ttl — время жизни lock-а (из SE_UPLOAD_LOCK_TTL).
	ttl time.Duration
	// holder — идентификатор текущего экземпляра (hostname).
	holder string
}

// NewLockManager создаёт LockManager.
//
// Параметры:
//   - dataDir: базовая директория хранилища ({dataDir}/.locks/ для lock-файлов)
//   - ttl: время жизни lock-а
//   - holder: идентификатор текущего экземпляра (обычно os.Hostname())
func NewLockManager(dataDir string, ttl time.Duration, holder string) *LockManager {
	return &LockManager{
		dataDir: dataDir,
		ttl:     ttl,
		holder:  holder,
	}
}

// EnsureDir создаёт директорию .locks/ если она не существует.
// Вызывается один раз при старте приложения.
func (lm *LockManager) EnsureDir() error {
	dir := lm.locksDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("не удалось создать директорию lock-файлов %s: %w", dir, err)
	}
	return nil
}

// Acquire захватывает lock для файла.
// Создаёт lock-файл с JSON-метаданными (holder, file_id, acquired_at, ttl_seconds).
// FileID уникален (UUID v4), поэтому коллизий не бывает.
func (lm *LockManager) Acquire(fileID string) error {
	info := LockInfo{
		Holder:     lm.holder,
		FileID:     fileID,
		AcquiredAt: time.Now().UTC(),
		TTLSeconds: int(lm.ttl.Seconds()),
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("ошибка сериализации lock: %w", err)
	}

	lockPath := lm.lockPath(fileID)

	// Атомарная запись: temp → fsync → rename
	tmpPath := lockPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("ошибка создания lock-файла %s: %w", lockPath, err)
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("ошибка записи lock-файла: %w", err)
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("ошибка fsync lock-файла: %w", err)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("ошибка закрытия lock-файла: %w", err)
	}

	if err := os.Rename(tmpPath, lockPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("ошибка rename lock-файла: %w", err)
	}

	return nil
}

// Release удаляет lock для файла.
// Если lock-файл не существует — не ошибка (идемпотентно).
func (lm *LockManager) Release(fileID string) error {
	lockPath := lm.lockPath(fileID)
	err := os.Remove(lockPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("ошибка удаления lock-файла %s: %w", lockPath, err)
	}
	return nil
}

// IsLocked проверяет наличие активного (не expired) lock-а для файла.
//
// Возвращает:
//   - true, *LockInfo — lock существует и TTL не истёк
//   - false, *LockInfo — lock существует, но TTL истёк
//   - false, nil — lock не существует
func (lm *LockManager) IsLocked(fileID string) (bool, *LockInfo, error) {
	lockPath := lm.lockPath(fileID)

	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil, nil
		}
		return false, nil, fmt.Errorf("ошибка чтения lock-файла: %w", err)
	}

	var info LockInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return false, nil, fmt.Errorf("ошибка десериализации lock-файла: %w", err)
	}

	now := time.Now().UTC()
	if info.IsExpired(now) {
		return false, &info, nil
	}

	return true, &info, nil
}

// List возвращает список всех lock-ов (и активных, и expired).
func (lm *LockManager) List() ([]LockInfo, error) {
	dir := lm.locksDir()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("ошибка чтения директории lock-ов: %w", err)
	}

	var locks []LockInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isLockFile(name) {
			continue
		}

		data, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil {
			continue
		}

		var info LockInfo
		if jsonErr := json.Unmarshal(data, &info); jsonErr != nil {
			continue
		}

		locks = append(locks, info)
	}

	return locks, nil
}

// Cleanup удаляет expired lock-файлы.
// При force=true удаляет все lock-файлы (включая активные).
func (lm *LockManager) Cleanup(force bool) (*CleanupResult, error) {
	locks, err := lm.List()
	if err != nil {
		return nil, fmt.Errorf("ошибка получения списка lock-ов: %w", err)
	}

	result := &CleanupResult{}
	now := time.Now().UTC()

	for _, lock := range locks {
		if force || lock.IsExpired(now) {
			// Удаляем lock-файл
			lockPath := lm.lockPath(lock.FileID)
			if rmErr := os.Remove(lockPath); rmErr != nil && !os.IsNotExist(rmErr) {
				continue
			}
			result.Cleaned++
			result.Removed = append(result.Removed, lock)
		} else {
			result.Remaining++
			result.Active = append(result.Active, lock)
		}
	}

	return result, nil
}

// TTL возвращает сконфигурированное время жизни lock-а.
func (lm *LockManager) TTL() time.Duration {
	return lm.ttl
}

// locksDir возвращает путь к директории lock-файлов.
func (lm *LockManager) locksDir() string {
	return filepath.Join(lm.dataDir, lockDir)
}

// lockPath возвращает путь к lock-файлу для данного fileID.
func (lm *LockManager) lockPath(fileID string) string {
	return filepath.Join(lm.locksDir(), fileID+lockSuffix)
}

// isLockFile проверяет, является ли файл lock-файлом по расширению.
func isLockFile(name string) bool {
	return filepath.Ext(name) == lockSuffix
}
