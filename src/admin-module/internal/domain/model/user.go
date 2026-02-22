// Пакет model — доменные модели Admin Module.
package model

import "time"

// AdminUser — пользователь из Keycloak с локальными дополнениями.
// Не хранится в БД — формируется из данных Keycloak + role_overrides.
type AdminUser struct {
	// ID — Keycloak user ID (sub)
	ID string
	// Username — имя пользователя в Keycloak
	Username string
	// Email — адрес электронной почты
	Email string
	// FirstName — имя
	FirstName string
	// LastName — фамилия
	LastName string
	// Enabled — активен ли аккаунт в Keycloak
	Enabled bool
	// Groups — группы пользователя из IdP
	Groups []string
	// IdpRole — роль из IdP (admin, readonly)
	IdpRole string
	// RoleOverride — локальное дополнение роли (admin, readonly, nil если нет)
	RoleOverride *string
	// EffectiveRole — итоговая роль = max(IdpRole, RoleOverride)
	EffectiveRole string
	// CreatedAt — дата создания в Keycloak
	CreatedAt time.Time
}

// RoleOverride — локальное дополнение роли пользователя.
// Хранится в таблице role_overrides.
type RoleOverride struct {
	// ID — UUID записи
	ID string
	// KeycloakUserID — идентификатор пользователя в Keycloak (sub)
	KeycloakUserID string
	// Username — кэшированное имя пользователя
	Username string
	// AdditionalRole — дополнительная роль (admin, readonly)
	AdditionalRole string
	// CreatedBy — кто установил override (username администратора)
	CreatedBy string
	// CreatedAt — время создания записи
	CreatedAt time.Time
	// UpdatedAt — время последнего обновления
	UpdatedAt time.Time
}
