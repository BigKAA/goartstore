package wal

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// testLogger возвращает логгер для тестов (вывод подавляется).
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError, // подавляем debug/info/warn в тестах
	}))
}

// TestNew_CreatesDirectory проверяет, что New создаёт директорию WAL.
func TestNew_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	walDir := filepath.Join(dir, "wal")

	w, err := New(walDir, testLogger())
	if err != nil {
		t.Fatalf("ожидалось успешное создание WAL, получена ошибка: %v", err)
	}

	if w.Dir() != walDir {
		t.Errorf("ожидался путь %s, получен %s", walDir, w.Dir())
	}

	// Проверяем, что директория создана
	info, err := os.Stat(walDir)
	if err != nil {
		t.Fatalf("директория WAL не создана: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("WAL path не является директорией")
	}
}

// TestNew_ReadOnlyDir проверяет ошибку при недоступной для записи директории.
func TestNew_ReadOnlyDir(t *testing.T) {
	dir := t.TempDir()
	walDir := filepath.Join(dir, "wal")

	// Создаём директорию только для чтения
	if err := os.MkdirAll(walDir, 0o550); err != nil {
		t.Fatalf("не удалось создать директорию: %v", err)
	}

	_, err := New(walDir, testLogger())
	if err == nil {
		t.Fatal("ожидалась ошибка при недоступной для записи директории")
	}
}

// TestStartTransaction проверяет создание новой транзакции.
func TestStartTransaction(t *testing.T) {
	w, err := New(t.TempDir(), testLogger())
	if err != nil {
		t.Fatalf("ошибка создания WAL: %v", err)
	}

	entry, err := w.StartTransaction(OpFileCreate, "file-123")
	if err != nil {
		t.Fatalf("ошибка создания транзакции: %v", err)
	}

	// Проверяем поля
	if entry.TransactionID == "" {
		t.Error("TransactionID не должен быть пустым")
	}
	if entry.Operation != OpFileCreate {
		t.Errorf("ожидалась операция %s, получена %s", OpFileCreate, entry.Operation)
	}
	if entry.Status != StatusPending {
		t.Errorf("ожидался статус %s, получен %s", StatusPending, entry.Status)
	}
	if entry.FileID != "file-123" {
		t.Errorf("ожидался FileID 'file-123', получен %q", entry.FileID)
	}
	if entry.StartedAt.IsZero() {
		t.Error("StartedAt не должен быть нулевым")
	}
	if entry.CompletedAt != nil {
		t.Error("CompletedAt должен быть nil для pending")
	}

	// Проверяем файл на диске
	walFile := filepath.Join(w.Dir(), walFileName(entry.TransactionID))
	if _, err := os.Stat(walFile); os.IsNotExist(err) {
		t.Errorf("WAL-файл не найден: %s", walFile)
	}
}

// TestCommit проверяет успешное завершение транзакции.
func TestCommit(t *testing.T) {
	w, err := New(t.TempDir(), testLogger())
	if err != nil {
		t.Fatalf("ошибка создания WAL: %v", err)
	}

	entry, err := w.StartTransaction(OpFileCreate, "file-123")
	if err != nil {
		t.Fatalf("ошибка создания транзакции: %v", err)
	}

	if err := w.Commit(entry.TransactionID); err != nil {
		t.Fatalf("ошибка коммита: %v", err)
	}

	// Читаем обратно и проверяем
	committed, err := w.GetTransaction(entry.TransactionID)
	if err != nil {
		t.Fatalf("ошибка чтения: %v", err)
	}

	if committed.Status != StatusCommitted {
		t.Errorf("ожидался статус %s, получен %s", StatusCommitted, committed.Status)
	}
	if committed.CompletedAt == nil {
		t.Error("CompletedAt не должен быть nil после коммита")
	}
}

