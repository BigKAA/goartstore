// client.go — HTTP-клиент к Keycloak Admin REST API.
// Реализует автоматическое получение service account token через Client Credentials flow,
// кэширование токена (обновление за 30s до expiration).
// Операции: ListUsers, GetUser, GetUserGroups, ListClients, CreateClient,
// UpdateClient, DeleteClient, GetClientSecret, RegenerateClientSecret, RealmInfo.
package keycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client — HTTP-клиент к Keycloak Admin REST API.
type Client struct {
	baseURL      string // Базовый URL Keycloak (без trailing slash)
	realm        string // Имя realm
	clientID     string // Client ID для Client Credentials flow
	clientSecret string // Client Secret

	httpClient *http.Client
	logger     *slog.Logger

	// Кэш токена доступа
	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

// New создаёт клиент к Keycloak Admin REST API.
// baseURL — базовый URL Keycloak (например, https://keycloak.kryukov.lan).
// realm — имя realm (например, artstore).
// clientID, clientSecret — credentials для Client Credentials flow.
// httpClient — HTTP-клиент (может содержать TLS конфигурацию).
func New(baseURL, realm, clientID, clientSecret string, httpClient *http.Client, logger *slog.Logger) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		realm:        realm,
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   httpClient,
		logger:       logger.With(slog.String("component", "keycloak_client")),
	}
}

// --- Аутентификация ---

// tokenEndpoint возвращает URL endpoint'а получения токена.
func (c *Client) tokenEndpoint() string {
	return fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", c.baseURL, c.realm)
}

// adminBaseURL возвращает базовый URL Admin REST API для realm.
func (c *Client) adminBaseURL() string {
	return fmt.Sprintf("%s/admin/realms/%s", c.baseURL, c.realm)
}

// getToken возвращает актуальный access token, обновляя при необходимости.
// Токен обновляется за 30 секунд до истечения.
func (c *Client) getToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Проверяем кэш: если токен валиден ещё 30 секунд — используем его
	if c.accessToken != "" && time.Now().Add(30*time.Second).Before(c.tokenExpiry) {
		return c.accessToken, nil
	}

	// Запрашиваем новый токен через Client Credentials flow
	token, err := c.requestToken(ctx)
	if err != nil {
		return "", err
	}

	c.accessToken = token.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)

	c.logger.Debug("Keycloak токен обновлён",
		slog.Time("expires_at", c.tokenExpiry),
	)

	return c.accessToken, nil
}

// requestToken выполняет Client Credentials flow.
func (c *Client) requestToken(ctx context.Context) (*TokenResponse, error) {
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenEndpoint(), strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("создание запроса токена: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("запрос токена Keycloak: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Keycloak вернул статус %d при запросе токена: %s", resp.StatusCode, string(body))
	}

	var token TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, fmt.Errorf("декодирование токена Keycloak: %w", err)
	}

	return &token, nil
}

// --- HTTP helpers ---

// doAuthorized выполняет HTTP-запрос к Admin REST API с авторизацией.
func (c *Client) doAuthorized(ctx context.Context, method, path string, body any) (*http.Response, error) {
	token, err := c.getToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("получение токена: %w", err)
	}

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("сериализация тела запроса: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	reqURL := c.adminBaseURL() + path
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("создание запроса: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}

// decodeResponse декодирует JSON ответ в target.
func decodeResponse(resp *http.Response, target any) error {
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Keycloak API вернул статус %d: %s", resp.StatusCode, string(body))
	}

	if target != nil {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			return fmt.Errorf("декодирование ответа Keycloak: %w", err)
		}
	}

	return nil
}

// checkResponse проверяет статус ответа (для запросов без тела ответа).
func checkResponse(resp *http.Response, expectedStatus int) error {
	defer resp.Body.Close()

	if resp.StatusCode != expectedStatus {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Keycloak API вернул статус %d (ожидался %d): %s",
			resp.StatusCode, expectedStatus, string(body))
	}

	return nil
}

// --- Users API ---

