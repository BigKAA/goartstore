package repository

import (
	"strings"
	"testing"
)

// --- Тесты buildSearchWhere ---

// TestBuildSearchWhere_Empty проверяет пустые фильтры.
func TestBuildSearchWhere_Empty(t *testing.T) {
	params := SearchParams{}
	where, args := buildSearchWhere(params, 1)

	if where != "" {
		t.Errorf("where = %q, ожидалась пустая строка", where)
	}
	if len(args) != 0 {
		t.Errorf("args count = %d, ожидался 0", len(args))
	}
}

// TestBuildSearchWhere_StatusOnly проверяет фильтрацию по статусу.
func TestBuildSearchWhere_StatusOnly(t *testing.T) {
	status := "active"
	params := SearchParams{Status: &status}
	where, args := buildSearchWhere(params, 1)

	if !strings.Contains(where, "status = $1") {
		t.Errorf("where = %q, ожидалось содержание 'status = $1'", where)
	}
	if len(args) != 1 {
		t.Errorf("args count = %d, ожидался 1", len(args))
	}
	if args[0] != "active" {
		t.Errorf("args[0] = %v, ожидался 'active'", args[0])
	}
}

// TestBuildSearchWhere_QueryPartial проверяет частичный поиск по имени файла.
func TestBuildSearchWhere_QueryPartial(t *testing.T) {
	query := "test"
	params := SearchParams{
		Query: &query,
		Mode:  "partial",
	}
	where, args := buildSearchWhere(params, 1)

	if !strings.Contains(where, "ILIKE") {
		t.Errorf("where = %q, ожидался ILIKE для partial mode", where)
	}
	if len(args) != 1 {
		t.Errorf("args count = %d, ожидался 1", len(args))
	}
	// Должен быть обёрнут в %...%
	if args[0] != "%test%" {
		t.Errorf("args[0] = %v, ожидался '%%test%%'", args[0])
	}
}

// TestBuildSearchWhere_QueryExact проверяет точный поиск по имени файла.
func TestBuildSearchWhere_QueryExact(t *testing.T) {
	query := "exact-file.txt"
	params := SearchParams{
		Query: &query,
		Mode:  "exact",
	}
	where, args := buildSearchWhere(params, 1)

	if !strings.Contains(where, "LOWER(original_filename) = LOWER($1)") {
		t.Errorf("where = %q, ожидался LOWER exact match", where)
	}
	if args[0] != "exact-file.txt" {
		t.Errorf("args[0] = %v, ожидался 'exact-file.txt'", args[0])
	}
}

// TestBuildSearchWhere_Tags проверяет фильтрацию по тегам.
func TestBuildSearchWhere_Tags(t *testing.T) {
	tags := []string{"logo", "draft"}
	params := SearchParams{Tags: &tags}
	where, args := buildSearchWhere(params, 1)

	if !strings.Contains(where, "tags @> $1") {
		t.Errorf("where = %q, ожидался tags @> $1", where)
	}
	if len(args) != 1 {
		t.Errorf("args count = %d, ожидался 1", len(args))
	}
}

// TestBuildSearchWhere_FileExtension проверяет фильтрацию по расширению.
func TestBuildSearchWhere_FileExtension(t *testing.T) {
	ext := "jpg"
	params := SearchParams{FileExtension: &ext}
	where, args := buildSearchWhere(params, 1)

	if !strings.Contains(where, "ILIKE") {
		t.Errorf("where = %q, ожидался ILIKE для file_extension", where)
	}
	if args[0] != "%.jpg" {
		t.Errorf("args[0] = %v, ожидался '%%.jpg'", args[0])
	}
}

