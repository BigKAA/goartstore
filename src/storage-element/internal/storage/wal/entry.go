// Пакет wal — файловый Write-Ahead Log для обеспечения
// атомарности операций с файлами в Storage Element.
// Каждая транзакция — отдельный файл {tx_id}.wal.json в SE_WAL_DIR.
package wal

import (
	"time"
)

// OperationType — тип операции, записываемой в WAL.
type OperationType string

const (
	// OpFileCreate — создание нового файла (upload)
	OpFileCreate OperationType = "file_create"
	// OpFileUpdate — обновление метаданных файла
	OpFileUpdate OperationType = "file_update"
	// OpFileDelete — удаление файла (soft delete)
	OpFileDelete OperationType = "file_delete"
)

// TransactionStatus — статус транзакции WAL.
type TransactionStatus string

const (
	// StatusPending — транзакция начата, операция в процессе
	StatusPending TransactionStatus = "pending"
	// StatusCommitted — транзакция успешно завершена
	StatusCommitted TransactionStatus = "committed"
	// StatusRolledBack — транзакция отменена (ошибка или ручной rollback)
	StatusRolledBack TransactionStatus = "rolled_back"
)

// Entry — запись WAL. Хранится как JSON-файл {tx_id}.wal.json.
type Entry struct {
	// TransactionID — уникальный идентификатор транзакции (UUID v4)
	TransactionID string `json:"transaction_id"`

	// Operation — тип операции
	Operation OperationType `json:"operation"`

	// Status — текущий статус транзакции
	Status TransactionStatus `json:"status"`

	// FileID — идентификатор файла, над которым выполняется операция
	FileID string `json:"file_id"`

	// StartedAt — время начала транзакции (UTC)
	StartedAt time.Time `json:"started_at"`

	// CompletedAt — время завершения транзакции (UTC).
	// nil для pending транзакций.
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// walFileName возвращает имя файла WAL для данной транзакции.
func walFileName(txID string) string {
	return txID + ".wal.json"
}
