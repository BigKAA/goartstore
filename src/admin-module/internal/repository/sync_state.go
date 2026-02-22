package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/arturkryukov/artsore/admin-module/internal/domain/model"
)

// SyncStateRepository — интерфейс для таблицы sync_state (одна строка).
type SyncStateRepository interface {
	// Get возвращает текущее состояние синхронизации.
	Get(ctx context.Context) (*model.SyncState, error)
	// UpdateSASyncAt обновляет время последней синхронизации SA.
	UpdateSASyncAt(ctx context.Context, t time.Time) error
	// UpdateFileSyncAt обновляет время последней синхронизации файлов.
	UpdateFileSyncAt(ctx context.Context, t time.Time) error
}

// syncStateRepo — реализация SyncStateRepository.
type syncStateRepo struct {
	db DBTX
}

// NewSyncStateRepository создаёт репозиторий состояния синхронизации.
func NewSyncStateRepository(db DBTX) SyncStateRepository {
	return &syncStateRepo{db: db}
}

func (r *syncStateRepo) Get(ctx context.Context) (*model.SyncState, error) {
	query := `
		SELECT id, last_sa_sync_at, last_file_sync_at, created_at, updated_at
		FROM sync_state
		WHERE id = 1`

	s := &model.SyncState{}
	err := r.db.QueryRow(ctx, query).Scan(
		&s.ID, &s.LastSASyncAt, &s.LastFileSyncAt, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения sync_state: %w", err)
	}
	return s, nil
}

func (r *syncStateRepo) UpdateSASyncAt(ctx context.Context, t time.Time) error {
	query := `UPDATE sync_state SET last_sa_sync_at = $1 WHERE id = 1`
	_, err := r.db.Exec(ctx, query, t)
	if err != nil {
		return fmt.Errorf("ошибка обновления last_sa_sync_at: %w", err)
	}
	return nil
}

func (r *syncStateRepo) UpdateFileSyncAt(ctx context.Context, t time.Time) error {
	query := `UPDATE sync_state SET last_file_sync_at = $1 WHERE id = 1`
	_, err := r.db.Exec(ctx, query, t)
	if err != nil {
		return fmt.Errorf("ошибка обновления last_file_sync_at: %w", err)
	}
	return nil
}