// TestRollback проверяет откат транзакции.
func TestRollback(t *testing.T) {
	w, err := New(t.TempDir(), testLogger())
	if err != nil {
		t.Fatalf("ошибка создания WAL: %v", err)
	}

	entry, err := w.StartTransaction(OpFileDelete, "file-456")
	if err != nil {
		t.Fatalf("ошибка создания транзакции: %v", err)
	}

	if err := w.Rollback(entry.TransactionID); err != nil {
		t.Fatalf("ошибка rollback: %v", err)
	}

	// Читаем обратно и проверяем
	rolledBack, err := w.GetTransaction(entry.TransactionID)
	if err != nil {
		t.Fatalf("ошибка чтения: %v", err)
	}

	if rolledBack.Status != StatusRolledBack {
		t.Errorf("ожидался статус %s, получен %s", StatusRolledBack, rolledBack.Status)
	}
	if rolledBack.CompletedAt == nil {
		t.Error("CompletedAt не должен быть nil после rollback")
	}
}

// TestCommit_NonPending проверяет ошибку коммита не-pending транзакции.
func TestCommit_NonPending(t *testing.T) {
	w, err := New(t.TempDir(), testLogger())
	if err != nil {
		t.Fatalf("ошибка создания WAL: %v", err)
	}

	entry, err := w.StartTransaction(OpFileCreate, "file-123")
	if err != nil {
		t.Fatalf("ошибка создания транзакции: %v", err)
	}

	// Первый коммит — успешно
	if err := w.Commit(entry.TransactionID); err != nil {
		t.Fatalf("ошибка первого коммита: %v", err)
	}

	// Повторный коммит — ошибка
	if err := w.Commit(entry.TransactionID); err == nil {
		t.Error("ожидалась ошибка при повторном коммите")
	}
}

// TestRollback_NonPending проверяет ошибку rollback не-pending транзакции.
func TestRollback_NonPending(t *testing.T) {
	w, err := New(t.TempDir(), testLogger())
	if err != nil {
		t.Fatalf("ошибка создания WAL: %v", err)
	}

	entry, err := w.StartTransaction(OpFileCreate, "file-123")
	if err != nil {
		t.Fatalf("ошибка создания транзакции: %v", err)
	}

	// Коммит
	if err := w.Commit(entry.TransactionID); err != nil {
		t.Fatalf("ошибка коммита: %v", err)
	}

	// Rollback закоммиченной — ошибка
	if err := w.Rollback(entry.TransactionID); err == nil {
		t.Error("ожидалась ошибка при rollback закоммиченной транзакции")
	}
}

// TestGetTransaction_NotFound проверяет ошибку при несуществующей транзакции.
func TestGetTransaction_NotFound(t *testing.T) {
	w, err := New(t.TempDir(), testLogger())
	if err != nil {
		t.Fatalf("ошибка создания WAL: %v", err)
	}

	_, err = w.GetTransaction("nonexistent-tx-id")
	if err == nil {
		t.Error("ожидалась ошибка для несуществующей транзакции")
	}
}

// TestRecoverPending проверяет восстановление pending транзакций.
func TestRecoverPending(t *testing.T) {
	w, err := New(t.TempDir(), testLogger())
	if err != nil {
		t.Fatalf("ошибка создания WAL: %v", err)
	}

	// Создаём 3 транзакции: 1 pending, 1 committed, 1 rolled_back
	pending, err := w.StartTransaction(OpFileCreate, "file-1")
	if err != nil {
		t.Fatalf("ошибка создания транзакции: %v", err)
	}

	committed, err := w.StartTransaction(OpFileUpdate, "file-2")
	if err != nil {
		t.Fatalf("ошибка создания транзакции: %v", err)
	}
	if err := w.Commit(committed.TransactionID); err != nil {
		t.Fatalf("ошибка коммита: %v", err)
	}

	rolledBack, err := w.StartTransaction(OpFileDelete, "file-3")
	if err != nil {
		t.Fatalf("ошибка создания транзакции: %v", err)
	}
	if err := w.Rollback(rolledBack.TransactionID); err != nil {
		t.Fatalf("ошибка rollback: %v", err)
	}

	// Восстановление — должна быть только 1 pending
	recovered, err := w.RecoverPending()
	if err != nil {
		t.Fatalf("ошибка восстановления: %v", err)
	}

	if len(recovered) != 1 {
		t.Fatalf("ожидалась 1 pending транзакция, получено %d", len(recovered))
	}
	if recovered[0].TransactionID != pending.TransactionID {
		t.Errorf("ожидался tx_id %s, получен %s", pending.TransactionID, recovered[0].TransactionID)
	}
}

