// Пакет adminclient — HTTP-клиент для взаимодействия с Admin Module.
// Получает SA-токен через client_credentials grant, запрашивает список SE,
// регистрирует файлы в реестре и проверяет health endpoint.
package adminclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// SEInfo — информация о Storage Element (из API Admin Module).
type SEInfo struct {
	// ID — UUID записи Storage Element в реестре AM
	ID string `json:"id"`
	// Name — человекочитаемое имя SE
	Name string `json:"name"`
	// URL — базовый URL SE (для загрузки файлов)
	URL string `json:"url"`
	// Mode — режим SE (edit, rw, ro, ar)
	Mode string `json:"mode"`
	// Status — статус SE (online, offline, degraded, maintenance)
	Status string `json:"status"`
	// AvailableBytes — доступное место на SE (nullable: nil = нет данных)
	AvailableBytes *int64 `json:"available_bytes"`
	// Priority — приоритет заполнения (0 = наивысший, lower = higher priority)
	Priority int `json:"priority"`
}

// FileRegisterRequest — запрос на регистрацию файла в Admin Module.
type FileRegisterRequest struct {
	// FileID — UUID файла, присвоённый Storage Element
	FileID string `json:"file_id"`
	// OriginalFilename — оригинальное имя загруженного файла
	OriginalFilename string `json:"original_filename"`
	// ContentType — MIME-тип файла
	ContentType string `json:"content_type"`
	// Size — размер файла в байтах
	Size int64 `json:"size"`
	// Checksum — SHA-256 хэш содержимого файла
	Checksum string `json:"checksum"`
	// StorageElementID — UUID записи SE в реестре AM
	StorageElementID string `json:"storage_element_id"`
	// RetentionPolicy — политика хранения ("temporary" или "permanent")
	RetentionPolicy string `json:"retention_policy"`
	// UploadedBy — идентификатор загрузившего (sub из JWT конечного пользователя)
	UploadedBy string `json:"uploaded_by"`
	// Description — описание файла (опционально)
	Description *string `json:"description,omitempty"`
	// Tags — теги файла (опционально)
	Tags []string `json:"tags,omitempty"`
	// TTLDays — срок хранения в днях (обязателен для temporary, 1-365)
	TTLDays *int `json:"ttl_days,omitempty"`
}

// FileRecord — запись файла из Admin Module (ответ POST /api/v1/files).
type FileRecord struct {
	FileID           string     `json:"file_id"`
	OriginalFilename string     `json:"original_filename"`
	ContentType      string     `json:"content_type"`
	Size             int64      `json:"size"`
	Checksum         string     `json:"checksum"`
	StorageElementID string     `json:"storage_element_id"`
	UploadedBy       string     `json:"uploaded_by"`
	UploadedAt       time.Time  `json:"uploaded_at"`
	Description      *string    `json:"description"`
	Tags             []string   `json:"tags"`
	Status           string     `json:"status"`
	RetentionPolicy  string     `json:"retention_policy"`
	TTLDays          *int       `json:"ttl_days"`
	ExpiresAt        *time.Time `json:"expires_at"`
}

// tokenInfo — закэшированный SA-токен с временем истечения.
type tokenInfo struct {
	accessToken string
	expiresAt   time.Time
}

// seListResponse — обёртка ответа GET /api/v1/storage-elements.
type seListResponse struct {
	Items []SEInfo `json:"items"`
}

// Client — HTTP-клиент для Admin Module.
type Client struct {
	httpClient   *http.Client
	adminURL     string
	tokenURL     string // URL Keycloak token endpoint для client_credentials grant
	clientID     string
	clientSecret string //nolint:gosec // G101: поле структуры, не содержит секрет напрямую
	logger       *slog.Logger

	// Кэш SA-токена (thread-safe)
	mu    sync.RWMutex
	token *tokenInfo
}

// New создаёт Admin Module клиент.
// adminURL — базовый URL Admin Module (например, http://admin-module:8000).
// tokenURL — URL Keycloak token endpoint для client_credentials grant.
// caCertPath — путь к CA-сертификату для TLS (пустая строка — стандартный пул).
// timeout — таймаут HTTP-запросов (из конфигурации IM_ADMIN_TIMEOUT).
func New(
	adminURL string,
	tokenURL string,
	caCertPath string,
	timeout time.Duration,
	clientID string,
	clientSecret string,
	logger *slog.Logger,
) (*Client, error) {
	httpClient := &http.Client{Timeout: timeout}

	if caCertPath != "" {
		tlsConfig, err := buildTLSConfig(caCertPath)
		if err != nil {
			return nil, fmt.Errorf("загрузка CA-сертификата AM: %w", err)
		}
		httpClient.Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
		logger.Info("CA-сертификат AM добавлен в пул доверия",
			slog.String("ca_cert", caCertPath),
		)
	}

	return &Client{
		httpClient:   httpClient,
		adminURL:     strings.TrimRight(adminURL, "/"),
		tokenURL:     strings.TrimRight(tokenURL, "/"),
		clientID:     clientID,
		clientSecret: clientSecret,
		logger:       logger.With(slog.String("component", "admin_client")),
	}, nil
}

