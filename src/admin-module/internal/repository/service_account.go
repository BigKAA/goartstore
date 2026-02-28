package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/bigkaa/goartstore/admin-module/internal/domain/model"
)

// ServiceAccountRepository — интерфейс CRUD для таблицы service_accounts.
type ServiceAccountRepository interface {
	// Create создаёт новый SA.
	Create(ctx context.Context, sa *model.ServiceAccount) error
	// GetByID возвращает SA по UUID.
	GetByID(ctx context.Context, id string) (*model.ServiceAccount, error)
	// GetByClientID возвращает SA по client_id (sa_<name>_<random>).
	GetByClientID(ctx context.Context, clientID string) (*model.ServiceAccount, error)
	// List возвращает список SA с фильтрацией по статусу.
	List(ctx context.Context, status *string, limit, offset int) ([]*model.ServiceAccount, error)
	// Update обновляет SA.
	Update(ctx context.Context, sa *model.ServiceAccount) error
	// Delete удаляет SA из БД.
	Delete(ctx context.Context, id string) error
	// Count возвращает количество SA.
	Count(ctx context.Context, status *string) (int, error)
}

// serviceAccountRepo — реализация ServiceAccountRepository.
type serviceAccountRepo struct {
	db DBTX
}

// NewServiceAccountRepository создаёт репозиторий Service Accounts.
func NewServiceAccountRepository(db DBTX) ServiceAccountRepository {
	return &serviceAccountRepo{db: db}
}

// scanServiceAccount сканирует строку результата в модель ServiceAccount.
func scanServiceAccount(row pgx.Row) (*model.ServiceAccount, error) {
	sa := &model.ServiceAccount{}
	err := row.Scan(
		&sa.ID, &sa.KeycloakClientID, &sa.ClientID, &sa.Name, &sa.Description,
		&sa.Scopes, &sa.Status, &sa.Source, &sa.LastSyncedAt,
		&sa.CreatedAt, &sa.UpdatedAt,
	)
	return sa, err
}

const saColumns = `id, keycloak_client_id, client_id, name, description,
	scopes, status, source, last_synced_at, created_at, updated_at`

func (r *serviceAccountRepo) Create(ctx context.Context, sa *model.ServiceAccount) error {
	query := `
		INSERT INTO service_accounts (id, keycloak_client_id, client_id, name, description,
			scopes, status, source, last_synced_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING created_at, updated_at`

	err := r.db.QueryRow(ctx, query,
		sa.ID, sa.KeycloakClientID, sa.ClientID, sa.Name, sa.Description,
		sa.Scopes, sa.Status, sa.Source, sa.LastSyncedAt,
	).Scan(&sa.CreatedAt, &sa.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: сервисный аккаунт с таким именем уже существует", ErrConflict)
		}
		return fmt.Errorf("ошибка создания SA: %w", err)
	}
	return nil
}

func (r *serviceAccountRepo) GetByID(ctx context.Context, id string) (*model.ServiceAccount, error) {
	query := fmt.Sprintf(`SELECT %s FROM service_accounts WHERE id = $1`, saColumns)
	sa, err := scanServiceAccount(r.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("ошибка получения SA: %w", err)
	}
	return sa, nil
}

func (r *serviceAccountRepo) GetByClientID(ctx context.Context, clientID string) (*model.ServiceAccount, error) {
	query := fmt.Sprintf(`SELECT %s FROM service_accounts WHERE client_id = $1`, saColumns)
	sa, err := scanServiceAccount(r.db.QueryRow(ctx, query, clientID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("ошибка получения SA по client_id: %w", err)
	}
	return sa, nil
}

func (r *serviceAccountRepo) List(ctx context.Context, status *string, limit, offset int) ([]*model.ServiceAccount, error) {
	var conditions []string
	var args []any
	argNum := 1

	if status != nil {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argNum))
		args = append(args, *status)
		argNum++
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM service_accounts
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, saColumns, where, argNum, argNum+1)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения списка SA: %w", err)
	}
	defer rows.Close()

	var result []*model.ServiceAccount
	for rows.Next() {
		sa := &model.ServiceAccount{}
		if err := rows.Scan(
			&sa.ID, &sa.KeycloakClientID, &sa.ClientID, &sa.Name, &sa.Description,
			&sa.Scopes, &sa.Status, &sa.Source, &sa.LastSyncedAt,
			&sa.CreatedAt, &sa.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("ошибка сканирования SA: %w", err)
		}
		result = append(result, sa)
	}
	return result, rows.Err()
}

func (r *serviceAccountRepo) Update(ctx context.Context, sa *model.ServiceAccount) error {
	query := `
		UPDATE service_accounts
		SET keycloak_client_id = $2, name = $3, description = $4,
			scopes = $5, status = $6, source = $7, last_synced_at = $8
		WHERE id = $1
		RETURNING updated_at`

	err := r.db.QueryRow(ctx, query,
		sa.ID, sa.KeycloakClientID, sa.Name, sa.Description,
		sa.Scopes, sa.Status, sa.Source, sa.LastSyncedAt,
	).Scan(&sa.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: client_id уже существует", ErrConflict)
		}
		return fmt.Errorf("ошибка обновления SA: %w", err)
	}
	return nil
}

func (r *serviceAccountRepo) Delete(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM service_accounts WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("ошибка удаления SA: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *serviceAccountRepo) Count(ctx context.Context, status *string) (int, error) {
	var args []any
	query := "SELECT COUNT(*) FROM service_accounts"

	if status != nil {
		query += " WHERE status = $1"
		args = append(args, *status)
	}

	var count int
	err := r.db.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("ошибка подсчёта SA: %w", err)
	}
	return count, nil
}
