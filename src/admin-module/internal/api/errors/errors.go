// Пакет errors — конструкторы стандартных ошибок в формате Artstore.
// Единый формат: {"error": {"code": "...", "message": "..."}}.
// Все HTTP-ответы с ошибками должны использовать WriteError.
package errors

import (
	"encoding/json"
	"net/http"
)

// Коды ошибок, определённые в OpenAPI контракте.
const (
	CodeValidationError = "VALIDATION_ERROR"
	CodeNotFound        = "NOT_FOUND"
	CodeUnauthorized    = "UNAUTHORIZED"
	CodeForbidden       = "FORBIDDEN"
	CodeConflict        = "CONFLICT"
	CodeSEUnavailable   = "SE_UNAVAILABLE"
	CodeIDPUnavailable  = "IDP_UNAVAILABLE"
	CodeInternalError   = "INTERNAL_ERROR"
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

// Conflict — 409 конфликт (дублирующийся ресурс).
func Conflict(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusConflict, CodeConflict, message)
}

// SEUnavailable — 502 Storage Element недоступен.
func SEUnavailable(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadGateway, CodeSEUnavailable, message)
}

// IDPUnavailable — 502 Identity Provider (Keycloak) недоступен.
func IDPUnavailable(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadGateway, CodeIDPUnavailable, message)
}

// InternalError — 500 внутренняя ошибка.
func InternalError(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusInternalServerError, CodeInternalError, message)
}
