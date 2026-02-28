// Пакет repository — слой доступа к данным PostgreSQL для Query Module.
// QM — read-only потребитель таблицы file_registry (owned by Admin Module),
// за исключением обновления статуса при lazy cleanup (MarkDeleted).
// Все запросы — чистый SQL через pgx, без ORM.
package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Ошибки слоя репозиториев.
var (
	// ErrNotFound — запись не найдена.
	ErrNotFound = errors.New("запись не найдена")
)

// DBTX — интерфейс для выполнения SQL-запросов.
// Реализуется как *pgxpool.Pool, так и pgx.Tx, что позволяет
// использовать репозитории как внутри, так и вне транзакций.
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}
