package mode

import (
	"errors"
	"sync"
	"testing"
)

// TestNewStateMachine проверяет создание конечного автомата.
func TestNewStateMachine(t *testing.T) {
	tests := []struct {
		mode    StorageMode
		wantErr bool
	}{
		{ModeEdit, false},
		{ModeRW, false},
		{ModeRO, false},
		{ModeAR, false},
		{StorageMode("invalid"), true},
		{StorageMode(""), true},
	}

	for _, tt := range tests {
		sm, err := NewStateMachine(tt.mode)
		if tt.wantErr {
			if err == nil {
				t.Errorf("NewStateMachine(%q): ожидалась ошибка", tt.mode)
			}
			continue
		}
		if err != nil {
			t.Errorf("NewStateMachine(%q): неожиданная ошибка: %v", tt.mode, err)
			continue
		}
		if sm.CurrentMode() != tt.mode {
			t.Errorf("CurrentMode(): ожидалось %q, получено %q", tt.mode, sm.CurrentMode())
		}
	}
}

// TestTransitions_EditIsolated проверяет, что edit — изолированный режим.
func TestTransitions_EditIsolated(t *testing.T) {
	sm, _ := NewStateMachine(ModeEdit)

	targets := []StorageMode{ModeRW, ModeRO, ModeAR}
	for _, target := range targets {
		if sm.CanTransitionTo(target) {
			t.Errorf("edit → %s не должен быть допустим", target)
		}
		err := sm.TransitionTo(target, false, "admin")
		if err == nil {
			t.Errorf("edit → %s должен вернуть ошибку", target)
		}
		var te *TransitionError
		if errors.As(err, &te) {
			if te.Code != "INVALID_TRANSITION" {
				t.Errorf("ожидался код INVALID_TRANSITION, получен %q", te.Code)
			}
		}
	}
}

// TestTransitions_RWToRO проверяет штатный переход rw → ro.
func TestTransitions_RWToRO(t *testing.T) {
	sm, _ := NewStateMachine(ModeRW)

	if !sm.CanTransitionTo(ModeRO) {
		t.Error("rw → ro должен быть допустим")
	}

	err := sm.TransitionTo(ModeRO, false, "admin")
	if err != nil {
		t.Fatalf("rw → ro: неожиданная ошибка: %v", err)
	}

	if sm.CurrentMode() != ModeRO {
		t.Errorf("ожидался режим ro, получен %q", sm.CurrentMode())
	}
}

// TestTransitions_ROToAR проверяет штатный переход ro → ar.
func TestTransitions_ROToAR(t *testing.T) {
	sm, _ := NewStateMachine(ModeRW)
	sm.TransitionTo(ModeRO, false, "admin")

	if !sm.CanTransitionTo(ModeAR) {
		t.Error("ro → ar должен быть допустим")
	}

	err := sm.TransitionTo(ModeAR, false, "admin")
	if err != nil {
		t.Fatalf("ro → ar: неожиданная ошибка: %v", err)
	}

	if sm.CurrentMode() != ModeAR {
		t.Errorf("ожидался режим ar, получен %q", sm.CurrentMode())
	}
}

// TestTransitions_ROToRW_WithConfirm проверяет откат ro → rw с подтверждением.
func TestTransitions_ROToRW_WithConfirm(t *testing.T) {
	sm, _ := NewStateMachine(ModeRW)
	sm.TransitionTo(ModeRO, false, "admin")

	// Без confirm — ошибка
	if !sm.NeedsConfirmation(ModeRW) {
		t.Error("ro → rw должен требовать подтверждения")
	}

	err := sm.TransitionTo(ModeRW, false, "admin")
	if err == nil {
		t.Fatal("ro → rw без confirm должен вернуть ошибку")
	}

	var te *TransitionError
	if !errors.As(err, &te) {
		t.Fatalf("ожидалась TransitionError, получена %T", err)
	}
	if te.Code != "CONFIRMATION_REQUIRED" {
		t.Errorf("ожидался код CONFIRMATION_REQUIRED, получен %q", te.Code)
	}

	// Режим не должен измениться
	if sm.CurrentMode() != ModeRO {
		t.Errorf("режим не должен измениться без confirm, текущий: %q", sm.CurrentMode())
	}

	// С confirm — успешно
	err = sm.TransitionTo(ModeRW, true, "admin")
	if err != nil {
		t.Fatalf("ro → rw с confirm: неожиданная ошибка: %v", err)
	}

	if sm.CurrentMode() != ModeRW {
		t.Errorf("ожидался режим rw, получен %q", sm.CurrentMode())
	}
}

