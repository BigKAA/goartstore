// mode_file.go — чтение/запись mode.json на общей файловой системе (NFS).
//
// В replicated mode leader записывает mode.json при каждой смене режима.
// Follower читает mode.json при обновлении индекса для синхронизации режима.
//
// Формат файла:
//
//	{"mode": "rw", "updated_at": "2026-01-01T00:00:00Z", "updated_by": "se-0:8010"}
package replica

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bigkaa/goartstore/storage-element/internal/domain/mode"
)

const (
	// ModeFileName — имя файла режима на общей FS.
	ModeFileName = "mode.json"
)

// ModeFileData — структура данных mode.json.
type ModeFileData struct {
	// Mode — текущий режим работы SE.
	Mode string `json:"mode"`
	// UpdatedAt — время последнего обновления.
	UpdatedAt time.Time `json:"updated_at"`
	// UpdatedBy — идентификатор экземпляра, обновившего режим.
	UpdatedBy string `json:"updated_by"`
}

// ModeFilePath возвращает полный путь к mode.json в указанной директории.
func ModeFilePath(dataDir string) string {
	return filepath.Join(dataDir, ModeFileName)
}

// SaveMode записывает текущий режим в mode.json (атомарно: temp → rename).
//
// Параметры:
//   - path: полный путь к mode.json
//   - m: текущий режим работы
//   - updatedBy: идентификатор экземпляра (hostname:port)
func SaveMode(path string, m mode.StorageMode, updatedBy string) error {
	data := ModeFileData{
		Mode:      string(m),
		UpdatedAt: time.Now().UTC(),
		UpdatedBy: updatedBy,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("ошибка сериализации mode.json: %w", err)
	}

	// Атомарная запись: temp файл → fsync → rename
	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("ошибка создания temp mode.json: %w", err)
	}

	if _, err := f.Write(jsonData); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("ошибка записи temp mode.json: %w", err)
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("ошибка fsync temp mode.json: %w", err)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("ошибка закрытия temp mode.json: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("ошибка rename mode.json: %w", err)
	}

	return nil
}

// LoadMode читает режим из mode.json.
// Возвращает ошибку, если файл не существует или содержит невалидные данные.
func LoadMode(path string) (mode.StorageMode, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения mode.json: %w", err)
	}

	var modeData ModeFileData
	if err = json.Unmarshal(data, &modeData); err != nil {
		return "", fmt.Errorf("ошибка десериализации mode.json: %w", err)
	}

	m, err := mode.ParseMode(modeData.Mode)
	if err != nil {
		return "", fmt.Errorf("невалидный режим в mode.json: %w", err)
	}

	return m, nil
}
