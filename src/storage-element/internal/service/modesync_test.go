package service

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/bigkaa/goartstore/storage-element/internal/domain/mode"
	"github.com/bigkaa/goartstore/storage-element/internal/modefile"
)

// setupModeSyncTestEnv создаёт тестовое окружение для ModeSyncService тестов.
// Возвращает: dataDir, modeFilePath, StateMachine (в режиме edit).
func setupModeSyncTestEnv(t *testing.T, initialMode mode.StorageMode) (string, string, *mode.StateMachine) {
	t.Helper()

	dir := t.TempDir()
	modeFilePath := modefile.ModeFilePath(dir)

	sm, err := mode.NewStateMachine(initialMode)
	if err != nil {
		t.Fatalf("Ошибка создания StateMachine: %v", err)
	}

	return dir, modeFilePath, sm
}

func TestModeSyncService_SyncOnce_NoModeFile(t *testing.T) {
	// mode.json не существует — SyncOnce не должен паниковать и не менять режим
	_, modeFilePath, sm := setupModeSyncTestEnv(t, mode.ModeEdit)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	svc := NewModeSyncService(modeFilePath, sm, 10*time.Second, logger)
	svc.SyncOnce()

	// Режим не изменился
	if sm.CurrentMode() != mode.ModeEdit {
		t.Errorf("Режим изменился без mode.json: хотели %s, получили %s", mode.ModeEdit, sm.CurrentMode())
	}
}

func TestModeSyncService_SyncOnce_SameMode(t *testing.T) {
	// mode.json существует с тем же режимом — ForceMode не вызывается
	_, modeFilePath, sm := setupModeSyncTestEnv(t, mode.ModeRW)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Записываем mode.json с тем же режимом
	if err := modefile.SaveMode(modeFilePath, mode.ModeRW, "test-host:8010"); err != nil {
		t.Fatalf("Ошибка записи mode.json: %v", err)
	}

	svc := NewModeSyncService(modeFilePath, sm, 10*time.Second, logger)
	svc.SyncOnce()

	if sm.CurrentMode() != mode.ModeRW {
		t.Errorf("Режим изменился при совпадении: хотели %s, получили %s", mode.ModeRW, sm.CurrentMode())
	}
}

func TestModeSyncService_SyncOnce_ModeChanged(t *testing.T) {
	// mode.json содержит другой режим — ForceMode должен обновить SM
	_, modeFilePath, sm := setupModeSyncTestEnv(t, mode.ModeRW)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Записываем mode.json с новым режимом (ro)
	if err := modefile.SaveMode(modeFilePath, mode.ModeRO, "other-pod:8010"); err != nil {
		t.Fatalf("Ошибка записи mode.json: %v", err)
	}

	svc := NewModeSyncService(modeFilePath, sm, 10*time.Second, logger)
	svc.SyncOnce()

	// Режим должен обновиться на ro
	if sm.CurrentMode() != mode.ModeRO {
		t.Errorf("Режим не обновлён: хотели %s, получили %s", mode.ModeRO, sm.CurrentMode())
	}
}

func TestModeSyncService_SyncOnce_EditToRW(t *testing.T) {
	// Проверяем переход edit → rw через mode.json
	_, modeFilePath, sm := setupModeSyncTestEnv(t, mode.ModeEdit)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Записываем mode.json с rw
	if err := modefile.SaveMode(modeFilePath, mode.ModeRW, "other-pod:8010"); err != nil {
		t.Fatalf("Ошибка записи mode.json: %v", err)
	}

	svc := NewModeSyncService(modeFilePath, sm, 10*time.Second, logger)
	svc.SyncOnce()

	// edit → rw через ForceMode (минуя валидацию переходов)
	if sm.CurrentMode() != mode.ModeRW {
		t.Errorf("Режим не обновлён: хотели %s, получили %s", mode.ModeRW, sm.CurrentMode())
	}
}

func TestModeSyncService_SyncOnce_InvalidModeFile(t *testing.T) {
	// mode.json содержит невалидные данные — SyncOnce не паникует
	dir, modeFilePath, sm := setupModeSyncTestEnv(t, mode.ModeEdit)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Записываем невалидный mode.json
	if err := os.WriteFile(modefile.ModeFilePath(dir), []byte("not json"), 0o640); err != nil {
		t.Fatalf("Ошибка записи файла: %v", err)
	}

	svc := NewModeSyncService(modeFilePath, sm, 10*time.Second, logger)
	svc.SyncOnce()

	// Режим не изменился
	if sm.CurrentMode() != mode.ModeEdit {
		t.Errorf("Режим изменился при невалидном mode.json: хотели %s, получили %s", mode.ModeEdit, sm.CurrentMode())
	}
}

func TestModeSyncService_StartStop(t *testing.T) {
	// Проверяем жизненный цикл: Start → Stop без паники
	_, modeFilePath, sm := setupModeSyncTestEnv(t, mode.ModeEdit)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	svc := NewModeSyncService(modeFilePath, sm, 50*time.Millisecond, logger)
	ctx := context.Background()

	svc.Start(ctx)

	// Даём сервису время выполнить несколько итераций
	time.Sleep(200 * time.Millisecond)

	svc.Stop()
	// Повторный Stop не должен паниковать
	svc.Stop()
}

func TestModeSyncService_DetectsChange_Background(t *testing.T) {
	// Проверяем что фоновый процесс обнаруживает изменение mode.json
	_, modeFilePath, sm := setupModeSyncTestEnv(t, mode.ModeEdit)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	svc := NewModeSyncService(modeFilePath, sm, 50*time.Millisecond, logger)
	ctx := context.Background()

	svc.Start(ctx)
	defer svc.Stop()

	// Записываем mode.json с новым режимом после запуска
	time.Sleep(30 * time.Millisecond)
	if err := modefile.SaveMode(modeFilePath, mode.ModeRW, "other-pod:8010"); err != nil {
		t.Fatalf("Ошибка записи mode.json: %v", err)
	}

	// Ждём пока сервис обнаружит изменение (2+ интервала)
	time.Sleep(200 * time.Millisecond)

	if sm.CurrentMode() != mode.ModeRW {
		t.Errorf("Фоновый процесс не обнаружил изменение: хотели %s, получили %s", mode.ModeRW, sm.CurrentMode())
	}
}
