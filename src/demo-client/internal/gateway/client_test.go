package gateway

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// newTestClient создаёт Gateway Client, настроенный на тестовый HTTP-сервер.
func newTestClient(serverURL string) *Client {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewClient(ClientConfig{
		BaseURL:        serverURL,
		RequestTimeout: 5 * time.Second,
		UploadTimeout:  10 * time.Second,
	}, func() string { return "test-token-123" }, logger)
}

// TestAuthHeaderInjection проверяет, что Bearer токен добавляется ко всем запросам.
func TestAuthHeaderInjection(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"items":[],"total":0,"limit":100,"offset":0,"has_more":false}`))
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	_, err := client.Search(SearchRequest{Query: "test"})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	expected := "Bearer test-token-123"
	if receivedAuth != expected {
		t.Errorf("ожидался заголовок '%s', получен '%s'", expected, receivedAuth)
	}
}

// TestEmptyTokenInjection проверяет поведение при пустом токене.
func TestEmptyTokenInjection(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"items":[],"total":0,"limit":100,"offset":0,"has_more":false}`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	client := NewClient(ClientConfig{
		BaseURL:        server.URL,
		RequestTimeout: 5 * time.Second,
	}, func() string { return "" }, logger)

	_, err := client.Search(SearchRequest{})
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	if receivedAuth != "" {
		t.Errorf("при пустом токене не должен быть Authorization заголовок, получен '%s'", receivedAuth)
	}
}

// TestHTTPErrorMapping проверяет маппинг HTTP-кодов в типизированные ошибки.
func TestHTTPErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		errorCode  string
		wantErr    error
	}{
		{
			name:       "400 → ErrValidation",
			statusCode: 400,
			errorCode:  "VALIDATION_ERROR",
			wantErr:    ErrValidation,
		},
		{
			name:       "401 → ErrUnauthorized",
			statusCode: 401,
			errorCode:  "UNAUTHORIZED",
			wantErr:    ErrUnauthorized,
		},
		{
			name:       "403 → ErrForbidden",
			statusCode: 403,
			errorCode:  "FORBIDDEN",
			wantErr:    ErrForbidden,
		},
		{
			name:       "404 → ErrNotFound",
			statusCode: 404,
			errorCode:  "NOT_FOUND",
			wantErr:    ErrNotFound,
		},
		{
			name:       "410 → ErrFileArchived",
			statusCode: 410,
			errorCode:  "FILE_ARCHIVED",
			wantErr:    ErrFileArchived,
		},
		{
			name:       "413 → ErrFileTooLarge",
			statusCode: 413,
			errorCode:  "FILE_TOO_LARGE",
			wantErr:    ErrFileTooLarge,
		},
		{
			name:       "502 → ErrServiceUnavailable",
			statusCode: 502,
			errorCode:  "SE_UNAVAILABLE",
			wantErr:    ErrServiceUnavailable,
		},
		{
			name:       "503 → ErrServiceUnavailable",
			statusCode: 503,
			errorCode:  "SERVICE_UNAVAILABLE",
			wantErr:    ErrServiceUnavailable,
		},
		{
			name:       "507 → ErrStorageFull",
			statusCode: 507,
			errorCode:  "STORAGE_FULL",
			wantErr:    ErrStorageFull,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				json.NewEncoder(w).Encode(ErrorResponse{
					Error: &ErrorDetail{
						Code:    tt.errorCode,
						Message: "test error message",
					},
				})
			}))
			defer server.Close()

			client := newTestClient(server.URL)
			_, err := client.Search(SearchRequest{Query: "test"})

			if err == nil {
				t.Fatal("ожидалась ошибка, получен nil")
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ожидалась ошибка %v, получена %v", tt.wantErr, err)
			}

			// Проверяем, что GatewayError содержит код и сообщение.
			var gwErr *GatewayError
			if errors.As(err, &gwErr) {
				if gwErr.Code != tt.errorCode {
					t.Errorf("ожидался код '%s', получен '%s'", tt.errorCode, gwErr.Code)
				}
				if gwErr.StatusCode != tt.statusCode {
					t.Errorf("ожидался HTTP %d, получен %d", tt.statusCode, gwErr.StatusCode)
				}
			} else {
				t.Errorf("ошибка не является GatewayError: %T", err)
			}
		})
	}
}

