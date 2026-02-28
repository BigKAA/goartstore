// Пакет errors — конструкторы стандартных ошибок в формате Artstore.
// Единый формат: {"error": {"code": "...", "message": "..."}}.
// Все HTTP-ответы с ошибками должны использовать WriteError.
package errors //nolint:revive // TODO: переименовать пакет errors, конфликт со stdlib

import (
	"encoding/json"
	"net/http"
)

// Коды ошибок, определённые в OpenAPI контракте.
const (
	CodeValidationError      = "VALIDATION_ERROR"
	CodeNotFound             = "NOT_FOUND"
	CodeUnauthorized         = "UNAUTHORIZED"
	CodeForbidden            = "FORBIDDEN"
	CodeModeNotAllowed       = "MODE_NOT_ALLOWED"
	CodeInvalidTransition    = "INVALID_TRANSITION"
	CodeConfirmationRequired = "CONFIRMATION_REQUIRED"
	CodeInvalidRange         = "INVALID_RANGE"
	CodeFileTooLarge         = "FILE_TOO_LARGE"
	CodeStorageFull          = "STORAGE_FULL"
	CodeReconcileInProgress  = "RECONCILE_IN_PROGRESS"
	CodeInternalError        = "INTERNAL_ERROR"
)

// errorBody — структура тела ответа ошибки.
type errorBody struct {
	Error errorDetail `json:"error"`
}

// errorDetail — детали ошибки.
type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteError записывает ответ ошибки в стандартном формате Artstore.
// statusCode — HTTP статус-код, code — машиночитаемый код, message — описание.
func WriteError(w http.ResponseWriter, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(errorBody{
		Error: errorDetail{
			Code:    code,
			Message: message,
		},
	})
}

// --- Конструкторы для типичных ошибок ---

// ValidationError — 400 некорректные входные данные.
func ValidationError(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadRequest, CodeValidationError, message)
}

// NotFound — 404 ресурс не найден.
func NotFound(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusNotFound, CodeNotFound, message)
}

// Unauthorized — 401 требуется аутентификация.
func Unauthorized(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusUnauthorized, CodeUnauthorized, message)
}

// Forbidden — 403 недостаточно прав.
func Forbidden(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusForbidden, CodeForbidden, message)
}

// ModeNotAllowed — 409 операция недоступна в текущем режиме.
func ModeNotAllowed(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusConflict, CodeModeNotAllowed, message)
}

// InvalidTransition — 409 недопустимый переход между режимами.
func InvalidTransition(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusConflict, CodeInvalidTransition, message)
}

// ConfirmationRequired — 409 обратный переход требует подтверждения.
func ConfirmationRequired(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusConflict, CodeConfirmationRequired, message)
}

// InvalidRange — 416 некорректный Range header.
func InvalidRange(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusRequestedRangeNotSatisfiable, CodeInvalidRange, message)
}

// FileTooLarge — 413 файл превышает лимит.
func FileTooLarge(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusRequestEntityTooLarge, CodeFileTooLarge, message)
}

// StorageFull — 507 нет свободного места.
func StorageFull(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusInsufficientStorage, CodeStorageFull, message)
}

// ReconcileInProgress — 409 сверка уже выполняется.
func ReconcileInProgress(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusConflict, CodeReconcileInProgress, message)
}

// InternalError — 500 внутренняя ошибка.
func InternalError(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusInternalServerError, CodeInternalError, message)
}
