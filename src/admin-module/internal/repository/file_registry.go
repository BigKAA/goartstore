package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/bigkaa/goartstore/admin-module/internal/domain/model"
)

// FileRegistryRepository — интерфейс CRUD для таблицы file_registry.
type FileRegistryRepository interface {
	// Register создаёт новую запись файла в реестре.
	Register(ctx context.Context, f *model.FileRecord) error
	// GetByID возвращает файл по UUID.
	GetByID(ctx context.Context, fileID string) (*model.FileRecord, error)
	// List возвращает список файлов с фильтрацией.
	List(ctx context.Context, filters FileListFilters, limit, offset int) ([]*model.FileRecord, error)
	// Update обновляет метаданные файла.
	Update(ctx context.Context, f *model.FileRecord) error
	// Delete физически удаляет запись файла из БД.
	Delete(ctx context.Context, fileID string) error
	// BatchUpsert вставляет или обновляет массив файлов (для sync).
	BatchUpsert(ctx context.Context, files []*model.FileRecord) (added, updated int, err error)
	// DeleteExcept удаляет записи файлов SE, кроме указанных в existingIDs.
	DeleteExcept(ctx context.Context, seID string, existingIDs []string) (int, error)
	// Count возвращает количество файлов с фильтрацией.
	Count(ctx context.Context, filters FileListFilters) (int, error)
}

// FileListFilters — фильтры для списка файлов.
type FileListFilters struct {
	RetentionPolicy  *string
	StorageElementID *string
	UploadedBy       *string
	SEMode           *string // Режим SE (для фильтрации файлов по mode SE, например "ar" для архивных)
}

// fileRegistryRepo — реализация FileRegistryRepository.
type fileRegistryRepo struct {
	db DBTX
}

// NewFileRegistryRepository создаёт репозиторий файлового реестра.
func NewFileRegistryRepository(db DBTX) FileRegistryRepository {
	return &fileRegistryRepo{db: db}
}

func (r *fileRegistryRepo) Register(ctx context.Context, f *model.FileRecord) error {
	query := `
		INSERT INTO file_registry (file_id, original_filename, content_type, size, checksum,
			storage_element_id, uploaded_by, uploaded_at, description, tags,
			retention_policy, ttl_days, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING created_at, updated_at`

	err := r.db.QueryRow(ctx, query,
		f.FileID, f.OriginalFilename, f.ContentType, f.Size, f.Checksum,
		f.StorageElementID, f.UploadedBy, f.UploadedAt, f.Description, f.Tags,
		f.RetentionPolicy, f.TTLDays, f.ExpiresAt,
	).Scan(&f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: файл с таким ID уже зарегистрирован", ErrConflict)
		}
		return fmt.Errorf("ошибка регистрации файла: %w", err)
	}
	return nil
}

func (r *fileRegistryRepo) GetByID(ctx context.Context, fileID string) (*model.FileRecord, error) {
	query := `
		SELECT file_id, original_filename, content_type, size, checksum,
			storage_element_id, uploaded_by, uploaded_at, description, tags,
			retention_policy, ttl_days, expires_at, created_at, updated_at
		FROM file_registry
		WHERE file_id = $1`

	f := &model.FileRecord{}
	err := r.db.QueryRow(ctx, query, fileID).Scan(
		&f.FileID, &f.OriginalFilename, &f.ContentType, &f.Size, &f.Checksum,
		&f.StorageElementID, &f.UploadedBy, &f.UploadedAt, &f.Description, &f.Tags,
		&f.RetentionPolicy, &f.TTLDays, &f.ExpiresAt, &f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("ошибка получения файла: %w", err)
	}
	return f, nil
}

// buildFileWhere строит WHERE-условие, JOIN и аргументы для фильтрации файлов.
// Возвращает joinClause (пустой, если JOIN не нужен), whereClause и args.
func buildFileWhere(filters FileListFilters, startArg int) (joinClause, whereClause string, args []any) {
	var conditions []string
	argNum := startArg

	if filters.RetentionPolicy != nil {
		conditions = append(conditions, fmt.Sprintf("fr.retention_policy = $%d", argNum))
		args = append(args, *filters.RetentionPolicy)
		argNum++
	}
	if filters.StorageElementID != nil {
		conditions = append(conditions, fmt.Sprintf("fr.storage_element_id = $%d", argNum))
		args = append(args, *filters.StorageElementID)
		argNum++
	}
	if filters.UploadedBy != nil {
		conditions = append(conditions, fmt.Sprintf("fr.uploaded_by = $%d", argNum))
		args = append(args, *filters.UploadedBy)
		argNum++
	}

	// Фильтр по режиму SE — требует JOIN с storage_elements
	join := ""
	if filters.SEMode != nil {
		join = "JOIN storage_elements se ON se.id = fr.storage_element_id"
		conditions = append(conditions, fmt.Sprintf("se.mode = $%d", argNum))
		args = append(args, *filters.SEMode)
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}
	return join, where, args
}