// ListUsers возвращает пользователей realm с фильтрацией по поисковому запросу.
// query — строка поиска (по username, email, firstName, lastName).
// Если query пустой — возвращает всех.
func (c *Client) ListUsers(ctx context.Context, query string, first, max int) ([]KeycloakUser, error) {
	path := fmt.Sprintf("/users?first=%d&max=%d", first, max)
	if query != "" {
		path += "&search=" + url.QueryEscape(query)
	}

	resp, err := c.doAuthorized(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var users []KeycloakUser
	if err := decodeResponse(resp, &users); err != nil {
		return nil, fmt.Errorf("ListUsers: %w", err)
	}

	return users, nil
}

// CountUsers возвращает количество пользователей в realm.
func (c *Client) CountUsers(ctx context.Context) (int, error) {
	resp, err := c.doAuthorized(ctx, http.MethodGet, "/users/count", nil)
	if err != nil {
		return 0, err
	}

	var count int
	if err := decodeResponse(resp, &count); err != nil {
		return 0, fmt.Errorf("CountUsers: %w", err)
	}

	return count, nil
}

// GetUser возвращает пользователя по Keycloak ID.
func (c *Client) GetUser(ctx context.Context, id string) (*KeycloakUser, error) {
	resp, err := c.doAuthorized(ctx, http.MethodGet, "/users/"+id, nil)
	if err != nil {
		return nil, err
	}

	var user KeycloakUser
	if err := decodeResponse(resp, &user); err != nil {
		return nil, fmt.Errorf("GetUser: %w", err)
	}

	return &user, nil
}

// GetUserGroups возвращает группы пользователя.
func (c *Client) GetUserGroups(ctx context.Context, userID string) ([]KeycloakGroup, error) {
	resp, err := c.doAuthorized(ctx, http.MethodGet, "/users/"+userID+"/groups", nil)
	if err != nil {
		return nil, err
	}

	var groups []KeycloakGroup
	if err := decodeResponse(resp, &groups); err != nil {
		return nil, fmt.Errorf("GetUserGroups: %w", err)
	}

	return groups, nil
}

// --- Clients API (для Service Accounts) ---

// ListClients возвращает клиентов realm с фильтрацией по clientId prefix.
// clientIDFilter — фильтр по clientId (Keycloak использует startsWith).
func (c *Client) ListClients(ctx context.Context, clientIDFilter string, first, max int) ([]KeycloakClient, error) {
	path := fmt.Sprintf("/clients?first=%d&max=%d", first, max)
	if clientIDFilter != "" {
		path += "&clientId=" + url.QueryEscape(clientIDFilter)
	}

	resp, err := c.doAuthorized(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var clients []KeycloakClient
	if err := decodeResponse(resp, &clients); err != nil {
		return nil, fmt.Errorf("ListClients: %w", err)
	}

	return clients, nil
}

// GetClient возвращает клиента по Keycloak internal ID.
func (c *Client) GetClient(ctx context.Context, id string) (*KeycloakClient, error) {
	resp, err := c.doAuthorized(ctx, http.MethodGet, "/clients/"+id, nil)
	if err != nil {
		return nil, err
	}

	var client KeycloakClient
	if err := decodeResponse(resp, &client); err != nil {
		return nil, fmt.Errorf("GetClient: %w", err)
	}

	return &client, nil
}

// CreateClient создаёт клиент в Keycloak (для Service Account — Client Credentials grant).
// Возвращает Keycloak internal ID созданного клиента.
func (c *Client) CreateClient(ctx context.Context, clientID, name, description string, scopes []string) (string, error) {
	createReq := clientCreateRequest{
		ClientID:                  clientID,
		Name:                      name,
		Description:               description,
		Enabled:                   true,
		ServiceAccountsEnabled:    true,
		ClientAuthenticatorType:   "client-secret",
		DirectAccessGrantsEnabled: false,
		StandardFlowEnabled:       false,
		PublicClient:              false,
		DefaultClientScopes:       scopes,
		Attributes: map[string]string{
			"managed_by": "admin-module",
		},
	}

	resp, err := c.doAuthorized(ctx, http.MethodPost, "/clients", createReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("CreateClient: Keycloak вернул статус %d: %s", resp.StatusCode, string(body))
	}

	// Keycloak возвращает Location header с ID созданного ресурса
	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("CreateClient: отсутствует Location header в ответе")
	}

	// Извлекаем ID из Location: .../clients/{id}
	parts := strings.Split(location, "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("CreateClient: не удалось извлечь ID из Location: %s", location)
	}

	return parts[len(parts)-1], nil
}

// UpdateClient обновляет клиента в Keycloak.
func (c *Client) UpdateClient(ctx context.Context, id string, client *KeycloakClient) error {
	resp, err := c.doAuthorized(ctx, http.MethodPut, "/clients/"+id, client)
	if err != nil {
		return err
	}

	return checkResponse(resp, http.StatusNoContent)
}

// DeleteClient удаляет клиента в Keycloak.
func (c *Client) DeleteClient(ctx context.Context, id string) error {
	resp, err := c.doAuthorized(ctx, http.MethodDelete, "/clients/"+id, nil)
	if err != nil {
		return err
	}

	return checkResponse(resp, http.StatusNoContent)
}

// GetClientSecret возвращает текущий секрет клиента.
func (c *Client) GetClientSecret(ctx context.Context, id string) (string, error) {
	resp, err := c.doAuthorized(ctx, http.MethodGet, "/clients/"+id+"/client-secret", nil)
	if err != nil {
		return "", err
	}

	var secret KeycloakClientSecret
	if err := decodeResponse(resp, &secret); err != nil {
		return "", fmt.Errorf("GetClientSecret: %w", err)
	}

	return secret.Value, nil
}

// RegenerateClientSecret генерирует новый секрет клиента.
func (c *Client) RegenerateClientSecret(ctx context.Context, id string) (string, error) {
	resp, err := c.doAuthorized(ctx, http.MethodPost, "/clients/"+id+"/client-secret", nil)
	if err != nil {
		return "", err
	}

	var secret KeycloakClientSecret
	if err := decodeResponse(resp, &secret); err != nil {
		return "", fmt.Errorf("RegenerateClientSecret: %w", err)
	}

	return secret.Value, nil
}

// --- Realm API ---

// RealmInfo возвращает информацию о realm.
func (c *Client) RealmInfo(ctx context.Context) (*RealmRepresentation, error) {
	resp, err := c.doAuthorized(ctx, http.MethodGet, "", nil)
	if err != nil {
		return nil, err
	}

	var realm RealmRepresentation
	if err := decodeResponse(resp, &realm); err != nil {
		return nil, fmt.Errorf("RealmInfo: %w", err)
	}

	return &realm, nil
}

// --- Readiness checker ---

// CheckReady проверяет доступность Keycloak через realm info.
// Реализует handlers.ReadinessChecker.
func (c *Client) CheckReady() (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	realm, err := c.RealmInfo(ctx)
	if err != nil {
		return "fail", fmt.Sprintf("Keycloak недоступен: %v", err)
	}

	if !realm.Enabled {
		return "degraded", fmt.Sprintf("Realm %s отключён", realm.Realm)
	}

	return "ok", fmt.Sprintf("Realm %s доступен", realm.Realm)
}

// TokenProvider возвращает функцию, которая предоставляет access token.
// Используется SE-клиентом для авторизации запросов к Storage Elements.
func (c *Client) TokenProvider() func(ctx context.Context) (string, error) {
	return c.getToken
}
