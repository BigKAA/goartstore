package wal

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// WAL — файловый Write-Ahead Log.
// Гарантирует атомарность операций: сначала создаётся запись WAL
// со статусом pending, затем выполняется операция, затем WAL
// коммитится или откатывается. При рестарте pending записи
// восстанавливаются для обработки.
type WAL struct {
	// dir — директория хранения WAL-файлов (SE_WAL_DIR)
	dir string
	// mu — мьютекс для потокобезопасности
	mu sync.Mutex
	// logger — логгер
	logger *slog.Logger
}

// New создаёт новый WAL-движок. Проверяет и создаёт директорию
// если она не существует. Возвращает ошибку при проблемах с FS.
func New(dir string, logger *slog.Logger) (*WAL, error) {
	// Создаём директорию если не существует
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("не удалось создать директорию WAL %s: %w", dir, err)
	}

	// Проверяем доступность на запись через temp файл
	testFile := filepath.Join(dir, ".wal_write_test")
	if err := os.WriteFile(testFile, []byte("ok"), 0o640); err != nil {
		return nil, fmt.Errorf("директория WAL %s недоступна для записи: %w", dir, err)
	}
	os.Remove(testFile)

	return &WAL{
		dir:    dir,
		logger: logger.With(slog.String("component", "wal")),
	}, nil
}

// StartTransaction создаёт новую WAL-запись со статусом pending.
// Возвращает Entry с уникальным TransactionID (UUID v4).
// Запись сохраняется атомарно: temp файл → fsync → rename.
func (w *WAL) StartTransaction(op OperationType, fileID string) (*Entry, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	entry := &Entry{
		TransactionID: uuid.New().String(),
		Operation:     op,
		Status:        StatusPending,
		FileID:        fileID,
		StartedAt:     time.Now().UTC(),
	}

	if err := w.writeEntry(entry); err != nil {
		return nil, fmt.Errorf("не удалось создать WAL-запись: %w", err)
	}

	w.logger.Debug("WAL транзакция начата",
		slog.String("tx_id", entry.TransactionID),
		slog.String("operation", string(entry.Operation)),
		slog.String("file_id", entry.FileID),
	)

	return entry, nil
}

// Commit помечает транзакцию как успешно завершённую.
// Устанавливает статус committed и время завершения.
func (w *WAL) Commit(txID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	entry, err := w.readEntry(txID)
	if err != nil {
		return fmt.Errorf("не удалось прочитать WAL-запись %s: %w", txID, err)
	}

	if entry.Status != StatusPending {
		return fmt.Errorf("WAL-запись %s имеет статус %s, ожидается %s", txID, entry.Status, StatusPending)
	}

	now := time.Now().UTC()
	entry.Status = StatusCommitted
	entry.CompletedAt = &now

	if err := w.writeEntry(entry); err != nil {
		return fmt.Errorf("не удалось обновить WAL-запись %s: %w", txID, err)
	}

	w.logger.Debug("WAL транзакция завершена",
		slog.String("tx_id", txID),
		slog.String("file_id", entry.FileID),
		slog.Duration("duration", now.Sub(entry.StartedAt)),
	)

	return nil
}

// Rollback помечает транзакцию как отменённую.
// Устанавливает статус rolled_back и время завершения.
func (w *WAL) Rollback(txID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	entry, err := w.readEntry(txID)
	if err != nil {
		return fmt.Errorf("не удалось прочитать WAL-запись %s: %w", txID, err)
	}

	if entry.Status != StatusPending {
		return fmt.Errorf("WAL-запись %s имеет статус %s, ожидается %s", txID, entry.Status, StatusPending)
	}

	now := time.Now().UTC()
	entry.Status = StatusRolledBack
	entry.CompletedAt = &now

	if err := w.writeEntry(entry); err != nil {
		return fmt.Errorf("не удалось обновить WAL-запись %s: %w", txID, err)
	}

	w.logger.Debug("WAL транзакция отменена",
		slog.String("tx_id", txID),
		slog.String("file_id", entry.FileID),
	)

	return nil
}

// RecoverPending находит и возвращает все WAL-записи со статусом pending.
// Вызывается при старте сервера для обработки незавершённых транзакций.
func (w *WAL) RecoverPending() ([]*Entry, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	entries, err := filepath.Glob(filepath.Join(w.dir, "*.wal.json"))
	if err != nil {
		return nil, fmt.Errorf("не удалось сканировать директорию WAL: %w", err)
	}

	var pending []*Entry
	for _, path := range entries {
		txID := strings.TrimSuffix(filepath.Base(path), ".wal.json")
		entry, err := w.readEntry(txID)
		if err != nil {
			w.logger.Warn("Не удалось прочитать WAL-запись при восстановлении",
				slog.String("path", path),
				slog.String("error", err.Error()),
			)
			continue
		}

		if entry.Status == StatusPending {
			pending = append(pending, entry)
			w.logger.Warn("Обнаружена незавершённая WAL-транзакция",
				slog.String("tx_id", entry.TransactionID),
				slog.String("operation", string(entry.Operation)),
				slog.String("file_id", entry.FileID),
				slog.Time("started_at", entry.StartedAt),
			)
		}
	}

	return pending, nil
}

// GetTransaction читает WAL-запись по идентификатору транзакции.
func (w *WAL) GetTransaction(txID string) (*Entry, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.readEntry(txID)
}

// CleanCommitted удаляет все завершённые (committed/rolled_back) WAL-записи.
// Используется для очистки директории WAL от накопившихся записей.
func (w *WAL) CleanCommitted() (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	entries, err := filepath.Glob(filepath.Join(w.dir, "*.wal.json"))
	if err != nil {
		return 0, fmt.Errorf("не удалось сканировать директорию WAL: %w", err)
	}

	cleaned := 0
	for _, path := range entries {
		txID := strings.TrimSuffix(filepath.Base(path), ".wal.json")
		entry, err := w.readEntry(txID)
		if err != nil {
			continue
		}

		if entry.Status == StatusCommitted || entry.Status == StatusRolledBack {
			if err := os.Remove(path); err != nil {
				w.logger.Warn("Не удалось удалить завершённую WAL-запись",
					slog.String("path", path),
					slog.String("error", err.Error()),
				)
				continue
			}
			cleaned++
		}
	}

	if cleaned > 0 {
		w.logger.Info("Очистка WAL завершена",
			slog.Int("cleaned", cleaned),
		)
	}

	return cleaned, nil
}

// writeEntry атомарно записывает WAL-запись на диск.
// Паттерн: temp файл → fsync → atomic rename.
func (w *WAL) writeEntry(entry *Entry) error {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("ошибка сериализации: %w", err)
	}

	targetPath := filepath.Join(w.dir, walFileName(entry.TransactionID))
	tmpPath := targetPath + ".tmp"

	// Запись во временный файл
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("ошибка создания временного файла: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("ошибка записи: %w", err)
	}

	// fsync для гарантии записи на диск
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("ошибка fsync: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("ошибка закрытия файла: %w", err)
	}

	// Атомарный rename
	if err := os.Rename(tmpPath, targetPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("ошибка атомарного переименования: %w", err)
	}

	return nil
}

// readEntry читает WAL-запись из файла.
func (w *WAL) readEntry(txID string) (*Entry, error) {
	path := filepath.Join(w.dir, walFileName(txID))

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения файла: %w", err)
	}

	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("ошибка десериализации: %w", err)
	}

	return &entry, nil
}

// Dir возвращает путь к директории WAL.
func (w *WAL) Dir() string {
	return w.dir
}
