package replica

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arturkryukov/artstore/storage-element/internal/domain/mode"
)

// newTestLogger создаёт логгер для тестов.
func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// TestElection_SingleInstance — один экземпляр захватывает lock и становится leader.
func TestElection_SingleInstance(t *testing.T) {
	tmpDir := t.TempDir()
	logger := newTestLogger()

	leaderCalled := false
	election := NewElection(tmpDir, 8010, 0, func() {
		leaderCalled = true
	}, func() {}, logger)

	if err := election.Start(); err != nil {
		t.Fatalf("Ошибка Start: %v", err)
	}
	defer election.Stop()

	if !leaderCalled {
		t.Error("onBecomeLeader не был вызван")
	}

	if election.CurrentRole() != RoleLeader {
		t.Errorf("Ожидалась роль leader, получена %s", election.CurrentRole())
	}

	if !election.IsLeader() {
		t.Error("IsLeader() должен вернуть true")
	}

	if election.LeaderAddr() == "" {
		t.Error("LeaderAddr() не должен быть пустым")
	}
}

// TestElection_TwoInstances — первый становится leader, второй follower.
func TestElection_TwoInstances(t *testing.T) {
	tmpDir := t.TempDir()
	logger := newTestLogger()

	// Первый экземпляр
	leader := NewElection(tmpDir, 8010, 0, func() {}, func() {}, logger)
	if err := leader.Start(); err != nil {
		t.Fatalf("Ошибка Start leader: %v", err)
	}
	defer leader.Stop()

	if leader.CurrentRole() != RoleLeader {
		t.Fatalf("Первый экземпляр должен быть leader, получена роль %s", leader.CurrentRole())
	}

	// Второй экземпляр
	followerCalled := false
	follower := NewElection(tmpDir, 8011, 0, func() {}, func() {
		followerCalled = true
	}, logger)
	if err := follower.Start(); err != nil {
		t.Fatalf("Ошибка Start follower: %v", err)
	}
	defer follower.Stop()

	if !followerCalled {
		t.Error("onBecomeFollower не был вызван")
	}

	if follower.CurrentRole() != RoleFollower {
		t.Errorf("Второй экземпляр должен быть follower, получена роль %s", follower.CurrentRole())
	}

	if follower.IsLeader() {
		t.Error("Follower не должен быть leader")
	}
}

// TestElection_LeaderInfoFile — .leader.info содержит корректный адрес.
func TestElection_LeaderInfoFile(t *testing.T) {
	tmpDir := t.TempDir()
	logger := newTestLogger()

	election := NewElection(tmpDir, 8010, 0, func() {}, func() {}, logger)
	if err := election.Start(); err != nil {
		t.Fatalf("Ошибка Start: %v", err)
	}
	defer election.Stop()

	// Проверяем .leader.info
	infoPath := filepath.Join(tmpDir, leaderInfoFile)
	data, err := os.ReadFile(infoPath)
	if err != nil {
		t.Fatalf("Ошибка чтения .leader.info: %v", err)
	}

	addr := strings.TrimSpace(string(data))
	if addr == "" {
		t.Error(".leader.info пуст")
	}

	// Адрес должен содержать порт
	if !strings.HasSuffix(addr, ":8010") {
		t.Errorf("Адрес должен заканчиваться на :8010, получен %q", addr)
	}

	// LeaderAddr() должен совпадать
	if election.LeaderAddr() != addr {
		t.Errorf("LeaderAddr() = %q, .leader.info = %q", election.LeaderAddr(), addr)
	}
}

// TestElection_FollowerBecomesLeader — после освобождения lock follower становится leader.
func TestElection_FollowerBecomesLeader(t *testing.T) {
	tmpDir := t.TempDir()
	logger := newTestLogger()

	// Первый экземпляр — leader
	leader := NewElection(tmpDir, 8010, 0, func() {}, func() {}, logger)
	if err := leader.Start(); err != nil {
		t.Fatalf("Ошибка Start leader: %v", err)
	}

	// Второй экземпляр — follower
	followerBecameLeader := make(chan struct{})
	follower := NewElection(tmpDir, 8011, 0, func() {
		close(followerBecameLeader)
	}, func() {}, logger)
	if err := follower.Start(); err != nil {
		t.Fatalf("Ошибка Start follower: %v", err)
	}
	defer follower.Stop()

	if follower.CurrentRole() != RoleFollower {
		t.Fatalf("Второй экземпляр должен быть follower")
	}

	// Освобождаем lock у первого (имитируем остановку leader)
	leader.Stop()

	// Ждём, что follower станет leader (retry каждые 5 сек)
	select {
	case <-followerBecameLeader:
		// Успех
	case <-time.After(15 * time.Second):
		t.Fatal("Follower не стал leader за 15 секунд")
	}

	if follower.CurrentRole() != RoleLeader {
		t.Errorf("Follower должен стать leader после освобождения lock, роль %s", follower.CurrentRole())
	}
}

// TestModeFile_SaveLoadRoundtrip — SaveMode/LoadMode roundtrip.
func TestModeFile_SaveLoadRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	modePath := filepath.Join(tmpDir, ModeFileName)

	testCases := []struct {
		name      string
		mode      string
		updatedBy string
	}{
		{"edit mode", "edit", "se-0:8010"},
		{"rw mode", "rw", "se-1:8011"},
		{"ro mode", "ro", "se-0:8010"},
		{"ar mode", "ar", "se-0:8010"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Сохраняем
			if err := SaveMode(modePath, modeFromStr(tc.mode), tc.updatedBy); err != nil {
				t.Fatalf("SaveMode ошибка: %v", err)
			}

			// Загружаем
			loaded, err := LoadMode(modePath)
			if err != nil {
				t.Fatalf("LoadMode ошибка: %v", err)
			}

			if string(loaded) != tc.mode {
				t.Errorf("Ожидался режим %q, получен %q", tc.mode, loaded)
			}
		})
	}
}

// TestModeFile_LoadNonExistent — LoadMode на несуществующем файле возвращает ошибку.
func TestModeFile_LoadNonExistent(t *testing.T) {
	_, err := LoadMode("/nonexistent/mode.json")
	if err == nil {
		t.Error("Ожидалась ошибка для несуществующего файла")
	}
}

// TestStandaloneProvider — StandaloneProvider всегда возвращает standalone/leader.
func TestStandaloneProvider(t *testing.T) {
	p := &StandaloneProvider{}

	if p.CurrentRole() != RoleStandalone {
		t.Errorf("Ожидалась роль standalone, получена %s", p.CurrentRole())
	}

	if !p.IsLeader() {
		t.Error("IsLeader() должен вернуть true")
	}

	if p.LeaderAddr() != "" {
		t.Errorf("LeaderAddr() должен быть пустым, получен %q", p.LeaderAddr())
	}
}

// modeFromStr — вспомогательная функция для тестов.
func modeFromStr(s string) mode.StorageMode {
	return mode.StorageMode(s)
}
