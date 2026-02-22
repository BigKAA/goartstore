package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/arturkryukov/artsore/admin-module/internal/domain/model"
)

// RoleOverrideRepository — интерфейс CRUD для таблицы role_overrides.
type RoleOverrideRepository interface {
	// Upsert создаёт или обновляет локальное дополнение роли.
	Upsert(ctx context.Context, ro *model.RoleOverride) error
	// GetByKeycloakUserID возвращает override по Keycloak user ID.
	GetByKeycloakUserID(ctx context.Context, keycloakUserID string) (*model.RoleOverride, error)
	// Delete удаляет override по Keycloak user ID.
	Delete(ctx context.Context, keycloakUserID string) error
	// List возвращает все overrides (с пагинацией).
	List(ctx context.Context, limit, offset int) ([]*model.RoleOverride, error)
	// Count возвращает количество overrides.
	Count(ctx context.Context) (int, error)
}

// roleOverrideRepo — реализация RoleOverrideRepository.
type roleOverrideRepo struct {
	db DBTX
}

// NewRoleOverrideRepository создаёт репозиторий Role Overrides.
func NewRoleOverrideRepository(db DBTX) RoleOverrideRepository {
	return &roleOverrideRepo{db: db}
}

const roColumns = `id, keycloak_user_id, username, additional_role, created_by, created_at, updated_at`

func (r *roleOverrideRepo) Upsert(ctx context.Context, ro *model.RoleOverride) error {
	query := `
		INSERT INTO role_overrides (keycloak_user_id, username, additional_role, created_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (keycloak_user_id) DO UPDATE SET
			username = EXCLUDED.username,
			additional_role = EXCLUDED.additional_role,
			created_by = EXCLUDED.created_by
		RETURNING id, created_at, updated_at`

	err := r.db.QueryRow(ctx, query,
		ro.KeycloakUserID, ro.Username, ro.AdditionalRole, ro.CreatedBy,
	).Scan(&ro.ID, &ro.CreatedAt, &ro.UpdatedAt)
	if err != nil {
		return fmt.Errorf("ошибка upsert role override: %w", err)
	}
	return nil
}

func (r *roleOverrideRepo) GetByKeycloakUserID(ctx context.Context, keycloakUserID string) (*model.RoleOverride, error) {
	query := fmt.Sprintf(`SELECT %s FROM role_overrides WHERE keycloak_user_id = $1`, roColumns)

	ro := &model.RoleOverride{}
	err := r.db.QueryRow(ctx, query, keycloakUserID).Scan(
		&ro.ID, &ro.KeycloakUserID, &ro.Username, &ro.AdditionalRole,
		&ro.CreatedBy, &ro.CreatedAt, &ro.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("ошибка получения role override: %w", err)
	}
	return ro, nil
}

func (r *roleOverrideRepo) Delete(ctx context.Context, keycloakUserID string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM role_overrides WHERE keycloak_user_id = $1`, keycloakUserID)
	if err != nil {
		return fmt.Errorf("ошибка удаления role override: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *roleOverrideRepo) List(ctx context.Context, limit, offset int) ([]*model.RoleOverride, error) {
	query := fmt.Sprintf(`
		SELECT %s
		FROM role_overrides
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`, roColumns)

	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения списка role overrides: %w", err)
	}
	defer rows.Close()

	var result []*model.RoleOverride
	for rows.Next() {
		ro := &model.RoleOverride{}
		if err := rows.Scan(
			&ro.ID, &ro.KeycloakUserID, &ro.Username, &ro.AdditionalRole,
			&ro.CreatedBy, &ro.CreatedAt, &ro.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("ошибка сканирования role override: %w", err)
		}
		result = append(result, ro)
	}
	return result, rows.Err()
}

func (r *roleOverrideRepo) Count(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM role_overrides`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("ошибка подсчёта role overrides: %w", err)
	}
	return count, nil
}