// TestTransitions_ARFinal проверяет, что ar — конечный режим.
func TestTransitions_ARFinal(t *testing.T) {
	sm, _ := NewStateMachine(ModeRW)
	sm.TransitionTo(ModeRO, false, "admin")
	sm.TransitionTo(ModeAR, false, "admin")

	targets := []StorageMode{ModeEdit, ModeRW, ModeRO}
	for _, target := range targets {
		if sm.CanTransitionTo(target) {
			t.Errorf("ar → %s не должен быть допустим", target)
		}
	}
}

// TestTransitions_ForbiddenSkip проверяет запрет пропуска режимов.
func TestTransitions_ForbiddenSkip(t *testing.T) {
	sm, _ := NewStateMachine(ModeRW)

	// rw → ar — пропуск ro запрещён
	if sm.CanTransitionTo(ModeAR) {
		t.Error("rw → ar (пропуск ro) не должен быть допустим")
	}

	err := sm.TransitionTo(ModeAR, false, "admin")
	if err == nil {
		t.Error("rw → ar должен вернуть ошибку")
	}
}

// TestTransitions_ForbiddenToEdit проверяет запрет перехода в edit из любого режима.
func TestTransitions_ForbiddenToEdit(t *testing.T) {
	modes := []StorageMode{ModeRW, ModeRO, ModeAR}

	for _, m := range modes {
		sm, _ := NewStateMachine(m)
		if sm.CanTransitionTo(ModeEdit) {
			t.Errorf("%s → edit не должен быть допустим", m)
		}
	}
}

// TestAllowedOperations проверяет матрицу операций для каждого режима.
func TestAllowedOperations(t *testing.T) {
	tests := []struct {
		mode     StorageMode
		allowed  []Operation
		disallow []Operation
	}{
		{
			mode:     ModeEdit,
			allowed:  []Operation{OpUpload, OpDownload, OpDelete, OpUpdate, OpList},
			disallow: nil,
		},
		{
			mode:     ModeRW,
			allowed:  []Operation{OpUpload, OpDownload, OpUpdate, OpList},
			disallow: []Operation{OpDelete},
		},
		{
			mode:     ModeRO,
			allowed:  []Operation{OpDownload, OpList},
			disallow: []Operation{OpUpload, OpDelete, OpUpdate},
		},
		{
			mode:     ModeAR,
			allowed:  []Operation{OpList},
			disallow: []Operation{OpUpload, OpDownload, OpDelete, OpUpdate},
		},
	}

	for _, tt := range tests {
		sm, _ := NewStateMachine(tt.mode)

		for _, op := range tt.allowed {
			if !sm.CanPerform(op) {
				t.Errorf("режим %s: операция %s должна быть допустима", tt.mode, op)
			}
		}

		for _, op := range tt.disallow {
			if sm.CanPerform(op) {
				t.Errorf("режим %s: операция %s не должна быть допустима", tt.mode, op)
			}
		}
	}
}

// TestAllowedOperations_List проверяет формат списка операций.
func TestAllowedOperations_List(t *testing.T) {
	sm, _ := NewStateMachine(ModeEdit)

	ops := sm.AllowedOperations()
	if len(ops) != 5 {
		t.Errorf("edit: ожидалось 5 операций, получено %d", len(ops))
	}

	sm2, _ := NewStateMachine(ModeAR)
	ops2 := sm2.AllowedOperations()
	if len(ops2) != 1 {
		t.Errorf("ar: ожидалось 1 операция, получено %d", len(ops2))
	}
}

// TestHistory проверяет запись истории переходов.
func TestHistory(t *testing.T) {
	sm, _ := NewStateMachine(ModeRW)

	// Начальная история — пустая
	if len(sm.History()) != 0 {
		t.Error("начальная история должна быть пустой")
	}

	// rw → ro
	sm.TransitionTo(ModeRO, false, "user1")
	// ro → rw (с confirm)
	sm.TransitionTo(ModeRW, true, "admin")

	history := sm.History()
	if len(history) != 2 {
		t.Fatalf("ожидалось 2 записи в истории, получено %d", len(history))
	}

	// Проверяем первый переход
	if history[0].From != ModeRW || history[0].To != ModeRO {
		t.Errorf("запись 0: ожидалось rw→ro, получено %s→%s", history[0].From, history[0].To)
	}
	if history[0].Subject != "user1" {
		t.Errorf("запись 0: ожидался subject 'user1', получен %q", history[0].Subject)
	}

	// Проверяем второй переход
	if history[1].From != ModeRO || history[1].To != ModeRW {
		t.Errorf("запись 1: ожидалось ro→rw, получено %s→%s", history[1].From, history[1].To)
	}
	if history[1].Subject != "admin" {
		t.Errorf("запись 1: ожидался subject 'admin', получен %q", history[1].Subject)
	}
}

