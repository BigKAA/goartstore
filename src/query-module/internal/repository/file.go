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

// fileColumnsWithSEMode — столбцы file_registry + se.mode через JOIN.
// Используется в запросах, где нужен режим Storage Element.
const fileColumnsWithSEMode = `fr.file_id, fr.original_filename, fr.content_type, fr.size, fr.checksum,
	fr.storage_element_id, fr.uploaded_by, fr.uploaded_at, fr.description, fr.tags,
	fr.retention_policy, fr.ttl_days, fr.expires_at, fr.created_at, fr.updated_at,
	COALESCE(se.mode, '') AS se_mode`

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
// QM использует read-only операции + Delete для lazy cleanup (hard delete).
type FileRepository interface {
	// GetByID возвращает файл по UUID.
	GetByID(ctx context.Context, fileID string) (*model.FileRecord, error)
	// Search выполняет поиск файлов по фильтрам.
	// Возвращает: список файлов, общее количество, ошибка.
	Search(ctx context.Context, params SearchParams) ([]*model.FileRecord, int, error)
	// Delete физически удаляет запись файла из БД (lazy cleanup при 404 от SE).
	Delete(ctx context.Context, fileID string) error
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
// Включает JOIN с storage_elements для получения se_mode.
func (r *fileRepo) GetByID(ctx context.Context, fileID string) (*model.FileRecord, error) {
	query := fmt.Sprintf(
		`SELECT %s FROM file_registry fr
		LEFT JOIN storage_elements se ON fr.storage_element_id = se.id
		WHERE fr.file_id = $1`, fileColumnsWithSEMode)

	f := &model.FileRecord{}
	err := r.db.QueryRow(ctx, query, fileID).Scan(
		&f.FileID, &f.OriginalFilename, &f.ContentType, &f.Size, &f.Checksum,
		&f.StorageElementID, &f.UploadedBy, &f.UploadedAt, &f.Description, &f.Tags,
		&f.RetentionPolicy, &f.TTLDays, &f.ExpiresAt, &f.CreatedAt, &f.UpdatedAt,
		&f.SEMode,
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
// Включает JOIN с storage_elements для получения se_mode.
// Возвращает (результаты, общее количество, ошибка).
func (r *fileRepo) Search(ctx context.Context, params SearchParams) ([]*model.FileRecord, int, error) {
	// Построение WHERE-условия (с алиасом fr для file_registry)
	where, args := buildSearchWhere(params, 1)
	argNum := len(args) + 1

	// Сортировка (безопасный whitelist)
	orderBy := buildOrderBy(params.SortBy, params.SortOrder)

	// Запрос данных с пагинацией (JOIN storage_elements для se_mode)
	dataQuery := fmt.Sprintf(
		`SELECT %s FROM file_registry fr
		LEFT JOIN storage_elements se ON fr.storage_element_id = se.id
		%s %s LIMIT $%d OFFSET $%d`,
		fileColumnsWithSEMode, where, orderBy, argNum, argNum+1,
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
			&f.RetentionPolicy, &f.TTLDays, &f.ExpiresAt, &f.CreatedAt, &f.UpdatedAt,
			&f.SEMode,
		); err != nil {
			return nil, 0, fmt.Errorf("ошибка сканирования файла: %w", err)
		}
		result = append(result, f)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("ошибка итерации результатов: %w", err)
	}

	// Запрос общего количества (с теми же фильтрами, без LIMIT/OFFSET)
	// COUNT не требует JOIN — se_mode не влияет на подсчёт
	countWhere, countArgs := buildSearchWhere(params, 1)
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM file_registry fr %s`, countWhere)

	var total int
	if err := r.db.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("ошибка подсчёта файлов: %w", err)
	}

	return result, total, nil
}

// Delete физически удаляет запись файла из БД (hard delete).
// Используется при lazy cleanup когда SE возвращает 404 — файл физически отсутствует.
func (r *fileRepo) Delete(ctx context.Context, fileID string) error {
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

// buildSearchWhere строит WHERE-условие и аргументы для поиска файлов.
// startArg — номер первого $-параметра (для корректной нумерации).
// Все столбцы используют алиас fr. (file_registry) для совместимости с JOIN.
//
//nolint:cyclop // сложность обусловлена количеством фильтров
func buildSearchWhere(params SearchParams, startArg int) (whereClause string, args []any) {
	var conditions []string
	argNum := startArg

	// Фильтр по query (поиск по имени файла)
	// normalize(NFC) — для корректного сравнения кириллицы:
	// macOS сохраняет имена файлов в NFD (й = и + ◌̆), а браузер отправляет NFC (й = одна буква).
	// Без нормализации ILIKE не сопоставит разные байтовые представления одного символа.
	if params.Query != nil && *params.Query != "" {
		if params.Mode == "exact" {
			// Exact: case-insensitive точное совпадение
			conditions = append(conditions, fmt.Sprintf(
				"LOWER(normalize(fr.original_filename, NFC)) = LOWER(normalize($%d, NFC))", argNum))
			args = append(args, *params.Query)
		} else {
			// Partial (по умолчанию): ILIKE подстрока
			conditions = append(conditions, fmt.Sprintf(
				"normalize(fr.original_filename, NFC) ILIKE normalize($%d, NFC)", argNum))
			args = append(args, "%"+*params.Query+"%")
		}
		argNum++
	}

	// Фильтр по filename (всегда partial match — ILIKE)
	if params.Filename != nil && *params.Filename != "" {
		conditions = append(conditions, fmt.Sprintf(
			"normalize(fr.original_filename, NFC) ILIKE normalize($%d, NFC)", argNum))
		args = append(args, "%"+*params.Filename+"%")
		argNum++
	}

	// Фильтр по расширению файла (exact match по суффиксу)
	if params.FileExtension != nil && *params.FileExtension != "" {
		conditions = append(conditions, fmt.Sprintf(
			"normalize(fr.original_filename, NFC) ILIKE normalize($%d, NFC)", argNum))
		args = append(args, "%."+*params.FileExtension)
		argNum++
	}

	// Фильтр по тегам (файл должен содержать все указанные теги — оператор @>)
	if params.Tags != nil && len(*params.Tags) > 0 {
		conditions = append(conditions, fmt.Sprintf("fr.tags @> $%d", argNum))
		args = append(args, *params.Tags)
		argNum++
	}

	// Фильтр по загрузившему (exact match)
	if params.UploadedBy != nil && *params.UploadedBy != "" {
		conditions = append(conditions, fmt.Sprintf("fr.uploaded_by = $%d", argNum))
		args = append(args, *params.UploadedBy)
		argNum++
	}

	// Фильтр по политике хранения
	if params.RetentionPolicy != nil && *params.RetentionPolicy != "" {
		conditions = append(conditions, fmt.Sprintf("fr.retention_policy = $%d", argNum))
		args = append(args, *params.RetentionPolicy)
		argNum++
	}

	// Фильтр по минимальному размеру
	if params.MinSize != nil {
		conditions = append(conditions, fmt.Sprintf("fr.size >= $%d", argNum))
		args = append(args, *params.MinSize)
		argNum++
	}

	// Фильтр по максимальному размеру
	if params.MaxSize != nil {
		conditions = append(conditions, fmt.Sprintf("fr.size <= $%d", argNum))
		args = append(args, *params.MaxSize)
		argNum++
	}

	// Фильтр по дате загрузки (после)
	if params.UploadedAfter != nil {
		conditions = append(conditions, fmt.Sprintf("fr.uploaded_at >= $%d", argNum))
		args = append(args, *params.UploadedAfter)
		argNum++
	}

	// Фильтр по дате загрузки (до)
	if params.UploadedBefore != nil {
		conditions = append(conditions, fmt.Sprintf("fr.uploaded_at <= $%d", argNum))
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
// Все столбцы используют алиас fr. для совместимости с JOIN.
// Предотвращает SQL-инъекции — только разрешённые значения.
func buildOrderBy(sortBy, sortOrder string) string {
	// Whitelist допустимых полей сортировки (с алиасом fr.)
	column := "fr." + defaultSortColumn
	switch sortBy {
	case "original_filename":
		column = "fr.original_filename"
	case "size":
		column = "fr.size"
	case defaultSortColumn:
		column = "fr." + defaultSortColumn
	}

	// Whitelist направлений сортировки
	direction := "DESC"
	if strings.EqualFold(sortOrder, "asc") {
		direction = "ASC"
	}

	return fmt.Sprintf("ORDER BY %s %s", column, direction)
}
