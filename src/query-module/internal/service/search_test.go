package service

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/bigkaa/goartstore/query-module/internal/domain/model"
	"github.com/bigkaa/goartstore/query-module/internal/repository"
)

// --- Mock repository ---

// mockFileRepo — мок FileRepository для unit-тестов.
type mockFileRepo struct {
	getByIDFn     func(ctx context.Context, fileID string) (*model.FileRecord, error)
	searchFn      func(ctx context.Context, params repository.SearchParams) ([]*model.FileRecord, int, error)
	markDeletedFn func(ctx context.Context, fileID string) error
}

func (m *mockFileRepo) GetByID(ctx context.Context, fileID string) (*model.FileRecord, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, fileID)
	}
	return nil, repository.ErrNotFound
}

func (m *mockFileRepo) Search(ctx context.Context, params repository.SearchParams) ([]*model.FileRecord, int, error) {
	if m.searchFn != nil {
		return m.searchFn(ctx, params)
	}
	return nil, 0, nil
}

func (m *mockFileRepo) MarkDeleted(ctx context.Context, fileID string) error {
	if m.markDeletedFn != nil {
		return m.markDeletedFn(ctx, fileID)
	}
	return nil
}

// --- Тесты SearchService ---

// TestSearchService_Search проверяет выполнение поиска через repository.
func TestSearchService_Search(t *testing.T) {
	files := []*model.FileRecord{
		{FileID: "file-1", OriginalFilename: "test1.txt", Status: "active"},
		{FileID: "file-2", OriginalFilename: "test2.txt", Status: "active"},
	}

	repo := &mockFileRepo{
		searchFn: func(_ context.Context, params repository.SearchParams) ([]*model.FileRecord, int, error) {
			if params.Limit != 100 {
				t.Errorf("Limit = %d, ожидался 100", params.Limit)
			}
			return files, 2, nil
		},
	}

	cache := NewCacheService(100, 5*time.Minute)
	logger := slog.Default()
	svc := NewSearchService(repo, cache, logger)

	result, err := svc.Search(context.Background(), repository.SearchParams{
		Limit:  100,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("Search ошибка: %v", err)
	}

	if result.Total != 2 {
		t.Errorf("Total = %d, ожидался 2", result.Total)
	}
	if len(result.Items) != 2 {
		t.Errorf("Items count = %d, ожидался 2", len(result.Items))
	}
	if result.HasMore {
		t.Error("HasMore = true, ожидался false")
	}
}

// TestSearchService_Search_HasMore проверяет флаг HasMore при пагинации.
func TestSearchService_Search_HasMore(t *testing.T) {
	files := []*model.FileRecord{
		{FileID: "file-1", Status: "active"},
	}

	repo := &mockFileRepo{
		searchFn: func(_ context.Context, _ repository.SearchParams) ([]*model.FileRecord, int, error) {
			return files, 5, nil // total=5, но вернули только 1 (limit=1)
		},
	}

	cache := NewCacheService(100, 5*time.Minute)
	svc := NewSearchService(repo, cache, slog.Default())

	result, err := svc.Search(context.Background(), repository.SearchParams{
		Limit:  1,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("Search ошибка: %v", err)
	}

	if !result.HasMore {
		t.Error("HasMore = false, ожидался true (total=5, offset+items=1)")
	}
}

// TestSearchService_GetFileMetadata_CacheHit проверяет получение из кэша.
func TestSearchService_GetFileMetadata_CacheHit(t *testing.T) {
	callCount := 0
	repo := &mockFileRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.FileRecord, error) {
			callCount++
			return &model.FileRecord{FileID: "cached-file", Status: "active"}, nil
		},
	}

	cache := NewCacheService(100, 5*time.Minute)
	svc := NewSearchService(repo, cache, slog.Default())

	// Первый вызов — cache miss, идёт в БД
	record, err := svc.GetFileMetadata(context.Background(), "cached-file")
	if err != nil {
		t.Fatalf("GetFileMetadata ошибка: %v", err)
	}
	if record.FileID != "cached-file" {
		t.Errorf("FileID = %q, ожидался %q", record.FileID, "cached-file")
	}
	if callCount != 1 {
		t.Errorf("repo.GetByID вызван %d раз, ожидался 1", callCount)
	}

	// Второй вызов — cache hit, в БД не идёт
	record, err = svc.GetFileMetadata(context.Background(), "cached-file")
	if err != nil {
		t.Fatalf("GetFileMetadata ошибка (cache hit): %v", err)
	}
	if record.FileID != "cached-file" {
		t.Errorf("FileID = %q, ожидался %q", record.FileID, "cached-file")
	}
	if callCount != 1 {
		t.Errorf("repo.GetByID вызван %d раз, ожидался 1 (cache hit)", callCount)
	}
}

// TestSearchService_GetFileMetadata_NotFound проверяет ErrNotFound.
func TestSearchService_GetFileMetadata_NotFound(t *testing.T) {
	repo := &mockFileRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.FileRecord, error) {
			return nil, repository.ErrNotFound
		},
	}

	cache := NewCacheService(100, 5*time.Minute)
	svc := NewSearchService(repo, cache, slog.Default())

	_, err := svc.GetFileMetadata(context.Background(), "non-existent")
	if err == nil {
		t.Fatal("ожидалась ошибка ErrNotFound")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("ошибка = %v, ожидалась ErrNotFound", err)
	}
}
