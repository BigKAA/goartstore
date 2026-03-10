// Пакет repository — слой доступа к данным PostgreSQL.
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
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// isUniqueViolation проверяет, является ли ошибка нарушением уникальности PostgreSQL.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" // unique_violation
	}
	return false
}
