package model

import "time"

// FileRecord — запись файла в реестре.
// Хранится в таблице file_registry.
type FileRecord struct {
	// FileID — UUID файла (задаётся при загрузке)
	FileID string
	// OriginalFilename — оригинальное имя файла
	OriginalFilename string
	// ContentType — MIME-тип файла
	ContentType string
	// Size — размер файла в байтах
	Size int64
	// Checksum — SHA-256 контрольная сумма
	Checksum string
	// StorageElementID — UUID Storage Element, на котором хранится файл
	StorageElementID string
	// UploadedBy — идентификатор загрузившего (username или client_id)
	UploadedBy string
	// UploadedAt — время загрузки
	UploadedAt time.Time
	// Description — описание файла (опционально)
	Description *string
	// Tags — теги файла
	Tags []string
	// Status — статус (active, deleted, expired)
	Status string
	// RetentionPolicy — политика хранения (permanent, temporary)
	RetentionPolicy string
	// TTLDays — время жизни в днях (для temporary)
	TTLDays *int
	// ExpiresAt — время истечения (для temporary)
	ExpiresAt *time.Time
	// CreatedAt — время создания записи
	CreatedAt time.Time
	// UpdatedAt — время последнего обновления
	UpdatedAt time.Time
}