// TestHistory_Immutable проверяет, что History возвращает копию.
func TestHistory_Immutable(t *testing.T) {
	sm, _ := NewStateMachine(ModeRW)
	sm.TransitionTo(ModeRO, false, "admin")

	h1 := sm.History()
	h1[0].Subject = "modified"

	h2 := sm.History()
	if h2[0].Subject == "modified" {
		t.Error("History должна возвращать копию, а не ссылку")
	}
}

// TestConcurrentAccess проверяет потокобезопасность конечного автомата.
func TestConcurrentAccess(t *testing.T) {
	sm, _ := NewStateMachine(ModeRW)

	var wg sync.WaitGroup
	const goroutines = 50

	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()

			// Параллельное чтение
			_ = sm.CurrentMode()
			_ = sm.CanTransitionTo(ModeRO)
			_ = sm.CanPerform(OpUpload)
			_ = sm.AllowedOperations()
			_ = sm.History()
		}()
	}

	wg.Wait()
}

// TestParseMode проверяет парсинг строки в StorageMode.
func TestParseMode(t *testing.T) {
	tests := []struct {
		input   string
		want    StorageMode
		wantErr bool
	}{
		{"edit", ModeEdit, false},
		{"rw", ModeRW, false},
		{"ro", ModeRO, false},
		{"ar", ModeAR, false},
		{"invalid", "", true},
		{"", "", true},
		{"RW", "", true}, // Case-sensitive
	}

	for _, tt := range tests {
		got, err := ParseMode(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseMode(%q): ожидалась ошибка", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseMode(%q): неожиданная ошибка: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseMode(%q): ожидалось %q, получено %q", tt.input, tt.want, got)
		}
	}
}

// TestFullLifecycle_Permanent проверяет полный жизненный цикл permanent storage.
func TestFullLifecycle_Permanent(t *testing.T) {
	sm, _ := NewStateMachine(ModeRW)

	// rw → ro → ar
	steps := []struct {
		target  StorageMode
		confirm bool
	}{
		{ModeRO, false},
		{ModeAR, false},
	}

	for _, step := range steps {
		err := sm.TransitionTo(step.target, step.confirm, "admin")
		if err != nil {
			t.Fatalf("переход → %s: %v", step.target, err)
		}
	}

	if sm.CurrentMode() != ModeAR {
		t.Errorf("конечный режим: ожидался ar, получен %q", sm.CurrentMode())
	}

	if len(sm.History()) != 2 {
		t.Errorf("ожидалось 2 записи в истории, получено %d", len(sm.History()))
	}
}

// TestFullLifecycle_WithRollback проверяет жизненный цикл с откатом.
func TestFullLifecycle_WithRollback(t *testing.T) {
	sm, _ := NewStateMachine(ModeRW)

	// rw → ro → (откат) rw → ro → ar
	sm.TransitionTo(ModeRO, false, "admin")
	sm.TransitionTo(ModeRW, true, "admin") // откат
	sm.TransitionTo(ModeRO, false, "admin")
	sm.TransitionTo(ModeAR, false, "admin")

	if sm.CurrentMode() != ModeAR {
		t.Errorf("конечный режим: ожидался ar, получен %q", sm.CurrentMode())
	}

	if len(sm.History()) != 4 {
		t.Errorf("ожидалось 4 записи в истории, получено %d", len(sm.History()))
	}
}

// TestNeedsConfirmation проверяет определение необходимости подтверждения.
func TestNeedsConfirmation(t *testing.T) {
	tests := []struct {
		current StorageMode
		target  StorageMode
		needs   bool
	}{
		{ModeRO, ModeRW, true},  // Единственный случай
		{ModeRW, ModeRO, false}, // Штатный переход
		{ModeRO, ModeAR, false}, // Штатный переход
		{ModeEdit, ModeRW, false},
	}

	for _, tt := range tests {
		sm, _ := NewStateMachine(tt.current)
		if sm.NeedsConfirmation(tt.target) != tt.needs {
			t.Errorf("%s → %s: NeedsConfirmation = %v, ожидалось %v",
				tt.current, tt.target, !tt.needs, tt.needs)
		}
	}
}
