package model

import "time"

// StorageElement — зарегистрированный Storage Element.
// Хранится в таблице storage_elements.
type StorageElement struct {
	// ID — UUID записи
	ID string
	// Name — человекочитаемое имя SE
	Name string
	// URL — адрес SE (https://se1.example.com)
	URL string
	// StorageID — идентификатор SE (из SE /api/v1/info)
	StorageID string
	// Mode — режим работы (edit, rw, ro, ar)
	Mode string
	// Status — статус (online, offline, degraded, maintenance)
	Status string
	// CapacityBytes — общая ёмкость в байтах
	CapacityBytes int64
	// UsedBytes — использованное пространство в байтах
	UsedBytes int64
	// AvailableBytes — доступное пространство (может быть nil)
	AvailableBytes *int64
	// LastSyncAt — время последней синхронизации info
	LastSyncAt *time.Time
	// LastFileSyncAt — время последней синхронизации файлов
	LastFileSyncAt *time.Time
	// CreatedAt — время создания записи
	CreatedAt time.Time
	// UpdatedAt — время последнего обновления
	UpdatedAt time.Time
}
