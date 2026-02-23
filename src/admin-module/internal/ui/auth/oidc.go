// oidc.go — OIDC-клиент для аутентификации Admin UI через Keycloak.
// Реализует Authorization Code Flow с PKCE (RFC 7636).
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OIDCClient — клиент для взаимодействия с Keycloak OIDC endpoints.
// Public client (без client_secret), использует PKCE для защиты.
type OIDCClient struct {
	// clientID — OIDC Client ID (по умолчанию "artstore-admin-ui").
	clientID string
	// authorizeURL — endpoint авторизации Keycloak.
	authorizeURL string
	// tokenURL — endpoint обмена code → tokens.
	tokenURL string
	// logoutURL — endpoint logout Keycloak.
	logoutURL string
	// issuer — issuer URL для валидации (realm URL).
	issuer string
	// httpClient — HTTP-клиент (с кастомным CA при необходимости).
	httpClient *http.Client
}

// OIDCConfig — конфигурация OIDC-клиента.
type OIDCConfig struct {
	// KeycloakURL — базовый URL Keycloak (без trailing slash).
	KeycloakURL string
	// Realm — имя realm в Keycloak.
	Realm string
	// ClientID — OIDC Client ID (public client).
	ClientID string
	// HTTPClient — HTTP-клиент (nil = http.DefaultClient).
	HTTPClient *http.Client
}

// NewOIDCClient создаёт новый OIDC-клиент на основе конфигурации.
func NewOIDCClient(cfg OIDCConfig) *OIDCClient {
	realmURL := fmt.Sprintf("%s/realms/%s", cfg.KeycloakURL, cfg.Realm)
	oidcBase := realmURL + "/protocol/openid-connect"

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &OIDCClient{
		clientID:     cfg.ClientID,
		authorizeURL: oidcBase + "/auth",
		tokenURL:     oidcBase + "/token",
		logoutURL:    oidcBase + "/logout",
		issuer:       realmURL,
		httpClient:   httpClient,
	}
}

// PKCEParams — параметры PKCE для одного auth flow.
type PKCEParams struct {
	// CodeVerifier — случайная строка для PKCE (хранится в state cookie).
	CodeVerifier string
	// CodeChallenge — SHA-256 хеш code_verifier (отправляется в authorize URL).
	CodeChallenge string
}

// GeneratePKCE генерирует пару code_verifier / code_challenge (S256).
// code_verifier: 43-128 символов, base64url(random bytes).
// code_challenge: base64url(SHA-256(code_verifier)).
func GeneratePKCE() (*PKCEParams, error) {
	// 32 bytes → 43 символа base64url (без padding)
	verifierBytes := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, verifierBytes); err != nil {
		return nil, fmt.Errorf("ошибка генерации code_verifier: %w", err)
	}
	codeVerifier := base64.RawURLEncoding.EncodeToString(verifierBytes)

	// code_challenge = base64url(SHA-256(code_verifier))
	hash := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(hash[:])

	return &PKCEParams{
		CodeVerifier:  codeVerifier,
		CodeChallenge: codeChallenge,
	}, nil
}

// AuthorizeURL формирует URL для redirect пользователя на Keycloak login.
// redirectURI — URL callback (например, http://localhost:8000/admin/callback).
// state — случайный state parameter для CSRF-защиты.
// codeChallenge — PKCE code_challenge (S256).
func (c *OIDCClient) AuthorizeURL(redirectURI, state, codeChallenge string) string {
	params := url.Values{
		"client_id":             {c.clientID},
		"response_type":        {"code"},
		"redirect_uri":         {redirectURI},
		"state":                {state},
		"scope":                {"openid profile email groups"},
		"code_challenge":       {codeChallenge},
		"code_challenge_method": {"S256"},
	}
	return c.authorizeURL + "?" + params.Encode()
}

// TokenResponse — ответ от token endpoint Keycloak.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token"`
}

// TokenError — ошибка от token endpoint Keycloak.
type TokenError struct {
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

// ExchangeCode обменивает authorization code на tokens через token endpoint.
// code — authorization code от Keycloak callback.
// redirectURI — тот же redirect URI, что использовался в authorize URL.
// codeVerifier — PKCE code_verifier (из state cookie).
func (c *OIDCClient) ExchangeCode(code, redirectURI, codeVerifier string) (*TokenResponse, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {c.clientID},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {codeVerifier},
	}

	return c.doTokenRequest(data)
}

// RefreshTokens обновляет access token через refresh token.
// Возвращает новую пару access_token + refresh_token.
func (c *OIDCClient) RefreshTokens(refreshToken string) (*TokenResponse, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {c.clientID},
		"refresh_token": {refreshToken},
	}

	return c.doTokenRequest(data)
}

// LogoutURL формирует URL для redirect пользователя на Keycloak logout.
// idTokenHint — id_token для корректного logout (опционально).
// postLogoutRedirectURI — URL для redirect после logout.
func (c *OIDCClient) LogoutURL(idTokenHint, postLogoutRedirectURI string) string {
	params := url.Values{
		"client_id":                {c.clientID},
		"post_logout_redirect_uri": {postLogoutRedirectURI},
	}
	if idTokenHint != "" {
		params.Set("id_token_hint", idTokenHint)
	}
	return c.logoutURL + "?" + params.Encode()
}

// GenerateState генерирует случайный state parameter для CSRF-защиты.
func GenerateState() (string, error) {
	stateBytes := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, stateBytes); err != nil {
		return "", fmt.Errorf("ошибка генерации state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(stateBytes), nil
}

// doTokenRequest выполняет POST-запрос к token endpoint Keycloak.
func (c *OIDCClient) doTokenRequest(data url.Values) (*TokenResponse, error) {
	req, err := http.NewRequest(http.MethodPost, c.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса к token endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var tokenErr TokenError
		if jsonErr := json.Unmarshal(body, &tokenErr); jsonErr == nil {
			return nil, fmt.Errorf("token endpoint error: %s — %s", tokenErr.Error, tokenErr.Description)
		}
		return nil, fmt.Errorf("token endpoint вернул статус %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("ошибка парсинга token response: %w", err)
	}

	return &tokenResp, nil
}
