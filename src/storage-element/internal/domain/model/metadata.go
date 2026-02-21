// Пакет model — доменные модели Storage Element.
// FileMetadata — единая структура метаданных файла, используется
// как in-memory представление и как формат attr.json на диске.
package model

import (
	"time"
)

// FileStatus — статус файла в хранилище.
type FileStatus string

const (
	// StatusActive — файл доступен для операций
	StatusActive FileStatus = "active"
	// StatusDeleted — помечен на удаление (ожидает очистки GC)
	StatusDeleted FileStatus = "deleted"
	// StatusExpired — TTL истёк (ожидает очистки GC)
	StatusExpired FileStatus = "expired"
)

// RetentionPolicy — политика хранения файла.
type RetentionPolicy string

const (
	// RetentionPermanent — без срока хранения
	RetentionPermanent RetentionPolicy = "permanent"
	// RetentionTemporary — с TTL, автоматически удаляется GC
	RetentionTemporary RetentionPolicy = "temporary"
)

// FileMetadata — метаданные файла. Соответствует содержимому attr.json.
// Поле StoragePath не входит в API-ответ, но сохраняется в attr.json
// для привязки метаданных к физическому файлу на диске.
type FileMetadata struct {
	// FileID — уникальный идентификатор файла (UUID v4)
	FileID string `json:"file_id"`

	// OriginalFilename — оригинальное имя файла при загрузке
	OriginalFilename string `json:"original_filename"`

	// StoragePath — имя файла на диске (относительно SE_DATA_DIR).
	// Формат: {name}_{user}_{timestamp}_{uuid}.{ext}
	// Не возвращается в API, используется только внутри SE.
	StoragePath string `json:"storage_path"`

	// ContentType — MIME-тип файла
	ContentType string `json:"content_type"`

	// Size — размер файла в байтах
	Size int64 `json:"size"`

	// Checksum — SHA-256 хэш содержимого файла
	Checksum string `json:"checksum"`

	// UploadedBy — идентификатор пользователя/сервиса (из JWT sub)
	UploadedBy string `json:"uploaded_by"`

	// UploadedAt — дата и время загрузки (UTC)
	UploadedAt time.Time `json:"uploaded_at"`

	// Status — текущий статус файла
	Status FileStatus `json:"status"`

	// RetentionPolicy — политика хранения
	RetentionPolicy RetentionPolicy `json:"retention_policy"`

	// TtlDays — срок хранения в днях (только для temporary).
	// nil для permanent файлов.
	TtlDays *int `json:"ttl_days,omitempty"`

	// ExpiresAt — дата истечения (uploaded_at + ttl_days).
	// nil для permanent файлов.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	// Tags — теги файла (опционально)
	Tags []string `json:"tags,omitempty"`

	// Description — описание файла (опционально)
	Description string `json:"description,omitempty"`
}

// IsExpired проверяет, истёк ли срок хранения файла.
func (m *FileMetadata) IsExpired(now time.Time) bool {
	if m.RetentionPolicy != RetentionTemporary {
		return false
	}
	if m.ExpiresAt == nil {
		return false
	}
	return now.After(*m.ExpiresAt)
}

// IsActive проверяет, что файл в активном состоянии.
func (m *FileMetadata) IsActive() bool {
	return m.Status == StatusActive
}
