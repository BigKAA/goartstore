package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/arturkryukov/artsore/admin-module/internal/domain/model"
)

// StorageElementRepository — интерфейс CRUD для таблицы storage_elements.
type StorageElementRepository interface {
	// Create создаёт новый SE в реестре.
	Create(ctx context.Context, se *model.StorageElement) error
	// GetByID возвращает SE по UUID.
	GetByID(ctx context.Context, id string) (*model.StorageElement, error)
	// List возвращает список SE с фильтрацией по mode и status.
	List(ctx context.Context, mode, status *string, limit, offset int) ([]*model.StorageElement, error)
	// Update обновляет SE.
	Update(ctx context.Context, se *model.StorageElement) error
	// Delete удаляет SE из реестра.
	Delete(ctx context.Context, id string) error
	// Count возвращает количество SE с фильтрацией.
	Count(ctx context.Context, mode, status *string) (int, error)
}

// storageElementRepo — реализация StorageElementRepository.
type storageElementRepo struct {
	db DBTX
}

// NewStorageElementRepository создаёт репозиторий Storage Elements.
func NewStorageElementRepository(db DBTX) StorageElementRepository {
	return &storageElementRepo{db: db}
}

func (r *storageElementRepo) Create(ctx context.Context, se *model.StorageElement) error {
	query := `
		INSERT INTO storage_elements (id, name, url, storage_id, mode, status,
			capacity_bytes, used_bytes, available_bytes, last_sync_at, last_file_sync_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING created_at, updated_at`

	err := r.db.QueryRow(ctx, query,
		se.ID, se.Name, se.URL, se.StorageID, se.Mode, se.Status,
		se.CapacityBytes, se.UsedBytes, se.AvailableBytes,
		se.LastSyncAt, se.LastFileSyncAt,
	).Scan(&se.CreatedAt, &se.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: url или storage_id уже зарегистрирован", ErrConflict)
		}
		return fmt.Errorf("ошибка создания SE: %w", err)
	}
	return nil
}

func (r *storageElementRepo) GetByID(ctx context.Context, id string) (*model.StorageElement, error) {
	query := `
		SELECT id, name, url, storage_id, mode, status,
			capacity_bytes, used_bytes, available_bytes,
			last_sync_at, last_file_sync_at, created_at, updated_at
		FROM storage_elements
		WHERE id = $1`

	se := &model.StorageElement{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&se.ID, &se.Name, &se.URL, &se.StorageID, &se.Mode, &se.Status,
		&se.CapacityBytes, &se.UsedBytes, &se.AvailableBytes,
		&se.LastSyncAt, &se.LastFileSyncAt, &se.CreatedAt, &se.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("ошибка получения SE: %w", err)
	}
	return se, nil
}

func (r *storageElementRepo) List(ctx context.Context, mode, status *string, limit, offset int) ([]*model.StorageElement, error) {
	// Динамическое построение WHERE
	var conditions []string
	var args []any
	argNum := 1

	if mode != nil {
		conditions = append(conditions, fmt.Sprintf("mode = $%d", argNum))
		args = append(args, *mode)
		argNum++
	}
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
		SELECT id, name, url, storage_id, mode, status,
			capacity_bytes, used_bytes, available_bytes,
			last_sync_at, last_file_sync_at, created_at, updated_at
		FROM storage_elements
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, where, argNum, argNum+1)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения списка SE: %w", err)
	}
	defer rows.Close()

	var result []*model.StorageElement
	for rows.Next() {
		se := &model.StorageElement{}
		if err := rows.Scan(
			&se.ID, &se.Name, &se.URL, &se.StorageID, &se.Mode, &se.Status,
			&se.CapacityBytes, &se.UsedBytes, &se.AvailableBytes,
			&se.LastSyncAt, &se.LastFileSyncAt, &se.CreatedAt, &se.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("ошибка сканирования SE: %w", err)
		}
		result = append(result, se)
	}
	return result, rows.Err()
}

func (r *storageElementRepo) Update(ctx context.Context, se *model.StorageElement) error {
	query := `
		UPDATE storage_elements
		SET name = $2, url = $3, storage_id = $4, mode = $5, status = $6,
			capacity_bytes = $7, used_bytes = $8, available_bytes = $9,
			last_sync_at = $10, last_file_sync_at = $11
		WHERE id = $1
		RETURNING updated_at`

	err := r.db.QueryRow(ctx, query,
		se.ID, se.Name, se.URL, se.StorageID, se.Mode, se.Status,
		se.CapacityBytes, se.UsedBytes, se.AvailableBytes,
		se.LastSyncAt, se.LastFileSyncAt,
	).Scan(&se.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrNotFound
		}
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: url или storage_id уже зарегистрирован", ErrConflict)
		}
		return fmt.Errorf("ошибка обновления SE: %w", err)
	}
	return nil
}

func (r *storageElementRepo) Delete(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM storage_elements WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("ошибка удаления SE: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *storageElementRepo) Count(ctx context.Context, mode, status *string) (int, error) {
	var conditions []string
	var args []any
	argNum := 1

	if mode != nil {
		conditions = append(conditions, fmt.Sprintf("mode = $%d", argNum))
		args = append(args, *mode)
		argNum++
	}
	if status != nil {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argNum))
		args = append(args, *status)
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := fmt.Sprintf(`SELECT COUNT(*) FROM storage_elements %s`, where)

	var count int
	err := r.db.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("ошибка подсчёта SE: %w", err)
	}
	return count, nil
}
