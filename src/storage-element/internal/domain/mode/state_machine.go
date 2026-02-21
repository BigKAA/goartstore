// Пакет mode — конечный автомат режимов работы Storage Element.
//
// Два жизненных цикла:
//   - edit — изолированный режим (temporary storage), переходы невозможны
//   - rw → ro → ar — жизненный цикл permanent storage
//     Единственный обратный переход: ro → rw (требует confirm: true)
//
// Потокобезопасен через sync.RWMutex.
package mode

import (
	"fmt"
	"sync"
	"time"
)

// StorageMode — режим работы Storage Element.
type StorageMode string

const (
	// ModeEdit — полный CRUD, temporary storage (изолированный)
	ModeEdit StorageMode = "edit"
	// ModeRW — чтение/запись, permanent storage
	ModeRW StorageMode = "rw"
	// ModeRO — только чтение
	ModeRO StorageMode = "ro"
	// ModeAR — архив (только метаданные)
	ModeAR StorageMode = "ar"
)

// Operation — операция над файлами.
type Operation string

const (
	OpUpload   Operation = "upload"
	OpDownload Operation = "download"
	OpDelete   Operation = "delete"
	OpUpdate   Operation = "update"
	OpList     Operation = "list"
)

// TransitionRecord — запись о переходе между режимами.
type TransitionRecord struct {
	From      StorageMode `json:"from"`
	To        StorageMode `json:"to"`
	Subject   string      `json:"subject"`
	Timestamp time.Time   `json:"timestamp"`
}

// StateMachine — конечный автомат режимов работы SE.
// Потокобезопасен для одновременного чтения/записи.
type StateMachine struct {
	mu      sync.RWMutex
	current StorageMode
	history []TransitionRecord
}

// validTransitions — матрица допустимых переходов.
// Ключ — текущий режим, значение — набор допустимых целевых режимов.
// Переход ro → rw помечен отдельно как требующий подтверждения.
var validTransitions = map[StorageMode]map[StorageMode]bool{
	ModeEdit: {}, // Изолированный режим — переходы запрещены
	ModeRW:   {ModeRO: true},
	ModeRO:   {ModeAR: true, ModeRW: true}, // ro → rw — откат с confirm
	ModeAR:   {},                            // Конечный режим — переходы запрещены
}

// allowedOperations — матрица допустимых операций для каждого режима.
var allowedOperations = map[StorageMode]map[Operation]bool{
	ModeEdit: {OpUpload: true, OpDownload: true, OpDelete: true, OpUpdate: true, OpList: true},
	ModeRW:   {OpUpload: true, OpDownload: true, OpUpdate: true, OpList: true},
	ModeRO:   {OpDownload: true, OpList: true},
	ModeAR:   {OpList: true},
}

// needsConfirmation — переходы, требующие явного подтверждения.
var needsConfirmation = map[StorageMode]map[StorageMode]bool{
	ModeRO: {ModeRW: true}, // Единственный обратный переход
}

// NewStateMachine создаёт конечный автомат с начальным режимом.
// Возвращает ошибку, если режим невалидный.
func NewStateMachine(initial StorageMode) (*StateMachine, error) {
	if !isValidMode(initial) {
		return nil, fmt.Errorf("недопустимый начальный режим: %q", initial)
	}

	return &StateMachine{
		current: initial,
		history: make([]TransitionRecord, 0),
	}, nil
}

// CurrentMode возвращает текущий режим работы.
func (sm *StateMachine) CurrentMode() StorageMode {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.current
}

// CanTransitionTo проверяет, допустим ли переход в указанный режим.
// Не проверяет необходимость подтверждения (confirm).
func (sm *StateMachine) CanTransitionTo(target StorageMode) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	transitions, ok := validTransitions[sm.current]
	if !ok {
		return false
	}
	return transitions[target]
}

// NeedsConfirmation проверяет, требует ли переход подтверждения.
// Возвращает true для перехода ro → rw.
func (sm *StateMachine) NeedsConfirmation(target StorageMode) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	confirms, ok := needsConfirmation[sm.current]
	if !ok {
		return false
	}
	return confirms[target]
}

// TransitionTo выполняет переход в указанный режим.
//
// Параметры:
//   - target: целевой режим
//   - confirm: подтверждение обратного перехода (true для ro → rw)
//   - subject: кто инициировал переход (sub из JWT)
//
// Ошибки:
//   - INVALID_TRANSITION — переход недопустим
//   - CONFIRMATION_REQUIRED — требуется confirm: true
func (sm *StateMachine) TransitionTo(target StorageMode, confirm bool, subject string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if !isValidMode(target) {
		return &TransitionError{
			Code:    "INVALID_TRANSITION",
			Message: fmt.Sprintf("недопустимый целевой режим: %q", target),
		}
	}

	// Проверяем допустимость перехода
	transitions, ok := validTransitions[sm.current]
	if !ok || !transitions[target] {
		return &TransitionError{
			Code: "INVALID_TRANSITION",
			Message: fmt.Sprintf("переход %s → %s недопустим",
				sm.current, target),
		}
	}

	// Проверяем необходимость подтверждения
	if confirms, ok := needsConfirmation[sm.current]; ok && confirms[target] {
		if !confirm {
			return &TransitionError{
				Code: "CONFIRMATION_REQUIRED",
				Message: fmt.Sprintf("обратный переход %s → %s требует подтверждения (confirm: true)",
					sm.current, target),
			}
		}
	}

	// Выполняем переход
	record := TransitionRecord{
		From:      sm.current,
		To:        target,
		Subject:   subject,
		Timestamp: time.Now().UTC(),
	}

	sm.current = target
	sm.history = append(sm.history, record)

	return nil
}

// AllowedOperations возвращает список операций, доступных в текущем режиме.
func (sm *StateMachine) AllowedOperations() []Operation {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	ops, ok := allowedOperations[sm.current]
	if !ok {
		return nil
	}

	result := make([]Operation, 0, len(ops))
	for op := range ops {
		result = append(result, op)
	}
	return result
}

// CanPerform проверяет, допустима ли операция в текущем режиме.
func (sm *StateMachine) CanPerform(op Operation) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	ops, ok := allowedOperations[sm.current]
	if !ok {
		return false
	}
	return ops[op]
}

// ForceMode устанавливает режим напрямую без валидации переходов.
// Используется follower в replicated mode для синхронизации режима из mode.json.
// Потокобезопасен (под мьютексом).
func (sm *StateMachine) ForceMode(target StorageMode) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.current = target
}

// History возвращает историю переходов (копия).
func (sm *StateMachine) History() []TransitionRecord {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make([]TransitionRecord, len(sm.history))
	copy(result, sm.history)
	return result
}

// TransitionError — ошибка перехода между режимами.
type TransitionError struct {
	Code    string // Машиночитаемый код (INVALID_TRANSITION, CONFIRMATION_REQUIRED)
	Message string // Человекочитаемое описание
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// isValidMode проверяет, является ли строка допустимым режимом.
func isValidMode(m StorageMode) bool {
	switch m {
	case ModeEdit, ModeRW, ModeRO, ModeAR:
		return true
	default:
		return false
	}
}

// ParseMode преобразует строку в StorageMode.
// Возвращает ошибку для недопустимых значений.
func ParseMode(s string) (StorageMode, error) {
	m := StorageMode(s)
	if !isValidMode(m) {
		return "", fmt.Errorf("недопустимый режим: %q, допустимые: edit, rw, ro, ar", s)
	}
	return m, nil
}
