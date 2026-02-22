package seclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// testLogger создаёт logger для тестов.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// setupMockSE создаёт mock HTTP-сервер Storage Element.
func setupMockSE(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

// mockTokenProvider возвращает фиксированный токен.
func mockTokenProvider(token string) TokenProvider {
	return func(ctx context.Context) (string, error) {
		return token, nil
	}
}

// mockTokenProviderError возвращает ошибку.
func mockTokenProviderError() TokenProvider {
	return func(ctx context.Context) (string, error) {
		return "", fmt.Errorf("ошибка получения токена")
	}
}

// TestClient_Info проверяет Info (GET /api/v1/info).
func TestClient_Info(t *testing.T) {
	server := setupMockSE(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/info" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Info — публичный endpoint, не должен требовать авторизацию
		if r.Header.Get("Authorization") != "" {
			t.Error("Info не должен передавать Authorization header")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SEInfo{
			StorageID: "se-001",
			Mode:      "rw",
			Status:    "online",
			Version:   "1.0.0",
			Capacity: &SECapacity{
				TotalBytes:     1099511627776,
				UsedBytes:      536870912000,
				AvailableBytes: 562640715776,
			},
		})
	})

	client, err := New("", nil, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	info, err := client.Info(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Ошибка Info: %v", err)
	}

	if info.StorageID != "se-001" {
		t.Errorf("ожидался StorageID=se-001, получен %s", info.StorageID)
	}
	if info.Mode != "rw" {
		t.Errorf("ожидался Mode=rw, получен %s", info.Mode)
	}
	if info.Status != "online" {
		t.Errorf("ожидался Status=online, получен %s", info.Status)
	}
	if info.Capacity == nil {
		t.Fatal("ожидался Capacity != nil")
	}
	if info.Capacity.TotalBytes != 1099511627776 {
		t.Errorf("ожидался TotalBytes=1099511627776, получен %d", info.Capacity.TotalBytes)
	}
}

// TestClient_Info_TrailingSlash проверяет Info с trailing slash в URL.
func TestClient_Info_TrailingSlash(t *testing.T) {
	server := setupMockSE(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/info" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(SEInfo{StorageID: "se-002", Mode: "ro", Status: "online"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	client, err := New("", nil, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	info, err := client.Info(context.Background(), server.URL+"/")
	if err != nil {
		t.Fatalf("Ошибка Info: %v", err)
	}

	if info.StorageID != "se-002" {
		t.Errorf("ожидался StorageID=se-002, получен %s", info.StorageID)
	}
}

// TestClient_Info_Error проверяет обработку ошибок Info.
func TestClient_Info_Error(t *testing.T) {
	server := setupMockSE(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("service unavailable"))
	})

	client, err := New("", nil, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Info(context.Background(), server.URL)
	if err == nil {
		t.Fatal("ожидалась ошибка, получен nil")
	}
}

// TestClient_Info_Unreachable проверяет обработку недоступного SE.
func TestClient_Info_Unreachable(t *testing.T) {
	client, err := New("", nil, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Info(context.Background(), "http://localhost:1")
	if err == nil {
		t.Fatal("ожидалась ошибка, получен nil")
	}
}

// TestClient_ListFiles проверяет ListFiles (GET /api/v1/files).
func TestClient_ListFiles(t *testing.T) {
	server := setupMockSE(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/files" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Проверяем авторизацию
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Проверяем пагинацию
		limit := r.URL.Query().Get("limit")
		offset := r.URL.Query().Get("offset")
		if limit != "100" {
			t.Errorf("ожидался limit=100, получен %s", limit)
		}
		if offset != "0" {
			t.Errorf("ожидался offset=0, получен %s", offset)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(FileListResponse{
			Files: []SEFileMetadata{
				{
					FileID:           "file-001",
					OriginalFilename: "photo.jpg",
					ContentType:      "image/jpeg",
					Size:             1024000,
					Checksum:         "sha256:abc123",
					UploadedBy:       "user-1",
					UploadedAt:       "2024-01-15T10:30:00Z",
					Status:           "active",
					RetentionPolicy:  "permanent",
				},
				{
					FileID:           "file-002",
					OriginalFilename: "document.pdf",
					ContentType:      "application/pdf",
					Size:             2048000,
					Checksum:         "sha256:def456",
					UploadedBy:       "user-2",
					UploadedAt:       "2024-01-16T14:00:00Z",
					Status:           "active",
					RetentionPolicy:  "temporary",
					TTLDays:          intPtr(30),
				},
			},
			Total:  2,
			Limit:  100,
			Offset: 0,
		})
	})

	client, err := New("", mockTokenProvider("test-token"), testLogger())
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.ListFiles(context.Background(), server.URL, 100, 0)
	if err != nil {
		t.Fatalf("Ошибка ListFiles: %v", err)
	}

	if resp.Total != 2 {
		t.Errorf("ожидался Total=2, получен %d", resp.Total)
	}
	if len(resp.Files) != 2 {
		t.Errorf("ожидалось 2 файла, получено %d", len(resp.Files))
	}
	if resp.Files[0].FileID != "file-001" {
		t.Errorf("ожидался FileID=file-001, получен %s", resp.Files[0].FileID)
	}
	if resp.Files[0].OriginalFilename != "photo.jpg" {
		t.Errorf("ожидался OriginalFilename=photo.jpg, получен %s", resp.Files[0].OriginalFilename)
	}
	if resp.Files[1].TTLDays == nil || *resp.Files[1].TTLDays != 30 {
		t.Error("ожидался TTLDays=30 для второго файла")
	}
}

// TestClient_ListFiles_NoToken проверяет ListFiles без tokenProvider.
func TestClient_ListFiles_NoToken(t *testing.T) {
	server := setupMockSE(t, func(w http.ResponseWriter, r *http.Request) {
		// Без токена — ответ без Authorization
		if r.Header.Get("Authorization") != "" {
			t.Error("не ожидался Authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(FileListResponse{
			Files:  nil,
			Total:  0,
			Limit:  100,
			Offset: 0,
		})
	})

	client, err := New("", nil, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.ListFiles(context.Background(), server.URL, 100, 0)
	if err != nil {
		t.Fatalf("Ошибка ListFiles: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("ожидался Total=0, получен %d", resp.Total)
	}
}

// TestClient_ListFiles_TokenError проверяет ошибку получения токена.
func TestClient_ListFiles_TokenError(t *testing.T) {
	server := setupMockSE(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("запрос не должен быть отправлен")
	})

	client, err := New("", mockTokenProviderError(), testLogger())
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.ListFiles(context.Background(), server.URL, 100, 0)
	if err == nil {
		t.Fatal("ожидалась ошибка, получен nil")
	}
}

// TestClient_ListFiles_Unauthorized проверяет 401 ответ от SE.
func TestClient_ListFiles_Unauthorized(t *testing.T) {
	server := setupMockSE(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"code":"UNAUTHORIZED","message":"invalid token"}}`))
	})

	client, err := New("", mockTokenProvider("bad-token"), testLogger())
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.ListFiles(context.Background(), server.URL, 100, 0)
	if err == nil {
		t.Fatal("ожидалась ошибка, получен nil")
	}
}

// TestClient_ListFiles_Pagination проверяет пагинацию ListFiles.
func TestClient_ListFiles_Pagination(t *testing.T) {
	requestCount := 0

	server := setupMockSE(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		offset := r.URL.Query().Get("offset")
		limit := r.URL.Query().Get("limit")

		w.Header().Set("Content-Type", "application/json")

		if offset == "0" && limit == "2" {
			json.NewEncoder(w).Encode(FileListResponse{
				Files: []SEFileMetadata{
					{FileID: "file-001"},
					{FileID: "file-002"},
				},
				Total:  5,
				Limit:  2,
				Offset: 0,
			})
		} else if offset == "2" && limit == "2" {
			json.NewEncoder(w).Encode(FileListResponse{
				Files: []SEFileMetadata{
					{FileID: "file-003"},
					{FileID: "file-004"},
				},
				Total:  5,
				Limit:  2,
				Offset: 2,
			})
		} else if offset == "4" && limit == "2" {
			json.NewEncoder(w).Encode(FileListResponse{
				Files: []SEFileMetadata{
					{FileID: "file-005"},
				},
				Total:  5,
				Limit:  2,
				Offset: 4,
			})
		}
	})

	client, err := New("", mockTokenProvider("token"), testLogger())
	if err != nil {
		t.Fatal(err)
	}

	// Первая страница
	resp1, err := client.ListFiles(context.Background(), server.URL, 2, 0)
	if err != nil {
		t.Fatalf("Ошибка первой страницы: %v", err)
	}
	if len(resp1.Files) != 2 {
		t.Errorf("ожидалось 2 файла, получено %d", len(resp1.Files))
	}
	if resp1.Total != 5 {
		t.Errorf("ожидался Total=5, получен %d", resp1.Total)
	}

	// Вторая страница
	resp2, err := client.ListFiles(context.Background(), server.URL, 2, 2)
	if err != nil {
		t.Fatalf("Ошибка второй страницы: %v", err)
	}
	if len(resp2.Files) != 2 {
		t.Errorf("ожидалось 2 файла, получено %d", len(resp2.Files))
	}

	// Третья страница (неполная)
	resp3, err := client.ListFiles(context.Background(), server.URL, 2, 4)
	if err != nil {
		t.Fatalf("Ошибка третьей страницы: %v", err)
	}
	if len(resp3.Files) != 1 {
		t.Errorf("ожидался 1 файл, получено %d", len(resp3.Files))
	}

	if requestCount != 3 {
		t.Errorf("ожидалось 3 запроса, было %d", requestCount)
	}
}

// TestNormalizeURL проверяет normalizeURL.
func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://se1.kryukov.lan", "https://se1.kryukov.lan"},
		{"https://se1.kryukov.lan/", "https://se1.kryukov.lan"},
		{"https://se1.kryukov.lan///", "https://se1.kryukov.lan"},
		{"http://localhost:8010", "http://localhost:8010"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("ожидалось %q, получено %q", tt.expected, result)
			}
		})
	}
}

func intPtr(v int) *int {
	return &v
}
