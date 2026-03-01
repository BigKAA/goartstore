// Пакет errors — конструкторы стандартных ошибок в формате Artstore.
// Единый формат: {"error": {"code": "...", "message": "..."}}.
// Все HTTP-ответы с ошибками должны использовать WriteError.
package errors //nolint:revive // TODO: переименовать пакет errors, конфликт со stdlib

import (
	"encoding/json"
	"net/http"
)

// Коды ошибок, определённые в OpenAPI контракте Ingester Module.
const (
	CodeValidationError    = "VALIDATION_ERROR"
	CodeUnauthorized       = "UNAUTHORIZED"
	CodeForbidden          = "FORBIDDEN"
	CodeFileTooLarge       = "FILE_TOO_LARGE"
	CodeNoStorageAvailable = "NO_STORAGE_AVAILABLE"
	CodeSEUploadFailed     = "SE_UPLOAD_FAILED"
	CodeAdminUnavailable   = "ADMIN_UNAVAILABLE"
	CodeStorageFull        = "STORAGE_FULL"
	CodeInternalError      = "INTERNAL_ERROR"
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

// Unauthorized — 401 требуется аутентификация.
func Unauthorized(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusUnauthorized, CodeUnauthorized, message)
}

// Forbidden — 403 недостаточно прав.
func Forbidden(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusForbidden, CodeForbidden, message)
}

// FileTooLarge — 413 файл превышает допустимый размер.
func FileTooLarge(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusRequestEntityTooLarge, CodeFileTooLarge, message)
}

// NoStorageAvailable — 502 нет доступных Storage Elements.
func NoStorageAvailable(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadGateway, CodeNoStorageAvailable, message)
}

// SEUploadFailed — 502 ошибка загрузки в Storage Element.
func SEUploadFailed(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadGateway, CodeSEUploadFailed, message)
}

// AdminUnavailable — 502 Admin Module недоступен.
func AdminUnavailable(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadGateway, CodeAdminUnavailable, message)
}

// StorageFull — 507 нет свободного места на доступных SE.
func StorageFull(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusInsufficientStorage, CodeStorageFull, message)
}

// InternalError — 500 внутренняя ошибка.
func InternalError(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusInternalServerError, CodeInternalError, message)
}
