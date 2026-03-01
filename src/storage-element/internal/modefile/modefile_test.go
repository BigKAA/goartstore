package modefile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bigkaa/goartstore/storage-element/internal/domain/mode"
)

func TestModeFilePath(t *testing.T) {
	path := ModeFilePath("/data/se")
	expected := filepath.Join("/data/se", "mode.json")
	if path != expected {
		t.Errorf("ожидалось %q, получено %q", expected, path)
	}
}

func TestSaveModeAndLoadMode(t *testing.T) {
	tmpDir := t.TempDir()
	path := ModeFilePath(tmpDir)

	// Сохраняем режим
	if err := SaveMode(path, mode.ModeRW, "se-test:8010"); err != nil {
		t.Fatalf("SaveMode: %v", err)
	}

	// Загружаем режим
	loaded, err := LoadMode(path)
	if err != nil {
		t.Fatalf("LoadMode: %v", err)
	}

	if loaded != mode.ModeRW {
		t.Errorf("ожидалось %q, получено %q", mode.ModeRW, loaded)
	}
}

func TestSaveModeOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	path := ModeFilePath(tmpDir)

	// Первое сохранение
	if err := SaveMode(path, mode.ModeEdit, "pod-1"); err != nil {
		t.Fatalf("SaveMode(edit): %v", err)
	}

	// Перезапись
	if err := SaveMode(path, mode.ModeRO, "pod-2"); err != nil {
		t.Fatalf("SaveMode(ro): %v", err)
	}

	loaded, err := LoadMode(path)
	if err != nil {
		t.Fatalf("LoadMode: %v", err)
	}

	if loaded != mode.ModeRO {
		t.Errorf("ожидалось %q, получено %q", mode.ModeRO, loaded)
	}
}

func TestLoadModeAllModes(t *testing.T) {
	modes := []mode.StorageMode{mode.ModeEdit, mode.ModeRW, mode.ModeRO, mode.ModeAR}

	for _, m := range modes {
		t.Run(string(m), func(t *testing.T) {
			tmpDir := t.TempDir()
			path := ModeFilePath(tmpDir)

			if err := SaveMode(path, m, "test-pod"); err != nil {
				t.Fatalf("SaveMode: %v", err)
			}

			loaded, err := LoadMode(path)
			if err != nil {
				t.Fatalf("LoadMode: %v", err)
			}
			if loaded != m {
				t.Errorf("ожидалось %q, получено %q", m, loaded)
			}
		})
	}
}

func TestLoadModeFileNotFound(t *testing.T) {
	_, err := LoadMode("/nonexistent/mode.json")
	if err == nil {
		t.Fatal("ожидалась ошибка при чтении несуществующего файла")
	}
}

func TestLoadModeInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "mode.json")

	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("ошибка записи: %v", err)
	}

	_, err := LoadMode(path)
	if err == nil {
		t.Fatal("ожидалась ошибка для невалидного JSON")
	}
}

func TestLoadModeInvalidMode(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "mode.json")

	data := []byte(`{"mode": "invalid", "updated_at": "2026-01-01T00:00:00Z", "updated_by": "test"}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("ошибка записи: %v", err)
	}

	_, err := LoadMode(path)
	if err == nil {
		t.Fatal("ожидалась ошибка для невалидного режима")
	}
}

func TestSaveModeAtomicTempCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	path := ModeFilePath(tmpDir)

	// После успешного SaveMode — temp файл не должен остаться
	if err := SaveMode(path, mode.ModeRW, "test"); err != nil {
		t.Fatalf("SaveMode: %v", err)
	}

	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatal("temp файл не удалён после SaveMode")
	}
}