// TestSearchRequest проверяет корректность формирования search-запроса.
func TestSearchRequest(t *testing.T) {
	var receivedBody string
	var receivedMethod string
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(SearchResponse{
			Items:   []FileInfo{{FileID: "test-id", OriginalFilename: "test.txt"}},
			Total:   1,
			Limit:   100,
			Offset:  0,
			HasMore: false,
		})
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	result, err := client.Search(SearchRequest{
		Query:           "test",
		RetentionPolicy: "temporary",
		Limit:           50,
		SortBy:          "uploaded_at",
		SortOrder:       "desc",
	})

	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	if receivedMethod != "POST" {
		t.Errorf("ожидался метод POST, получен %s", receivedMethod)
	}
	if receivedPath != "/query/api/v1/search" {
		t.Errorf("ожидался путь /query/api/v1/search, получен %s", receivedPath)
	}
	if !strings.Contains(receivedBody, `"query":"test"`) {
		t.Errorf("тело запроса не содержит query: %s", receivedBody)
	}
	if result.Total != 1 {
		t.Errorf("ожидался total 1, получен %d", result.Total)
	}
	if len(result.Items) != 1 || result.Items[0].FileID != "test-id" {
		t.Errorf("неожиданные элементы результата: %v", result.Items)
	}
}

// TestUploadMultipart проверяет формирование multipart upload запроса.
func TestUploadMultipart(t *testing.T) {
	var receivedContentType string
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedContentType = r.Header.Get("Content-Type")

		// Парсим multipart для проверки.
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Errorf("ошибка парсинга multipart: %v", err)
		}

		// Проверяем файл.
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Errorf("ошибка получения файла: %v", err)
		} else {
			defer file.Close()
			if header.Filename != "test.txt" {
				t.Errorf("ожидалось имя файла 'test.txt', получено '%s'", header.Filename)
			}
			content, _ := io.ReadAll(file)
			if string(content) != "hello world" {
				t.Errorf("ожидалось содержимое 'hello world', получено '%s'", string(content))
			}
		}

		// Проверяем form fields.
		if r.FormValue("description") != "тестовый файл" {
			t.Errorf("ожидалось описание 'тестовый файл', получено '%s'", r.FormValue("description"))
		}
		if r.FormValue("retention_policy") != "permanent" {
			t.Errorf("ожидалась retention 'permanent', получена '%s'", r.FormValue("retention_policy"))
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(UploadResult{
			FileID:           "new-file-id",
			OriginalFilename: "test.txt",
			Size:             11,
			Status:           "active",
		})
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	result, err := client.Upload(
		"test.txt",
		strings.NewReader("hello world"),
		11,
		UploadParams{
			Description:     "тестовый файл",
			Tags:            []string{"test", "demo"},
			RetentionPolicy: "permanent",
		},
	)

	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if receivedMethod != "POST" {
		t.Errorf("ожидался метод POST, получен %s", receivedMethod)
	}
	if !strings.Contains(receivedContentType, "multipart/form-data") {
		t.Errorf("ожидался Content-Type multipart/form-data, получен '%s'", receivedContentType)
	}
	if result.FileID != "new-file-id" {
		t.Errorf("ожидался file_id 'new-file-id', получен '%s'", result.FileID)
	}
}

