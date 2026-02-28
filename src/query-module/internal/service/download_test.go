package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bigkaa/goartstore/query-module/internal/adminclient"
	"github.com/bigkaa/goartstore/query-module/internal/domain/model"
	"github.com/bigkaa/goartstore/query-module/internal/repository"
	"github.com/bigkaa/goartstore/query-module/internal/seclient"
)

// --- Mock-сервер для SE ---

// newMockSEServer создаёт тестовый HTTP-сервер, имитирующий Storage Element.
// handler определяет поведение SE для каждого запроса.
func newMockSEServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

// --- Mock Admin Module сервер ---

// newMockAMServer создаёт тестовый HTTP-сервер, имитирующий Admin Module.
// seURL — URL mock SE сервера, который будет возвращён в ответе GetStorageElement.
func newMockAMServer(seURL string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/token" && r.Method == http.MethodPost:
			// Token endpoint — возвращаем тестовый токен
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":3600,"token_type":"bearer"}`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/storage-elements/"):
			// GetStorageElement — возвращаем информацию о SE
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"se-1","name":"test-se","url":"` + seURL + `","mode":"rw","status":"online"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// newTestDownloadService создаёт DownloadService для тестов с mock-серверами.
func newTestDownloadService(
	t *testing.T,
	repo repository.FileRepository,
	amServer *httptest.Server,
	seServer *httptest.Server,
) *DownloadService {
	t.Helper()

	logger := slog.Default()

	// Создаём реальный adminclient с mock AM server
	adminClient, err := adminclient.New(
		amServer.URL,
		"", // без CA
		10*time.Second,
		"test-client",
		"test-secret",
		logger,
	)
	if err != nil {
		t.Fatalf("Ошибка создания adminclient: %v", err)
	}

	// Создаём реальный seclient с mock SE server (tokenProvider через adminclient)
	seClient, err := seclient.New(
		"", // без CA
		30*time.Second,
		adminClient.GetToken,
		logger,
	)
	if err != nil {
		t.Fatalf("Ошибка создания seclient: %v", err)
	}

	cache := NewCacheService(100, 5*time.Minute)

	return NewDownloadService(repo, cache, adminClient, seClient, logger)
}

// --- Тесты DownloadService ---

