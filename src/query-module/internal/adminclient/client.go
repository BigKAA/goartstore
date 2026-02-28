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
	clientID     string
	clientSecret string //nolint:gosec // G101: поле структуры, не содержит секрет напрямую
	logger       *slog.Logger

	// Кэш SA-токена (thread-safe)
	mu    sync.RWMutex
	token *tokenInfo
}

// New создаёт Admin Module клиент.
// adminURL — базовый URL Admin Module (например, http://admin-module:8000).
// caCertPath — путь к CA-сертификату для TLS (пустая строка — стандартный пул).
// timeout — таймаут HTTP-запросов (из конфигурации QM_ADMIN_TIMEOUT).
func New(
	adminURL string,
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

// requestToken запрашивает новый SA-токен через client_credentials grant.
// Вызывается под write lock.
func (c *Client) requestToken(ctx context.Context) (string, error) {
	// Token endpoint — через AM proxy /auth/token
	tokenURL := c.adminURL + "/auth/token"

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