// GetToken возвращает SA-токен для авторизации запросов.
// Использует кэш: если токен ещё валиден (exp - 30s), возвращает закэшированный.
// Иначе запрашивает новый через client_credentials grant к Keycloak token endpoint.
func (c *Client) GetToken(ctx context.Context) (string, error) {
	// Проверяем кэш (read lock)
	c.mu.RLock()
	if c.token != nil && time.Now().Before(c.token.expiresAt) {
		token := c.token.accessToken
		c.mu.RUnlock()
		return token, nil
	}
	c.mu.RUnlock()

	// Запрашиваем новый токен (write lock)
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check после получения write lock
	if c.token != nil && time.Now().Before(c.token.expiresAt) {
		return c.token.accessToken, nil
	}

	token, err := c.requestToken(ctx)
	if err != nil {
		return "", err
	}

	return token, nil
}

// GetStorageElements запрашивает список Storage Elements с фильтрами mode и status.
// GET /api/v1/storage-elements?mode={mode}&status={status}
// Пустые параметры не отправляются (опускаются из query string).
// Результат: список SE из AM API. AvailableBytes может быть nil.
func (c *Client) GetStorageElements(ctx context.Context, mode, status string) ([]SEInfo, error) {
	params := url.Values{}
	if mode != "" {
		params.Set("mode", mode)
	}
	if status != "" {
		params.Set("status", status)
	}
	reqURL := fmt.Sprintf("%s/api/v1/storage-elements?%s", c.adminURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("создание запроса GetStorageElements: %w", err)
	}

	// SA-токен для авторизации
	token, err := c.GetToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("получение токена для AM: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req) //nolint:gosec // G704: URL из конфигурации
	if err != nil {
		return nil, fmt.Errorf("запрос GetStorageElements к %s: %w", c.adminURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AM вернул статус %d для GetStorageElements: %s", resp.StatusCode, string(body))
	}

	var listResp seListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("декодирование ответа SE list от AM: %w", err)
	}

	c.logger.Debug("Получен список SE из AM",
		slog.String("mode", mode),
		slog.String("status", status),
		slog.Int("count", len(listResp.Items)),
	)

	return listResp.Items, nil
}

// RegisterFile регистрирует файл в реестре Admin Module.
// POST /api/v1/files с телом FileRegisterRequest.
// Возвращает FileRecord из AM (включая uploaded_at, status, expires_at).
func (c *Client) RegisterFile(ctx context.Context, fileReq FileRegisterRequest) (*FileRecord, error) {
	reqURL := c.adminURL + "/api/v1/files"

	body, err := json.Marshal(fileReq)
	if err != nil {
		return nil, fmt.Errorf("маршалинг FileRegisterRequest: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("создание запроса RegisterFile: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// SA-токен для авторизации
	token, err := c.GetToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("получение токена для AM: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req) //nolint:gosec // G704: URL из конфигурации
	if err != nil {
		return nil, fmt.Errorf("запрос RegisterFile к %s: %w", c.adminURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AM вернул статус %d для RegisterFile: %s", resp.StatusCode, string(respBody))
	}

	var record FileRecord
	if err := json.NewDecoder(resp.Body).Decode(&record); err != nil {
		return nil, fmt.Errorf("декодирование FileRecord от AM: %w", err)
	}

	c.logger.Debug("Файл зарегистрирован в AM",
		slog.String("file_id", record.FileID),
		slog.String("storage_element_id", record.StorageElementID),
	)

	return &record, nil
}

// CheckHealth проверяет готовность Admin Module.
// GET /health/ready — используется для readiness check Ingester Module.
func (c *Client) CheckHealth(ctx context.Context) error {
	reqURL := c.adminURL + "/health/ready"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("создание запроса CheckHealth: %w", err)
	}

	resp, err := c.httpClient.Do(req) //nolint:gosec // G704: URL из конфигурации
	if err != nil {
		return fmt.Errorf("запрос CheckHealth к %s: %w", c.adminURL, err)
	}
	defer resp.Body.Close()
	// Вычитываем тело для переиспользования соединения
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("AM health endpoint вернул статус %d", resp.StatusCode)
	}

	return nil
}

// requestToken запрашивает новый SA-токен через client_credentials grant.
// Вызывается под write lock.
func (c *Client) requestToken(ctx context.Context) (string, error) {
	// Token endpoint — напрямую Keycloak (client_credentials grant)
	tokenURL := c.tokenURL

	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("создание запроса token: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req) //nolint:gosec // G704: URL из конфигурации
	if err != nil {
		return "", fmt.Errorf("запрос token к AM: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("AM token endpoint вернул статус %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		Token     string `json:"access_token"` //nolint:gosec // G117: JSON-маппинг OAuth2 ответа
		ExpiresIn int    `json:"expires_in"`
		TokenType string `json:"token_type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("декодирование token response: %w", err)
	}

	if tokenResp.Token == "" {
		return "", fmt.Errorf("пустой access_token в ответе AM")
	}

	// Кэшируем токен (с запасом 30 секунд до истечения)
	c.token = &tokenInfo{
		accessToken: tokenResp.Token,
		expiresAt:   time.Now().Add(time.Duration(tokenResp.ExpiresIn)*time.Second - 30*time.Second),
	}

	c.logger.Debug("SA-токен получен от AM",
		slog.Int("expires_in", tokenResp.ExpiresIn),
	)

	return tokenResp.Token, nil
}

// buildTLSConfig создаёт TLS-конфигурацию с кастомным CA-сертификатом.
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