// TestDownloadStreaming проверяет потоковое скачивание файла.
func TestDownloadStreaming(t *testing.T) {
	fileContent := "test file content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/query/api/v1/files/file-123/download" {
			t.Errorf("неожиданный путь: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Disposition", `attachment; filename="test.txt"`)
		w.Header().Set("ETag", `"abc123"`)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fileContent))
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	result, err := client.Download("file-123")

	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	defer result.Body.Close()

	if result.ContentType != "text/plain" {
		t.Errorf("ожидался content-type 'text/plain', получен '%s'", result.ContentType)
	}
	if result.Filename != "test.txt" {
		t.Errorf("ожидалось имя файла 'test.txt', получено '%s'", result.Filename)
	}
	if result.ETag != `"abc123"` {
		t.Errorf("ожидался ETag '\"abc123\"', получен '%s'", result.ETag)
	}

	// Читаем содержимое.
	body, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatalf("ошибка чтения body: %v", err)
	}
	if string(body) != fileContent {
		t.Errorf("ожидалось содержимое '%s', получено '%s'", fileContent, string(body))
	}
}

// TestDownloadFileArchived проверяет обработку 410 FILE_ARCHIVED.
func TestDownloadFileArchived(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGone)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error: &ErrorDetail{
				Code:    "FILE_ARCHIVED",
				Message: "File is in archive storage and not available for download",
			},
		})
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	_, err := client.Download("archived-file-id")

	if err == nil {
		t.Fatal("ожидалась ошибка для archived файла")
	}
	if !errors.Is(err, ErrFileArchived) {
		t.Errorf("ожидалась ошибка ErrFileArchived, получена %v", err)
	}
}

// TestGetMetadata проверяет получение метаданных файла.
func TestGetMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/query/api/v1/files/meta-file-id" {
			t.Errorf("неожиданный путь: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(FileInfo{
			FileID:           "meta-file-id",
			OriginalFilename: "document.pdf",
			ContentType:      "application/pdf",
			Size:             1024000,
			Status:           "active",
			RetentionPolicy:  "permanent",
			SEMode:           "rw",
		})
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	result, err := client.GetMetadata("meta-file-id")

	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if result.FileID != "meta-file-id" {
		t.Errorf("ожидался file_id 'meta-file-id', получен '%s'", result.FileID)
	}
	if result.OriginalFilename != "document.pdf" {
		t.Errorf("ожидалось имя 'document.pdf', получено '%s'", result.OriginalFilename)
	}
	if result.SEMode != "rw" {
		t.Errorf("ожидался se_mode 'rw', получен '%s'", result.SEMode)
	}
}

// TestHealthCheck проверяет health-check backend-сервисов.
func TestHealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/upload/health/ready":
			w.WriteHeader(http.StatusOK)
		case "/query/health/ready":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	results := client.HealthCheck()

	if !results["ingester"].Ready {
		t.Error("ожидался ingester Ready=true")
	}
	if results["query"].Ready {
		t.Error("ожидался query Ready=false")
	}
}

// TestNormalizePath проверяет нормализацию путей для метрик.
func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "/query/api/v1/files/550e8400-e29b-41d4-a716-446655440000/download",
			expected: "/query/api/v1/files/:id/download",
		},
		{
			input:    "/query/api/v1/files/12345",
			expected: "/query/api/v1/files/:id",
		},
		{
			input:    "/upload/api/v1/files/upload",
			expected: "/upload/api/v1/files/upload",
		},
		{
			input:    "/health/ready",
			expected: "/health/ready",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizePath(tt.input)
			if result != tt.expected {
				t.Errorf("normalizePath(%s) = '%s', ожидалось '%s'", tt.input, result, tt.expected)
			}
		})
	}
}

// TestErrorResponseWithoutJSON проверяет обработку ошибки без JSON body.
func TestErrorResponseWithoutJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	client := newTestClient(server.URL)
	_, err := client.Search(SearchRequest{})

	if err == nil {
		t.Fatal("ожидалась ошибка")
	}

	// Ошибка должна быть, даже без JSON body.
	var gwErr *GatewayError
	if errors.As(err, &gwErr) {
		if gwErr.StatusCode != 500 {
			t.Errorf("ожидался HTTP 500, получен %d", gwErr.StatusCode)
		}
	}
}
