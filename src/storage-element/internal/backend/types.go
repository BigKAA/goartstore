package backend

import "time"

// SaveResult — результат сохранения файла.
type SaveResult struct {
	// StoragePath — относительный путь файла в хранилище
	StoragePath string
	// FullPath — абсолютный/полный путь файла
	FullPath string
	// Size — размер записанных данных в байтах
	Size int64
	// Checksum — SHA-256 хэш содержимого файла
	Checksum string
}

// LockInfo — метаданные lock-а.
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

// CleanupResult — результат очистки lock-ов.
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
