package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// UISetting — модель записи из таблицы ui_settings.
type UISetting struct {
	// Ключ настройки (dot-notation, например "prometheus.url")
	Key string
	// Значение настройки (строковое представление)
	Value string
	// Время последнего обновления
	UpdatedAt time.Time
	// Кто обновил настройку (username)
	UpdatedBy string
}

// UISettingsRepository — интерфейс для таблицы ui_settings.
type UISettingsRepository interface {
	// Get возвращает настройку по ключу. Если не найдена — ErrNotFound.
	Get(ctx context.Context, key string) (*UISetting, error)
	// Set создаёт или обновляет настройку (upsert).
	Set(ctx context.Context, key, value, updatedBy string) error
	// List возвращает все настройки.
	List(ctx context.Context) ([]UISetting, error)
	// ListByPrefix возвращает настройки с ключами, начинающимися на prefix.
	ListByPrefix(ctx context.Context, prefix string) ([]UISetting, error)
	// Delete удаляет настройку по ключу.
	Delete(ctx context.Context, key string) error
}

// uiSettingsRepo — реализация UISettingsRepository.
type uiSettingsRepo struct {
	db DBTX
}

// NewUISettingsRepository создаёт репозиторий настроек UI.
func NewUISettingsRepository(db DBTX) UISettingsRepository {
	return &uiSettingsRepo{db: db}
}

// Get возвращает настройку по ключу.
func (r *uiSettingsRepo) Get(ctx context.Context, key string) (*UISetting, error) {
	query := `
		SELECT key, value, updated_at, updated_by
		FROM ui_settings
		WHERE key = $1`

	s := &UISetting{}
	err := r.db.QueryRow(ctx, query, key).Scan(
		&s.Key, &s.Value, &s.UpdatedAt, &s.UpdatedBy,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("ошибка получения ui_settings[%s]: %w", key, err)
	}
	return s, nil
}

// Set создаёт или обновляет настройку (INSERT ... ON CONFLICT DO UPDATE).
func (r *uiSettingsRepo) Set(ctx context.Context, key, value, updatedBy string) error {
	query := `
		INSERT INTO ui_settings (key, value, updated_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value,
			updated_by = EXCLUDED.updated_by,
			updated_at = NOW()`

	_, err := r.db.Exec(ctx, query, key, value, updatedBy)
	if err != nil {
		return fmt.Errorf("ошибка сохранения ui_settings[%s]: %w", key, err)
	}
	return nil
}

// List возвращает все настройки, отсортированные по ключу.
func (r *uiSettingsRepo) List(ctx context.Context) ([]UISetting, error) {
	query := `
		SELECT key, value, updated_at, updated_by
		FROM ui_settings
		ORDER BY key`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения списка ui_settings: %w", err)
	}
	defer rows.Close()

	var settings []UISetting
	for rows.Next() {
		var s UISetting
		if err := rows.Scan(&s.Key, &s.Value, &s.UpdatedAt, &s.UpdatedBy); err != nil {
			return nil, fmt.Errorf("ошибка сканирования ui_settings: %w", err)
		}
		settings = append(settings, s)
	}
	return settings, rows.Err()
}

// ListByPrefix возвращает настройки с ключами, начинающимися на prefix.
// Например, prefix="prometheus." вернёт "prometheus.url", "prometheus.enabled" и т.д.
func (r *uiSettingsRepo) ListByPrefix(ctx context.Context, prefix string) ([]UISetting, error) {
	query := `
		SELECT key, value, updated_at, updated_by
		FROM ui_settings
		WHERE key LIKE $1
		ORDER BY key`

	rows, err := r.db.Query(ctx, query, prefix+"%")
	if err != nil {
		return nil, fmt.Errorf("ошибка получения ui_settings по префиксу %q: %w", prefix, err)
	}
	defer rows.Close()

	var settings []UISetting
	for rows.Next() {
		var s UISetting
		if err := rows.Scan(&s.Key, &s.Value, &s.UpdatedAt, &s.UpdatedBy); err != nil {
			return nil, fmt.Errorf("ошибка сканирования ui_settings: %w", err)
		}
		settings = append(settings, s)
	}
	return settings, rows.Err()
}

// Delete удаляет настройку по ключу.
func (r *uiSettingsRepo) Delete(ctx context.Context, key string) error {
	query := `DELETE FROM ui_settings WHERE key = $1`
	tag, err := r.db.Exec(ctx, query, key)
	if err != nil {
		return fmt.Errorf("ошибка удаления ui_settings[%s]: %w", key, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
