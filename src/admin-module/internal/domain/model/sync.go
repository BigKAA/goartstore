package model

import "time"

// SyncState — состояние синхронизации (одна строка в БД).
// Хранится в таблице sync_state (id = 1, всегда одна запись).
type SyncState struct {
	// ID — всегда 1
	ID int
	// LastSASyncAt — время последней синхронизации SA с Keycloak
	LastSASyncAt *time.Time
	// LastFileSyncAt — время последней синхронизации файлового реестра
	LastFileSyncAt *time.Time
	// CreatedAt — время создания записи
	CreatedAt time.Time
	// UpdatedAt — время последнего обновления
	UpdatedAt time.Time
}

// SyncResult — результат синхронизации файлов с одним SE.
type SyncResult struct {
	// StorageElementID — UUID SE, с которым проводилась синхронизация
	StorageElementID string
	// FilesOnSE — общее количество файлов на SE
	FilesOnSE int
	// FilesAdded — новых файлов добавлено в реестр
	FilesAdded int
	// FilesUpdated — файлов обновлено
	FilesUpdated int
	// FilesMarkedDeleted — файлов помечено как deleted
	FilesMarkedDeleted int
	// StartedAt — время начала синхронизации
	StartedAt time.Time
	// CompletedAt — время завершения синхронизации
	CompletedAt time.Time
}

// SASyncResult — результат синхронизации SA с Keycloak.
type SASyncResult struct {
	// TotalLocal — общее количество SA в локальной БД
	TotalLocal int
	// TotalKeycloak — общее количество SA clients в Keycloak
	TotalKeycloak int
	// CreatedLocal — новых SA создано локально (обнаружены в Keycloak)
	CreatedLocal int
	// CreatedKeycloak — новых clients создано в Keycloak (обнаружены локально)
	CreatedKeycloak int
	// Updated — SA обновлены (расхождение scopes)
	Updated int
	// SyncedAt — время синхронизации
	SyncedAt time.Time
}
