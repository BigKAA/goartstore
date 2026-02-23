// mode.go — обработчик POST /api/v1/mode/transition.
// Смена режима работы Storage Element (rw→ro→ar, ro→rw с confirm).
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/arturkryukov/artstore/storage-element/internal/api/errors"
	"github.com/arturkryukov/artstore/storage-element/internal/api/generated"
	"github.com/arturkryukov/artstore/storage-element/internal/api/middleware"
	"github.com/arturkryukov/artstore/storage-element/internal/domain/mode"
)

// ModePersister — интерфейс для сохранения режима в mode.json (replicated mode).
type ModePersister interface {
	SaveMode(m mode.StorageMode) error
}

// ModeHandler — обработчик endpoint смены режима.
type ModeHandler struct {
	sm            *mode.StateMachine
	logger        *slog.Logger
	modePersister ModePersister
}

// NewModeHandler создаёт обработчик смены режима.
// modePersister — сохранение mode.json (nil для standalone).
func NewModeHandler(sm *mode.StateMachine, logger *slog.Logger, modePersister ModePersister) *ModeHandler {
	return &ModeHandler{
		sm:            sm,
		logger:        logger.With(slog.String("component", "mode_handler")),
		modePersister: modePersister,
	}
}

// TransitionMode обрабатывает POST /api/v1/mode/transition.
// Требует scope storage:write.
func (h *ModeHandler) TransitionMode(w http.ResponseWriter, r *http.Request) {
	// Парсим тело запроса
	var req generated.ModeTransitionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.ValidationError(w, "Некорректный JSON: "+err.Error())
		return
	}

	// Извлекаем subject из JWT
	subject := middleware.SubjectFromContext(r.Context())

	// Преобразуем target_mode из API в доменную модель
	targetMode := mode.StorageMode(req.TargetMode)

	// Получаем текущий режим до перехода
	previousMode := h.sm.CurrentMode()

	// Определяем confirm
	confirm := false
	if req.Confirm != nil {
		confirm = *req.Confirm
	}

	// Выполняем переход
	err := h.sm.TransitionTo(targetMode, confirm, subject)
	if err != nil {
		// Обрабатываем ошибки перехода
		if transErr, ok := err.(*mode.TransitionError); ok {
			switch transErr.Code {
			case "CONFIRMATION_REQUIRED":
				errors.ConfirmationRequired(w, transErr.Message)
			case "INVALID_TRANSITION":
				errors.InvalidTransition(w, transErr.Message)
			default:
				errors.InvalidTransition(w, transErr.Message)
			}
			return
		}
		errors.InternalError(w, "Ошибка смены режима")
		return
	}

	// Сохраняем mode.json (replicated mode — leader записывает при смене режима)
	if h.modePersister != nil {
		if persistErr := h.modePersister.SaveMode(targetMode); persistErr != nil {
			h.logger.Error("Ошибка сохранения mode.json",
				slog.String("error", persistErr.Error()),
			)
			// Режим уже изменён в памяти — не откатываем, но логируем
		}
	}

	now := time.Now().UTC()

	// Логируем переход
	h.logger.Info("Режим изменён",
		slog.String("from", string(previousMode)),
		slog.String("to", string(targetMode)),
		slog.String("subject", subject),
		slog.Time("at", now),
	)

	// Формируем ответ
	resp := generated.ModeTransitionResponse{
		PreviousMode:   generated.ModeTransitionResponsePreviousMode(previousMode),
		CurrentMode:    generated.ModeTransitionResponseCurrentMode(targetMode),
		TransitionedAt: now,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