// TestDownloadService_Success проверяет успешный proxy download.
func TestDownloadService_Success(t *testing.T) {
	fileContent := "test file content"

	// Mock SE — возвращает файл
	seSrv := newMockSEServer(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", "17")
		w.Header().Set("Content-Disposition", `attachment; filename="test.txt"`)
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fileContent))
	})
	defer seSrv.Close()

	// Mock AM — возвращает SE URL
	amSrv := newMockAMServer(seSrv.URL)
	defer amSrv.Close()

	repo := &mockFileRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.FileRecord, error) {
			return &model.FileRecord{
				FileID:           "file-1",
				StorageElementID: "se-1",
				Status:           "active",
			}, nil
		},
	}

	svc := newTestDownloadService(t, repo, amSrv, seSrv)

	// Выполняем download
	rec := httptest.NewRecorder()
	err := svc.Download(context.Background(), rec, "file-1", "")
	if err != nil {
		t.Fatalf("Download ошибка: %v", err)
	}

	// Проверяем ответ
	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, ожидался 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != fileContent {
		t.Errorf("Body = %q, ожидался %q", string(body), fileContent)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("Content-Type = %q, ожидался application/octet-stream", ct)
	}

	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="test.txt"` {
		t.Errorf("Content-Disposition = %q, ожидался attachment; filename=\"test.txt\"", cd)
	}
}

// TestDownloadService_RangeRequest проверяет proxy download с Range header.
func TestDownloadService_RangeRequest(t *testing.T) {
	// Mock SE — проверяет Range header и возвращает 206
	seSrv := newMockSEServer(func(w http.ResponseWriter, r *http.Request) {
		rangeHdr := r.Header.Get("Range")
		if rangeHdr != "bytes=0-9" {
			t.Errorf("Range header = %q, ожидался bytes=0-9", rangeHdr)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Range", "bytes 0-9/17")
		w.Header().Set("Content-Length", "10")
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("test file "))
	})
	defer seSrv.Close()

	amSrv := newMockAMServer(seSrv.URL)
	defer amSrv.Close()

	repo := &mockFileRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.FileRecord, error) {
			return &model.FileRecord{
				FileID:           "file-1",
				StorageElementID: "se-1",
				Status:           "active",
			}, nil
		},
	}

	svc := newTestDownloadService(t, repo, amSrv, seSrv)

	rec := httptest.NewRecorder()
	err := svc.Download(context.Background(), rec, "file-1", "bytes=0-9")
	if err != nil {
		t.Fatalf("Download ошибка: %v", err)
	}

	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		t.Errorf("StatusCode = %d, ожидался 206", resp.StatusCode)
	}

	if cr := rec.Header().Get("Content-Range"); cr != "bytes 0-9/17" {
		t.Errorf("Content-Range = %q, ожидался bytes 0-9/17", cr)
	}
}

// TestDownloadService_LazyCleanup проверяет lazy cleanup при 404 от SE.
func TestDownloadService_LazyCleanup(t *testing.T) {
	markDeletedCalled := false

	// Mock SE — возвращает 404
	seSrv := newMockSEServer(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer seSrv.Close()

	amSrv := newMockAMServer(seSrv.URL)
	defer amSrv.Close()

	repo := &mockFileRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.FileRecord, error) {
			return &model.FileRecord{
				FileID:           "file-1",
				StorageElementID: "se-1",
				Status:           "active",
			}, nil
		},
		markDeletedFn: func(_ context.Context, fileID string) error {
			markDeletedCalled = true
			if fileID != "file-1" {
				t.Errorf("MarkDeleted fileID = %q, ожидался file-1", fileID)
			}
			return nil
		},
	}

	svc := newTestDownloadService(t, repo, amSrv, seSrv)

	rec := httptest.NewRecorder()
	err := svc.Download(context.Background(), rec, "file-1", "")
	if err == nil {
		t.Fatal("ожидалась ошибка ErrFileDeleted")
	}
	if !errors.Is(err, ErrFileDeleted) {
		t.Errorf("ошибка = %v, ожидалась ErrFileDeleted", err)
	}

	if !markDeletedCalled {
		t.Error("MarkDeleted не был вызван (lazy cleanup)")
	}
}

// TestDownloadService_FileNotFound проверяет ErrNotFound при отсутствии файла в БД.
func TestDownloadService_FileNotFound(t *testing.T) {
	// Mock SE/AM не нужны — ошибка на уровне repository
	seSrv := newMockSEServer(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer seSrv.Close()

	amSrv := newMockAMServer(seSrv.URL)
	defer amSrv.Close()

	repo := &mockFileRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.FileRecord, error) {
			return nil, repository.ErrNotFound
		},
	}

	svc := newTestDownloadService(t, repo, amSrv, seSrv)

	rec := httptest.NewRecorder()
	err := svc.Download(context.Background(), rec, "non-existent", "")
	if err == nil {
		t.Fatal("ожидалась ошибка ErrNotFound")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("ошибка = %v, ожидалась ErrNotFound", err)
	}
}

// TestDownloadService_DeletedFile проверяет ErrNotFound для файла со статусом deleted.
func TestDownloadService_DeletedFile(t *testing.T) {
	seSrv := newMockSEServer(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer seSrv.Close()

	amSrv := newMockAMServer(seSrv.URL)
	defer amSrv.Close()

	repo := &mockFileRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.FileRecord, error) {
			return &model.FileRecord{
				FileID:           "deleted-file",
				StorageElementID: "se-1",
				Status:           "deleted",
			}, nil
		},
	}

	svc := newTestDownloadService(t, repo, amSrv, seSrv)

	rec := httptest.NewRecorder()
	err := svc.Download(context.Background(), rec, "deleted-file", "")
	if err == nil {
		t.Fatal("ожидалась ошибка ErrNotFound")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("ошибка = %v, ожидалась ErrNotFound", err)
	}
}

// TestDownloadService_SEError проверяет ошибку при неожиданном статусе от SE.
func TestDownloadService_SEError(t *testing.T) {
	// Mock SE — возвращает 500
	seSrv := newMockSEServer(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer seSrv.Close()

	amSrv := newMockAMServer(seSrv.URL)
	defer amSrv.Close()

	repo := &mockFileRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.FileRecord, error) {
			return &model.FileRecord{
				FileID:           "file-1",
				StorageElementID: "se-1",
				Status:           "active",
			}, nil
		},
	}

	svc := newTestDownloadService(t, repo, amSrv, seSrv)

	rec := httptest.NewRecorder()
	err := svc.Download(context.Background(), rec, "file-1", "")
	if err == nil {
		t.Fatal("ожидалась ошибка при 500 от SE")
	}

	if strings.Contains(err.Error(), "неожиданный статус 500") {
		// Ожидаемая ошибка
		return
	}
	// Тоже допустимо — SE вернул ошибку
}

// TestDownloadService_CacheInvalidation проверяет инвалидацию кэша при lazy cleanup.
func TestDownloadService_CacheInvalidation(t *testing.T) {
	getByIDCount := 0

	// Mock SE — первый запрос 200, второй 404
	callCount := 0
	seSrv := newMockSEServer(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("content"))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer seSrv.Close()

	amSrv := newMockAMServer(seSrv.URL)
	defer amSrv.Close()

	repo := &mockFileRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.FileRecord, error) {
			getByIDCount++
			return &model.FileRecord{
				FileID:           "file-1",
				StorageElementID: "se-1",
				Status:           "active",
			}, nil
		},
		markDeletedFn: func(_ context.Context, _ string) error {
			return nil
		},
	}

	svc := newTestDownloadService(t, repo, amSrv, seSrv)

	// Первый download — успех, FileRecord кэшируется
	rec1 := httptest.NewRecorder()
	err := svc.Download(context.Background(), rec1, "file-1", "")
	if err != nil {
		t.Fatalf("Первый Download ошибка: %v", err)
	}

	// Второй download — 404, lazy cleanup инвалидирует кэш
	rec2 := httptest.NewRecorder()
	_ = svc.Download(context.Background(), rec2, "file-1", "")

	// Третий download — FileRecord должен загрузиться из БД снова (кэш инвалидирован)
	callCount = 0 // сбрасываем для нового 200
	rec3 := httptest.NewRecorder()
	_ = svc.Download(context.Background(), rec3, "file-1", "")

	// GetByID должен быть вызван минимум 2 раза (первый cache miss + после инвалидации)
	if getByIDCount < 2 {
		t.Errorf("GetByID вызван %d раз, ожидался >= 2 (кэш должен быть инвалидирован)", getByIDCount)
	}
}

// TestDownloadService_AuthorizationHeader проверяет, что SE получает Authorization header.
func TestDownloadService_AuthorizationHeader(t *testing.T) {
	receivedAuth := ""

	seSrv := newMockSEServer(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	defer seSrv.Close()

	amSrv := newMockAMServer(seSrv.URL)
	defer amSrv.Close()

	repo := &mockFileRepo{
		getByIDFn: func(_ context.Context, _ string) (*model.FileRecord, error) {
			return &model.FileRecord{
				FileID:           "file-1",
				StorageElementID: "se-1",
				Status:           "active",
			}, nil
		},
	}

	svc := newTestDownloadService(t, repo, amSrv, seSrv)

	rec := httptest.NewRecorder()
	err := svc.Download(context.Background(), rec, "file-1", "")
	if err != nil {
		t.Fatalf("Download ошибка: %v", err)
	}

	if receivedAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q, ожидался 'Bearer test-token'", receivedAuth)
	}
}
