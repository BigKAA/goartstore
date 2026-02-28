// Пакет model — доменные модели Query Module.
// FileRecord — маппинг таблицы file_registry (owned by Admin Module).
package model

import "time"

// FileRecord — запись файла в реестре file_registry.
// QM использует эту модель только для чтения (+ обновление статуса при lazy cleanup).
// Структура полностью совместима с Admin Module FileRecord.
type FileRecord struct {
	// FileID — UUID файла (задаётся при загрузке через Admin Module)
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
	// UploadedBy — идентификатор загрузившего (sub из JWT)
	UploadedBy string
	// UploadedAt — время загрузки
	UploadedAt time.Time
	// Description — описание файла (опционально)
	Description *string
	// Tags — теги файла (массив строк)
	Tags []string
	// Status — статус файла: active, deleted, expired
	Status string
	// RetentionPolicy — политика хранения: permanent, temporary
	RetentionPolicy string
	// TTLDays — время жизни в днях (для temporary файлов)
	TTLDays *int
	// ExpiresAt — время истечения (для temporary файлов)
	ExpiresAt *time.Time
	// CreatedAt — время создания записи
	CreatedAt time.Time
	// UpdatedAt — время последнего обновления
	UpdatedAt time.Time
}