func (r *fileRegistryRepo) List(ctx context.Context, filters FileListFilters, limit, offset int) ([]*model.FileRecord, error) {
	join, where, args := buildFileWhere(filters, 1)
	argNum := len(args) + 1

	query := fmt.Sprintf(`
		SELECT fr.file_id, fr.original_filename, fr.content_type, fr.size, fr.checksum,
			fr.storage_element_id, fr.uploaded_by, fr.uploaded_at, fr.description, fr.tags,
			fr.retention_policy, fr.ttl_days, fr.expires_at, fr.created_at, fr.updated_at
		FROM file_registry fr
		%s
		%s
		ORDER BY fr.uploaded_at DESC
		LIMIT $%d OFFSET $%d`, join, where, argNum, argNum+1)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения списка файлов: %w", err)
	}
	defer rows.Close()

	var result []*model.FileRecord
	for rows.Next() {
		f := &model.FileRecord{}
		if err := rows.Scan(
			&f.FileID, &f.OriginalFilename, &f.ContentType, &f.Size, &f.Checksum,
			&f.StorageElementID, &f.UploadedBy, &f.UploadedAt, &f.Description, &f.Tags,
			&f.RetentionPolicy, &f.TTLDays, &f.ExpiresAt, &f.CreatedAt, &f.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("ошибка сканирования файла: %w", err)
		}
		result = append(result, f)
	}
	return result, rows.Err()
}

func (r *fileRegistryRepo) Update(ctx context.Context, f *model.FileRecord) error {
	query := `
		UPDATE file_registry
		SET description = $2, tags = $3,
			retention_policy = $4, ttl_days = $5, expires_at = $6
		WHERE file_id = $1
		RETURNING updated_at`

	err := r.db.QueryRow(ctx, query,
		f.FileID, f.Description, f.Tags,
		f.RetentionPolicy, f.TTLDays, f.ExpiresAt,
	).Scan(&f.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("ошибка обновления файла: %w", err)
	}
	return nil
}

// Delete физически удаляет запись файла из БД (hard delete).
func (r *fileRegistryRepo) Delete(ctx context.Context, fileID string) error {
	query := `DELETE FROM file_registry WHERE file_id = $1`

	tag, err := r.db.Exec(ctx, query, fileID)
	if err != nil {
		return fmt.Errorf("ошибка удаления файла: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// BatchUpsert вставляет или обновляет файлы (INSERT ON CONFLICT UPDATE) через pgx.Batch.
// Используется при синхронизации файлового реестра с SE.
// Все запросы отправляются одним round-trip к БД, что значительно быстрее
// поштучного выполнения (O(1) round-trip вместо O(N)).
// Возвращает количество добавленных и обновлённых записей.
func (r *fileRegistryRepo) BatchUpsert(ctx context.Context, files []*model.FileRecord) (added, updated int, err error) {
	if len(files) == 0 {
		return 0, 0, nil
	}

	query := `
		INSERT INTO file_registry (file_id, original_filename, content_type, size, checksum,
			storage_element_id, uploaded_by, uploaded_at, description, tags,
			retention_policy, ttl_days, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (file_id) DO UPDATE SET
			original_filename = EXCLUDED.original_filename,
			content_type = EXCLUDED.content_type,
			size = EXCLUDED.size,
			checksum = EXCLUDED.checksum,
			description = EXCLUDED.description,
			tags = EXCLUDED.tags
		RETURNING (xmax = 0) AS is_insert`

	batch := &pgx.Batch{}
	for _, f := range files {
		batch.Queue(query,
			f.FileID, f.OriginalFilename, f.ContentType, f.Size, f.Checksum,
			f.StorageElementID, f.UploadedBy, f.UploadedAt, f.Description, f.Tags,
			f.RetentionPolicy, f.TTLDays, f.ExpiresAt,
		)
	}

	br := r.db.SendBatch(ctx, batch)
	defer func() {
		if closeErr := br.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("ошибка закрытия batch: %w", closeErr)
		}
	}()

	for i, f := range files {
		var isInsert bool
		if err := br.QueryRow().Scan(&isInsert); err != nil {
			return added, updated, fmt.Errorf("ошибка upsert файла %s (индекс %d): %w", f.FileID, i, err)
		}
		if isInsert {
			added++
		} else {
			updated++
		}
	}
	return added, updated, nil
}

// DeleteExcept удаляет записи файлов SE, кроме указанных в existingIDs.
// Возвращает количество удалённых записей.
func (r *fileRegistryRepo) DeleteExcept(ctx context.Context, seID string, existingIDs []string) (int, error) {
	query := `
		DELETE FROM file_registry
		WHERE storage_element_id = $1
			AND file_id != ALL($2)`

	tag, err := r.db.Exec(ctx, query, seID, existingIDs)
	if err != nil {
		return 0, fmt.Errorf("ошибка удаления устаревших файлов: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (r *fileRegistryRepo) Count(ctx context.Context, filters FileListFilters) (int, error) {
	join, where, args := buildFileWhere(filters, 1)
	query := fmt.Sprintf(`SELECT COUNT(*) FROM file_registry fr %s %s`, join, where)

	var count int
	err := r.db.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("ошибка подсчёта файлов: %w", err)
	}
	return count, nil
}
