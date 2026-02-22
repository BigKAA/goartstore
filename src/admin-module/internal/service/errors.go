// errors.go — ошибки бизнес-логики сервисного слоя.
package service

import "errors"

var (
	// ErrNotFound — ресурс не найден.
	ErrNotFound = errors.New("ресурс не найден")
	// ErrConflict — конфликт (дублирующийся ресурс).
	ErrConflict = errors.New("конфликт — ресурс уже существует")
	// ErrInvalidRole — некорректная роль.
	ErrInvalidRole = errors.New("некорректная роль: допустимые значения — admin, readonly")
	// ErrSEUnavailable — Storage Element недоступен.
	ErrSEUnavailable = errors.New("Storage Element недоступен")
	// ErrIDPUnavailable — Identity Provider (Keycloak) недоступен.
	ErrIDPUnavailable = errors.New("Identity Provider недоступен")
	// ErrValidation — ошибка валидации входных данных.
	ErrValidation = errors.New("ошибка валидации")
)
