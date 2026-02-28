package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bigkaa/goartstore/query-module/internal/domain/model"
)

// fileColumns — список столбцов таблицы file_registry для SELECT-запросов.
// DRY: одно место для всех SELECT'ов.
const fileColumns = `file_id, original_filename, content_type, size, checksum,
	storage_element_id, uploaded_by, uploaded_at, description, tags,
	status, retention_policy, ttl_days, expires_at, created_at, updated_at`

// SearchParams — параметры поиска файлов.
// Все поля — указатели, nil = фильтр не применяется.
type SearchParams struct {
	// Query — поисковый запрос по имени файла (exact или partial match)
	Query *string
	// Filename — фильтр по имени файла (partial match)
	Filename *string
	// FileExtension — фильтр по расширению файла (без точки)
	FileExtension *string
	// Tags — фильтр по тегам (файл должен содержать все указанные теги)
	Tags *[]string
	// UploadedBy — фильтр по загрузившему (exact match)
	UploadedBy *string
	// RetentionPolicy — фильтр по политике хранения (permanent/temporary)
	RetentionPolicy *string
	// Status — фильтр по статусу (active/deleted/expired)
	Status *string
	// MinSize — минимальный размер файла (байт)
	MinSize *int64
	// MaxSize — максимальный размер файла (байт)
	MaxSize *int64
	// UploadedAfter — файлы, загруженные после указанной даты
	UploadedAfter *time.Time
	// UploadedBefore — файлы, загруженные до указанной даты
	UploadedBefore *time.Time
	// Mode — режим поиска: "exact" или "partial" (по умолчанию)
	Mode string
	// SortBy — поле сортировки: uploaded_at, original_filename, size
	SortBy string
	// SortOrder — направление: asc, desc
	SortOrder string
	// Limit — количество результатов
	Limit int
	// Offset — смещение
	Offset int
}

// FileRepository — интерфейс доступа к файлам в file_registry.
// QM использует read-only операции + MarkDeleted для lazy cleanup.
type FileRepository interface {
	// GetByID возвращает файл по UUID.
	GetByID(ctx context.Context, fileID string) (*model.FileRecord, error)
	// Search выполняет поиск файлов по фильтрам.
	// Возвращает: список файлов, общее количество, ошибка.
	Search(ctx context.Context, params SearchParams) ([]*model.FileRecord, int, error)
	// MarkDeleted обновляет статус файла на 'deleted' (lazy cleanup при 404 от SE).
	MarkDeleted(ctx context.Context, fileID string) error
}

// fileRepo — реализация FileRepository через pgx.
type fileRepo struct {
	db DBTX
}

// NewFileRepository создаёт репозиторий файлов.
func NewFileRepository(db DBTX) FileRepository {
	return &fileRepo{db: db}
}

// GetByID возвращает файл по UUID или ErrNotFound.
func (r *fileRepo) GetByID(ctx context.Context, fileID string) (*model.FileRecord, error) {
	query := fmt.Sprintf(`SELECT %s FROM file_registry WHERE file_id = $1`, fileColumns)

	f := &model.FileRecord{}
	err := r.db.QueryRow(ctx, query, fileID).Scan(
		&f.FileID, &f.OriginalFilename, &f.ContentType, &f.Size, &f.Checksum,
		&f.StorageElementID, &f.UploadedBy, &f.UploadedAt, &f.Description, &f.Tags,
		&f.Status, &f.RetentionPolicy, &f.TTLDays, &f.ExpiresAt, &f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("ошибка получения файла: %w", err)
	}
	return f, nil
}