// TestBuildSearchWhere_SizeRange проверяет фильтрацию по размеру.
func TestBuildSearchWhere_SizeRange(t *testing.T) {
	minSize := int64(1024)
	maxSize := int64(10485760)
	params := SearchParams{
		MinSize: &minSize,
		MaxSize: &maxSize,
	}
	where, args := buildSearchWhere(params, 1)

	if !strings.Contains(where, "size >= $1") {
		t.Errorf("where = %q, ожидался size >= $1", where)
	}
	if !strings.Contains(where, "size <= $2") {
		t.Errorf("where = %q, ожидался size <= $2", where)
	}
	if len(args) != 2 {
		t.Errorf("args count = %d, ожидался 2", len(args))
	}
}

// TestBuildSearchWhere_MultipleFilters проверяет комбинацию фильтров.
func TestBuildSearchWhere_MultipleFilters(t *testing.T) {
	query := "report"
	status := "active"
	uploadedBy := "admin"
	params := SearchParams{
		Query:      &query,
		Status:     &status,
		UploadedBy: &uploadedBy,
		Mode:       "partial",
	}
	where, args := buildSearchWhere(params, 1)

	// Должно быть 3 условия, объединённых AND
	if strings.Count(where, "AND") != 2 {
		t.Errorf("where = %q, ожидалось 2 AND", where)
	}
	if len(args) != 3 {
		t.Errorf("args count = %d, ожидался 3", len(args))
	}
}

// TestBuildSearchWhere_StartArgOffset проверяет корректную нумерацию аргументов.
func TestBuildSearchWhere_StartArgOffset(t *testing.T) {
	status := "active"
	params := SearchParams{Status: &status}

	// Начинаем с $5 (как если WHERE добавляется после других параметров)
	where, args := buildSearchWhere(params, 5)

	if !strings.Contains(where, "status = $5") {
		t.Errorf("where = %q, ожидался status = $5", where)
	}
	if len(args) != 1 {
		t.Errorf("args count = %d, ожидался 1", len(args))
	}
}

// --- Тесты buildOrderBy ---

// TestBuildOrderBy_Default проверяет сортировку по умолчанию.
func TestBuildOrderBy_Default(t *testing.T) {
	orderBy := buildOrderBy("", "")
	if orderBy != "ORDER BY uploaded_at DESC" {
		t.Errorf("orderBy = %q, ожидался 'ORDER BY uploaded_at DESC'", orderBy)
	}
}

// TestBuildOrderBy_ByFilename проверяет сортировку по имени файла.
func TestBuildOrderBy_ByFilename(t *testing.T) {
	orderBy := buildOrderBy("original_filename", "asc")
	if orderBy != "ORDER BY original_filename ASC" {
		t.Errorf("orderBy = %q, ожидался 'ORDER BY original_filename ASC'", orderBy)
	}
}

// TestBuildOrderBy_BySize проверяет сортировку по размеру.
func TestBuildOrderBy_BySize(t *testing.T) {
	orderBy := buildOrderBy("size", "desc")
	if orderBy != "ORDER BY size DESC" {
		t.Errorf("orderBy = %q, ожидался 'ORDER BY size DESC'", orderBy)
	}
}

// TestBuildOrderBy_InvalidField проверяет безопасность whitelist.
func TestBuildOrderBy_InvalidField(t *testing.T) {
	// SQL-инъекция через sort field — должен fallback на uploaded_at
	orderBy := buildOrderBy("'; DROP TABLE files; --", "asc")
	if !strings.Contains(orderBy, "uploaded_at") {
		t.Errorf("orderBy = %q, ожидался fallback на uploaded_at", orderBy)
	}
}

// TestBuildOrderBy_InvalidDirection проверяет безопасность направления сортировки.
func TestBuildOrderBy_InvalidDirection(t *testing.T) {
	// SQL-инъекция через direction — должен fallback на DESC
	orderBy := buildOrderBy("uploaded_at", "'; DROP TABLE files; --")
	if !strings.Contains(orderBy, "DESC") {
		t.Errorf("orderBy = %q, ожидался fallback на DESC", orderBy)
	}
}
