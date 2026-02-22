package model

import "time"

// ServiceAccount — сервисный аккаунт.
// Хранится в таблице service_accounts.
type ServiceAccount struct {
	// ID — UUID записи
	ID string
	// KeycloakClientID — UUID client в Keycloak (может быть nil для несинхронизированных)
	KeycloakClientID *string
	// ClientID — идентификатор для аутентификации (формат sa_<name>_<random>)
	ClientID string
	// Name — человекочитаемое имя
	Name string
	// Description — описание (опционально)
	Description *string
	// Scopes — разрешённые scopes (files:read, files:write, storage:read, ...)
	Scopes []string
	// Status — статус (active, suspended)
	Status string
	// Source — источник создания (local — через Admin Module, keycloak — из Keycloak)
	Source string
	// LastSyncedAt — время последней синхронизации с Keycloak
	LastSyncedAt *time.Time
	// CreatedAt — время создания записи
	CreatedAt time.Time
	// UpdatedAt — время последнего обновления
	UpdatedAt time.Time
}