// Search выполняет поиск файлов с динамическими фильтрами, сортировкой и пагинацией.
// Возвращает (результаты, общее количество, ошибка).
func (r *fileRepo) Search(ctx context.Context, params SearchParams) ([]*model.FileRecord, int, error) {
	// Построение WHERE-условия
	where, args := buildSearchWhere(params, 1)
	argNum := len(args) + 1

	// Сортировка (безопасный whitelist)
	orderBy := buildOrderBy(params.SortBy, params.SortOrder)

	// Запрос данных с пагинацией
	dataQuery := fmt.Sprintf(
		`SELECT %s FROM file_registry %s %s LIMIT $%d OFFSET $%d`,
		fileColumns, where, orderBy, argNum, argNum+1,
	)
	args = append(args, params.Limit, params.Offset)

	rows, err := r.db.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("ошибка поиска файлов: %w", err)
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
			return nil, 0, fmt.Errorf("ошибка сканирования файла: %w", err)
		}
		result = append(result, f)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("ошибка итерации результатов: %w", err)
	}

	// Запрос общего количества (с теми же фильтрами, без LIMIT/OFFSET)
	countWhere, countArgs := buildSearchWhere(params, 1)
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM file_registry %s`, countWhere)

	var total int
	if err := r.db.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ошибка подсчёта файлов: %w", err)
	}

	return result, total, nil
}

// MarkDeleted обновляет статус файла на 'deleted' (lazy cleanup).
// Используется когда SE возвращает 404 — файл удалён GC.
func (r *fileRepo) MarkDeleted(ctx context.Context, fileID string) error {
	query := `
		UPDATE file_registry
		SET status = 'deleted'
		WHERE file_id = $1 AND status != 'deleted'`

	tag, err := r.db.Exec(ctx, query, fileID)
	if err != nil {
		return fmt.Errorf("ошибка пометки файла как удалённого: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// buildSearchWhere строит WHERE-условие и аргументы для поиска файлов.
// startArg — номер первого $-параметра (для корректной нумерации).
//
//nolint:cyclop // сложность обусловлена количеством фильтров
func buildSearchWhere(params SearchParams, startArg int) (whereClause string, args []any) {
	var conditions []string
	argNum := startArg

	// Фильтр по query (поиск по имени файла)
	if params.Query != nil && *params.Query != "" {
		if params.Mode == "exact" {
			// Exact: case-insensitive точное совпадение
			conditions = append(conditions, fmt.Sprintf("LOWER(original_filename) = LOWER($%d)", argNum))
			args = append(args, *params.Query)
		} else {
			// Partial (по умолчанию): ILIKE подстрока
			conditions = append(conditions, fmt.Sprintf("original_filename ILIKE $%d", argNum))
			args = append(args, "%"+*params.Query+"%")
		}
		argNum++
	}

	// Фильтр по filename (всегда partial match — ILIKE)
	if params.Filename != nil && *params.Filename != "" {
		conditions = append(conditions, fmt.Sprintf("original_filename ILIKE $%d", argNum))
		args = append(args, "%"+*params.Filename+"%")
		argNum++
	}

	// Фильтр по расширению файла (exact match по суффиксу)
	if params.FileExtension != nil && *params.FileExtension != "" {
		conditions = append(conditions, fmt.Sprintf("original_filename ILIKE $%d", argNum))
		args = append(args, "%."+*params.FileExtension)
		argNum++
	}

	// Фильтр по тегам (файл должен содержать все указанные теги — оператор @>)
	if params.Tags != nil && len(*params.Tags) > 0 {
		conditions = append(conditions, fmt.Sprintf("tags @> $%d", argNum))
		args = append(args, *params.Tags)
		argNum++
	}

	// Фильтр по загрузившему (exact match)
	if params.UploadedBy != nil && *params.UploadedBy != "" {
		conditions = append(conditions, fmt.Sprintf("uploaded_by = $%d", argNum))
		args = append(args, *params.UploadedBy)
		argNum++
	}

	// Фильтр по политике хранения
	if params.RetentionPolicy != nil && *params.RetentionPolicy != "" {
		conditions = append(conditions, fmt.Sprintf("retention_policy = $%d", argNum))
		args = append(args, *params.RetentionPolicy)
		argNum++
	}

	// Фильтр по статусу
	if params.Status != nil && *params.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argNum))
		args = append(args, *params.Status)
		argNum++
	}

	// Фильтр по минимальному размеру
	if params.MinSize != nil {
		conditions = append(conditions, fmt.Sprintf("size >= $%d", argNum))
		args = append(args, *params.MinSize)
		argNum++
	}

	// Фильтр по максимальному размеру
	if params.MaxSize != nil {
		conditions = append(conditions, fmt.Sprintf("size <= $%d", argNum))
		args = append(args, *params.MaxSize)
		argNum++
	}

	// Фильтр по дате загрузки (после)
	if params.UploadedAfter != nil {
		conditions = append(conditions, fmt.Sprintf("uploaded_at >= $%d", argNum))
		args = append(args, *params.UploadedAfter)
		argNum++
	}

	// Фильтр по дате загрузки (до)
	if params.UploadedBefore != nil {
		conditions = append(conditions, fmt.Sprintf("uploaded_at <= $%d", argNum))
		args = append(args, *params.UploadedBefore)
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}
	return where, args
}

// Допустимые поля сортировки (whitelist для предотвращения SQL-инъекций).
const defaultSortColumn = "uploaded_at"

// buildOrderBy строит ORDER BY с безопасным whitelist полей.
// Предотвращает SQL-инъекции — только разрешённые значения.
func buildOrderBy(sortBy, sortOrder string) string {
	// Whitelist допустимых полей сортировки
	column := defaultSortColumn
	switch sortBy {
	case "original_filename":
		column = "original_filename"
	case "size":
		column = "size"
	case defaultSortColumn:
		column = defaultSortColumn
	}

	// Whitelist направлений сортировки
	direction := "DESC"
	if strings.EqualFold(sortOrder, "asc") {
		direction = "ASC"
	}

	return fmt.Sprintf("ORDER BY %s %s", column, direction)
}