// TestRecoverPending_Empty проверяет восстановление при пустом WAL.
func TestRecoverPending_Empty(t *testing.T) {
	w, err := New(t.TempDir(), testLogger())
	if err != nil {
		t.Fatalf("ошибка создания WAL: %v", err)
	}

	recovered, err := w.RecoverPending()
	if err != nil {
		t.Fatalf("ошибка восстановления: %v", err)
	}

	if len(recovered) != 0 {
		t.Errorf("ожидалось 0 pending транзакций, получено %d", len(recovered))
	}
}

// TestCleanCommitted проверяет очистку завершённых WAL-записей.
func TestCleanCommitted(t *testing.T) {
	w, err := New(t.TempDir(), testLogger())
	if err != nil {
		t.Fatalf("ошибка создания WAL: %v", err)
	}

	// Создаём транзакции: 1 pending, 2 committed
	_, err = w.StartTransaction(OpFileCreate, "file-1")
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}

	tx2, err := w.StartTransaction(OpFileUpdate, "file-2")
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	w.Commit(tx2.TransactionID)

	tx3, err := w.StartTransaction(OpFileDelete, "file-3")
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	w.Rollback(tx3.TransactionID)

	// Очистка — должны удалиться 2 (committed + rolled_back)
	cleaned, err := w.CleanCommitted()
	if err != nil {
		t.Fatalf("ошибка очистки: %v", err)
	}

	if cleaned != 2 {
		t.Errorf("ожидалось 2 очищенных записи, получено %d", cleaned)
	}

	// Pending должна остаться
	recovered, _ := w.RecoverPending()
	if len(recovered) != 1 {
		t.Errorf("ожидалась 1 pending запись, получено %d", len(recovered))
	}
}

// TestAtomicWrite проверяет, что WAL-файлы записываются атомарно.
func TestAtomicWrite(t *testing.T) {
	w, err := New(t.TempDir(), testLogger())
	if err != nil {
		t.Fatalf("ошибка создания WAL: %v", err)
	}

	entry, err := w.StartTransaction(OpFileCreate, "file-atomic")
	if err != nil {
		t.Fatalf("ошибка создания транзакции: %v", err)
	}

	// Проверяем, что temp файл не остался
	tmpPath := filepath.Join(w.Dir(), walFileName(entry.TransactionID)+".tmp")
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("временный файл не должен существовать после записи: %s", tmpPath)
	}

	// Проверяем, что JSON валиден
	data, err := os.ReadFile(filepath.Join(w.Dir(), walFileName(entry.TransactionID)))
	if err != nil {
		t.Fatalf("ошибка чтения: %v", err)
	}

	var readEntry Entry
	if err := json.Unmarshal(data, &readEntry); err != nil {
		t.Fatalf("невалидный JSON: %v", err)
	}
}

// TestConcurrentAccess проверяет потокобезопасность WAL.
func TestConcurrentAccess(t *testing.T) {
	w, err := New(t.TempDir(), testLogger())
	if err != nil {
		t.Fatalf("ошибка создания WAL: %v", err)
	}

	const goroutines = 20
	var wg sync.WaitGroup
	errors := make(chan error, goroutines)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			// Каждая горутина: create → commit
			entry, err := w.StartTransaction(OpFileCreate, "file-concurrent")
			if err != nil {
				errors <- err
				return
			}

			if err := w.Commit(entry.TransactionID); err != nil {
				errors <- err
				return
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("ошибка в горутине: %v", err)
	}
}

// TestMultipleOperationTypes проверяет все типы операций WAL.
func TestMultipleOperationTypes(t *testing.T) {
	w, err := New(t.TempDir(), testLogger())
	if err != nil {
		t.Fatalf("ошибка создания WAL: %v", err)
	}

	ops := []OperationType{OpFileCreate, OpFileUpdate, OpFileDelete}

	for _, op := range ops {
		entry, err := w.StartTransaction(op, "file-ops-test")
		if err != nil {
			t.Fatalf("ошибка создания транзакции (%s): %v", op, err)
		}
		if entry.Operation != op {
			t.Errorf("ожидалась операция %s, получена %s", op, entry.Operation)
		}
		if err := w.Commit(entry.TransactionID); err != nil {
			t.Fatalf("ошибка коммита (%s): %v", op, err)
		}
	}
}
