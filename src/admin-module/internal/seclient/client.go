// Пакет seclient — HTTP-клиент для взаимодействия с Storage Elements.
// Поддерживает TLS с кастомным CA (AM_SE_CA_CERT_PATH).
// Операции: Info (GET /api/v1/info), ListFiles (GET /api/v1/files) с пагинацией.
package seclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// TokenProvider — функция, возвращающая JWT для авторизации запросов к SE.
// Получает токен от Keycloak через Client Credentials flow.
type TokenProvider func(ctx context.Context) (string, error)

// SEInfo — информация о Storage Element (ответ GET /api/v1/info).
type SEInfo struct {
	StorageID   string `json:"storage_id"`
	Mode        string `json:"mode"`
	Status      string `json:"status"`
	Version     string `json:"version"`
	Capacity    *SECapacity `json:"capacity,omitempty"`
	ReplicaMode string `json:"replica_mode,omitempty"`
}

// SECapacity — данные о ёмкости Storage Element.
type SECapacity struct {
	TotalBytes     int64 `json:"total_bytes"`
	UsedBytes      int64 `json:"used_bytes"`
	AvailableBytes int64 `json:"available_bytes"`
}

// SEFileMetadata — метаданные файла на Storage Element (ответ GET /api/v1/files).
type SEFileMetadata struct {
	FileID           string   `json:"file_id"`
	OriginalFilename string   `json:"original_filename"`
	ContentType      string   `json:"content_type"`
	Size             int64    `json:"size"`
	Checksum         string   `json:"checksum"`
	UploadedBy       string   `json:"uploaded_by"`
	UploadedAt       string   `json:"uploaded_at"`
	Status           string   `json:"status"`
	RetentionPolicy  string   `json:"retention_policy"`
	TTLDays          *int     `json:"ttl_days,omitempty"`
	ExpiresAt        *string  `json:"expires_at,omitempty"`
	Description      *string  `json:"description,omitempty"`
	Tags             []string `json:"tags,omitempty"`
}

// FileListResponse — ответ SE на GET /api/v1/files с пагинацией.
type FileListResponse struct {
	Files      []SEFileMetadata `json:"files"`
	Total      int              `json:"total"`
	Limit      int              `json:"limit"`
	Offset     int              `json:"offset"`
}

// Client — HTTP-клиент для Storage Elements.
type Client struct {
	httpClient    *http.Client
	tokenProvider TokenProvider
	logger        *slog.Logger
}

// New создаёт SE-клиент.
// caCertPath — путь к CA-сертификату для TLS (пустая строка — стандартный пул).
// tokenProvider — функция для получения JWT (может быть nil для public endpoints).
func New(caCertPath string, tokenProvider TokenProvider, logger *slog.Logger) (*Client, error) {
	httpClient := &http.Client{Timeout: 30 * time.Second}

	if caCertPath != "" {
		tlsConfig, err := buildTLSConfig(caCertPath)
		if err != nil {
			return nil, fmt.Errorf("загрузка CA-сертификата SE: %w", err)
		}
		httpClient.Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
		logger.Info("CA-сертификат SE добавлен в пул доверия",
			slog.String("ca_cert", caCertPath),
		)
	}

	return &Client{
		httpClient:    httpClient,
		tokenProvider: tokenProvider,
		logger:        logger.With(slog.String("component", "se_client")),
	}, nil
}

// buildTLSConfig создаёт TLS-конфигурацию с кастомным CA.
func buildTLSConfig(caCertPath string) (*tls.Config, error) {
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("чтение CA-сертификата: %w", err)
	}

	caCertPool, err := x509.SystemCertPool()
	if err != nil {
		caCertPool = x509.NewCertPool()
	}
	caCertPool.AppendCertsFromPEM(caCert)

	return &tls.Config{
		RootCAs: caCertPool,
	}, nil
}

// Info запрашивает информацию о Storage Element.
// GET /api/v1/info — публичный endpoint, не требует авторизации.
func (c *Client) Info(ctx context.Context, seURL string) (*SEInfo, error) {
	reqURL := normalizeURL(seURL) + "/api/v1/info"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("создание запроса Info: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("запрос Info к %s: %w", seURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("SE %s вернул статус %d: %s", seURL, resp.StatusCode, string(body))
	}

	var info SEInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("декодирование Info от %s: %w", seURL, err)
	}

	return &info, nil
}

// ListFiles запрашивает список файлов у Storage Element с пагинацией.
// GET /api/v1/files?limit=N&offset=M — требует авторизации (scope: files:read).
func (c *Client) ListFiles(ctx context.Context, seURL string, limit, offset int) (*FileListResponse, error) {
	reqURL := fmt.Sprintf("%s/api/v1/files?limit=%d&offset=%d", normalizeURL(seURL), limit, offset)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("создание запроса ListFiles: %w", err)
	}

	// Добавляем авторизацию
	if c.tokenProvider != nil {
		token, err := c.tokenProvider(ctx)
		if err != nil {
			return nil, fmt.Errorf("получение токена для SE: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("запрос ListFiles к %s: %w", seURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("SE %s ListFiles вернул статус %d: %s", seURL, resp.StatusCode, string(body))
	}

	var fileResp FileListResponse
	if err := json.NewDecoder(resp.Body).Decode(&fileResp); err != nil {
		return nil, fmt.Errorf("декодирование ListFiles от %s: %w", seURL, err)
	}

	return &fileResp, nil
}

// normalizeURL убирает trailing slash из URL.
func normalizeURL(rawURL string) string {
	return strings.TrimRight(rawURL, "/")
}
