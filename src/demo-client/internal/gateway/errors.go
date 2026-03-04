// Пакет gateway предоставляет HTTP-клиент для взаимодействия с API Gateway artstore.
// Типизированные ошибки для маппинга HTTP-кодов в user-friendly сообщения на уровне UI.
package gateway

import (
	"errors"
	"fmt"
)

// Типизированные ошибки Gateway.
// Используются для маппинга HTTP-кодов от IM/QM в user-friendly сообщения на UI.
var (
	// ErrFileArchived — файл находится в архивном хранилище (SE mode=ar), скачивание недоступно.
	// HTTP 410 Gone от Query Module.
	ErrFileArchived = errors.New("file is archived and not available for download")

	// ErrNotFound — файл не найден в реестре или удалён.
	// HTTP 404 Not Found.
	ErrNotFound = errors.New("file not found")

	// ErrStorageFull — нет свободного места на доступных Storage Elements.
	// HTTP 507 Insufficient Storage.
	ErrStorageFull = errors.New("storage is full, no space available")

	// ErrFileTooLarge — размер файла превышает допустимый лимит.
	// HTTP 413 Payload Too Large.
	ErrFileTooLarge = errors.New("file size exceeds maximum allowed limit")

	// ErrServiceUnavailable — один из backend-сервисов недоступен.
	// HTTP 502 Bad Gateway или 503 Service Unavailable.
	ErrServiceUnavailable = errors.New("service is temporarily unavailable")

	// ErrUnauthorized — невалидный или отсутствующий JWT токен.
	// HTTP 401 Unauthorized.
	ErrUnauthorized = errors.New("unauthorized: invalid or missing token")

	// ErrForbidden — недостаточно прав для выполнения операции.
	// HTTP 403 Forbidden.
	ErrForbidden = errors.New("forbidden: insufficient permissions")

	// ErrValidation — ошибка валидации входных данных.
	// HTTP 400 Bad Request.
	ErrValidation = errors.New("validation error")

	// ErrModeNotAllowed — SE не в разрешённом режиме для удаления.
	// HTTP 409 Conflict (CODE: MODE_NOT_ALLOWED).
	ErrModeNotAllowed = errors.New("delete not allowed: storage element is not in edit mode")

	// ErrFileUploadInProgress — файл в процессе загрузки, удаление невозможно.
	// HTTP 409 Conflict (CODE: FILE_UPLOAD_IN_PROGRESS).
	ErrFileUploadInProgress = errors.New("delete not allowed: file upload is in progress")

	// ErrConflict — общая ошибка конфликта (409) без конкретного кода.
	ErrConflict = errors.New("conflict: operation cannot be completed")
)

// Error оборачивает типизированную ошибку с дополнительным контекстом от API.
type Error struct {
	// Err — базовая типизированная ошибка (ErrNotFound, ErrFileArchived и т.д.)
	Err error
	// Code — код ошибки от API (например, "FILE_ARCHIVED", "NOT_FOUND")
	Code string
	// Message — человекочитаемое сообщение от API
	Message string
	// StatusCode — HTTP статус код ответа
	StatusCode int
}

// Error возвращает строковое представление ошибки.
func (e *Error) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s: %s (HTTP %d)", e.Code, e.Message, e.StatusCode)
	}
	return fmt.Sprintf("%s (HTTP %d)", e.Code, e.StatusCode)
}

// Unwrap возвращает базовую типизированную ошибку для проверки через errors.Is().
func (e *Error) Unwrap() error {
	return e.Err
}

// mapHTTPError маппит HTTP статус код и тело ответа в типизированную ошибку.
func mapHTTPError(statusCode int, apiErr *ErrorResponse) error {
	var code, message string
	if apiErr != nil && apiErr.Error != nil {
		code = apiErr.Error.Code
		message = apiErr.Error.Message
	}

	var baseErr error
	switch statusCode {
	case 400:
		baseErr = ErrValidation
	case 401:
		baseErr = ErrUnauthorized
	case 403:
		baseErr = ErrForbidden
	case 404:
		baseErr = ErrNotFound
	case 410:
		baseErr = ErrFileArchived
	case 409:
		// Маппим 409 Conflict по коду ошибки API
		if code == "MODE_NOT_ALLOWED" {
			baseErr = ErrModeNotAllowed
		} else if code == "FILE_UPLOAD_IN_PROGRESS" {
			baseErr = ErrFileUploadInProgress
		} else {
			baseErr = ErrConflict
		}
	case 413:
		baseErr = ErrFileTooLarge
	case 502, 503:
		baseErr = ErrServiceUnavailable
	case 507:
		baseErr = ErrStorageFull
	default:
		baseErr = fmt.Errorf("unexpected HTTP status %d", statusCode)
	}

	return &Error{
		Err:        baseErr,
		Code:       code,
		Message:    message,
		StatusCode: statusCode,
	}
}
