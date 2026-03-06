// Пакет adminclient — HTTP-клиент для взаимодействия с Admin Module.
// Получает SA-токен через client_credentials grant и запрашивает информацию о Storage Elements.
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
	// ID — UUID Storage Element
	ID string `json:"id"`
	// Name — человекочитаемое имя SE
	Name string `json:"name"`
	// URL — базовый URL SE (для скачивания файлов)
	URL string `json:"url"`
	// Mode — режим SE (edit, rw, ro, ar)
	Mode string `json:"mode"`
	// Status — статус SE (online, offline, degraded, maintenance)
	Status string `json:"status"`
}

// tokenInfo — закэшированный SA-токен с временем истечения.
type tokenInfo struct {
	accessToken string
	expiresAt   time.Time
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
// timeout — таймаут HTTP-запросов (из конфигурации QM_ADMIN_TIMEOUT).
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

// GetStorageElement запрашивает информацию о Storage Element по ID.
// GET /api/v1/storage-elements/{id}
func (c *Client) GetStorageElement(ctx context.Context, seID string) (*SEInfo, error) {
	reqURL := fmt.Sprintf("%s/api/v1/storage-elements/%s", c.adminURL, seID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("создание запроса GetStorageElement: %w", err)
	}

	// Получаем SA-токен для авторизации
	token, err := c.GetToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("получение токена для AM: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req) //nolint:gosec // G704: URL из конфигурации
	if err != nil {
		return nil, fmt.Errorf("запрос GetStorageElement к %s: %w", c.adminURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AM вернул статус %d для SE %s: %s", resp.StatusCode, seID, string(body))
	}

	var info SEInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("декодирование ответа SE от AM: %w", err)
	}

	return &info, nil
}

// DeleteFile удаляет файл через Admin Module API (hard delete).
// DELETE /api/v1/files/{file_id}
// Используется при lazy cleanup в QM — когда SE возвращает 404.
func (c *Client) DeleteFile(ctx context.Context, fileID string) error {
	reqURL := fmt.Sprintf("%s/api/v1/files/%s", c.adminURL, fileID)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("создание запроса DeleteFile: %w", err)
	}

	// SA-токен для авторизации
	token, err := c.GetToken(ctx)
	if err != nil {
		return fmt.Errorf("получение токена для AM: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req) //nolint:gosec // G704: URL из конфигурации
	if err != nil {
		return fmt.Errorf("запрос DeleteFile к %s: %w", c.adminURL, err)
	}
	defer resp.Body.Close()

	// 204 No Content — успешное удаление
	// 404 — файл уже удалён (не ошибка, идемпотентно)
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		c.logger.Debug("Файл удалён через AM",
			slog.String("file_id", fileID),
			slog.Int("status", resp.StatusCode),
		)
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("AM вернул статус %d для DeleteFile %s: %s", resp.StatusCode, fileID, string(body))
}

// seListResponse — обёртка ответа GET /api/v1/storage-elements.
type seListResponse struct {
	Items []SEInfo `json:"items"`
}

// GetStorageElements запрашивает список Storage Elements с заданным статусом.
// GET /api/v1/storage-elements?status={status}
// Используется для мониторинга топологии (периодический опрос).
func (c *Client) GetStorageElements(ctx context.Context, status string) ([]SEInfo, error) {
	reqURL := fmt.Sprintf("%s/api/v1/storage-elements?status=%s",
		c.adminURL, url.QueryEscape(status))

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
		slog.String("status", status),
		slog.Int("count", len(listResp.Items)),
	)

	return listResp.Items, nil
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
