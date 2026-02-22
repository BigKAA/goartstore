// Пакет repository — слой доступа к данным PostgreSQL.
// Все запросы — чистый SQL через pgx, без ORM.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Ошибки слоя репозиториев.
var (
	// ErrNotFound — запись не найдена.
	ErrNotFound = errors.New("запись не найдена")
	// ErrConflict — конфликт уникальности (дублирующийся ресурс).
	ErrConflict = errors.New("конфликт — запись уже существует")
)

// DBTX — интерфейс для выполнения SQL-запросов.
// Реализуется как *pgxpool.Pool, так и pgx.Tx, что позволяет
// использовать репозитории как внутри, так и вне транзакций.
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// TxRunner позволяет выполнять операции в транзакции.
type TxRunner struct {
	pool *pgxpool.Pool
}

// NewTxRunner создаёт TxRunner для управления транзакциями.
func NewTxRunner(pool *pgxpool.Pool) *TxRunner {
	return &TxRunner{pool: pool}
}

// RunInTx выполняет fn внутри транзакции.
// При ошибке fn — транзакция откатывается.
// При успехе — коммитится.
func (r *TxRunner) RunInTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ошибка начала транзакции: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // откат после коммита — no-op

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// isUniqueViolation проверяет, является ли ошибка нарушением уникальности PostgreSQL.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" // unique_violation
	}
	return false
}
