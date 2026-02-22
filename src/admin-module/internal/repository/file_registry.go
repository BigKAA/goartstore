package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/arturkryukov/artsore/admin-module/internal/domain/model"
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
	// Delete выполняет soft delete (status → deleted).
	Delete(ctx context.Context, fileID string) error
	// BatchUpsert вставляет или обновляет массив файлов (для sync).
	BatchUpsert(ctx context.Context, files []*model.FileRecord) (added, updated int, err error)
	// MarkDeletedExcept помечает файлы SE как deleted, кроме указанных.
	MarkDeletedExcept(ctx context.Context, seID string, existingIDs []string) (int, error)
	// Count возвращает количество файлов с фильтрацией.
	Count(ctx context.Context, filters FileListFilters) (int, error)
}

// FileListFilters — фильтры для списка файлов.
type FileListFilters struct {
	Status           *string
	RetentionPolicy  *string
	StorageElementID *string
	UploadedBy       *string
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
			storage_element_id, uploaded_by, uploaded_at, description, tags, status,
			retention_policy, ttl_days, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING created_at, updated_at`

	err := r.db.QueryRow(ctx, query,
		f.FileID, f.OriginalFilename, f.ContentType, f.Size, f.Checksum,
		f.StorageElementID, f.UploadedBy, f.UploadedAt, f.Description, f.Tags,
		f.Status, f.RetentionPolicy, f.TTLDays, f.ExpiresAt,
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
			status, retention_policy, ttl_days, expires_at, created_at, updated_at
		FROM file_registry
		WHERE file_id = $1`

	f := &model.FileRecord{}
	err := r.db.QueryRow(ctx, query, fileID).Scan(
		&f.FileID, &f.OriginalFilename, &f.ContentType, &f.Size, &f.Checksum,
		&f.StorageElementID, &f.UploadedBy, &f.UploadedAt, &f.Description, &f.Tags,
		&f.Status, &f.RetentionPolicy, &f.TTLDays, &f.ExpiresAt, &f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("ошибка получения файла: %w", err)
	}
	return f, nil
}

// buildFileWhere строит WHERE-условие и аргументы для фильтрации файлов.
func buildFileWhere(filters FileListFilters, startArg int) (string, []any) {
	var conditions []string
	var args []any
	argNum := startArg

	if filters.Status != nil {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argNum))
		args = append(args, *filters.Status)
		argNum++
	}
	if filters.RetentionPolicy != nil {
		conditions = append(conditions, fmt.Sprintf("retention_policy = $%d", argNum))
		args = append(args, *filters.RetentionPolicy)
		argNum++
	}
	if filters.StorageElementID != nil {
		conditions = append(conditions, fmt.Sprintf("storage_element_id = $%d", argNum))
		args = append(args, *filters.StorageElementID)
		argNum++
	}
	if filters.UploadedBy != nil {
		conditions = append(conditions, fmt.Sprintf("uploaded_by = $%d", argNum))
		args = append(args, *filters.UploadedBy)
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}
	return where, args
}

func (r *fileRegistryRepo) List(ctx context.Context, filters FileListFilters, limit, offset int) ([]*model.FileRecord, error) {
	where, args := buildFileWhere(filters, 1)
	argNum := len(args) + 1

	query := fmt.Sprintf(`
		SELECT file_id, original_filename, content_type, size, checksum,
			storage_element_id, uploaded_by, uploaded_at, description, tags,
			status, retention_policy, ttl_days, expires_at, created_at, updated_at
		FROM file_registry
		%s
		ORDER BY uploaded_at DESC
		LIMIT $%d OFFSET $%d`, where, argNum, argNum+1)

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
			&f.Status, &f.RetentionPolicy, &f.TTLDays, &f.ExpiresAt, &f.CreatedAt, &f.UpdatedAt,
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
		SET description = $2, tags = $3, status = $4,
			retention_policy = $5, ttl_days = $6, expires_at = $7
		WHERE file_id = $1
		RETURNING updated_at`

	err := r.db.QueryRow(ctx, query,
		f.FileID, f.Description, f.Tags, f.Status,
		f.RetentionPolicy, f.TTLDays, f.ExpiresAt,
	).Scan(&f.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrNotFound
		}
		return fmt.Errorf("ошибка обновления файла: %w", err)
	}
	return nil
}

func (r *fileRegistryRepo) Delete(ctx context.Context, fileID string) error {
	query := `
		UPDATE file_registry
		SET status = 'deleted'
		WHERE file_id = $1 AND status != 'deleted'`

	tag, err := r.db.Exec(ctx, query, fileID)
	if err != nil {
		return fmt.Errorf("ошибка удаления файла: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// BatchUpsert вставляет или обновляет файлы (INSERT ON CONFLICT UPDATE).
// Используется при синхронизации файлового реестра с SE.
// Возвращает количество добавленных и обновлённых записей.
func (r *fileRegistryRepo) BatchUpsert(ctx context.Context, files []*model.FileRecord) (added, updated int, err error) {
	if len(files) == 0 {
		return 0, 0, nil
	}

	for _, f := range files {
		query := `
			INSERT INTO file_registry (file_id, original_filename, content_type, size, checksum,
				storage_element_id, uploaded_by, uploaded_at, description, tags, status,
				retention_policy, ttl_days, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			ON CONFLICT (file_id) DO UPDATE SET
				original_filename = EXCLUDED.original_filename,
				content_type = EXCLUDED.content_type,
				size = EXCLUDED.size,
				checksum = EXCLUDED.checksum,
				description = EXCLUDED.description,
				tags = EXCLUDED.tags,
				status = EXCLUDED.status
			RETURNING (xmax = 0) AS is_insert`

		var isInsert bool
		err := r.db.QueryRow(ctx, query,
			f.FileID, f.OriginalFilename, f.ContentType, f.Size, f.Checksum,
			f.StorageElementID, f.UploadedBy, f.UploadedAt, f.Description, f.Tags,
			f.Status, f.RetentionPolicy, f.TTLDays, f.ExpiresAt,
		).Scan(&isInsert)
		if err != nil {
			return added, updated, fmt.Errorf("ошибка upsert файла %s: %w", f.FileID, err)
		}
		if isInsert {
			added++
		} else {
			updated++
		}
	}
	return added, updated, nil
}

// MarkDeletedExcept помечает файлы SE как deleted, кроме указанных в existingIDs.
// Возвращает количество помеченных файлов.
func (r *fileRegistryRepo) MarkDeletedExcept(ctx context.Context, seID string, existingIDs []string) (int, error) {
	query := `
		UPDATE file_registry
		SET status = 'deleted'
		WHERE storage_element_id = $1
			AND status != 'deleted'
			AND file_id != ALL($2)`

	tag, err := r.db.Exec(ctx, query, seID, existingIDs)
	if err != nil {
		return 0, fmt.Errorf("ошибка пометки удалённых файлов: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (r *fileRegistryRepo) Count(ctx context.Context, filters FileListFilters) (int, error) {
	where, args := buildFileWhere(filters, 1)
	query := fmt.Sprintf(`SELECT COUNT(*) FROM file_registry %s`, where)

	var count int
	err := r.db.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("ошибка подсчёта файлов: %w", err)
	}
	return count, nil
}
