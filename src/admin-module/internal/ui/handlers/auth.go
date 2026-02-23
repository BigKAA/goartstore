// Пакет handlers — HTTP-обработчики Admin UI.
// auth.go — аутентификация через Keycloak OIDC (Authorization Code + PKCE).
package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/arturkryukov/artstore/admin-module/internal/domain/rbac"
	"github.com/arturkryukov/artstore/admin-module/internal/ui/auth"
)

// Имя cookie для хранения PKCE state (code_verifier + state).
const stateCookieName = "artstore_auth_state"

// stateCookieMaxAge — максимальный возраст state cookie (5 минут).
const stateCookieMaxAge = 5 * 60

// AuthHandler — обработчики аутентификации Admin UI.
type AuthHandler struct {
	oidcClient     *auth.OIDCClient
	sessionManager *auth.SessionManager
	logger         *slog.Logger
	// adminGroups — группы Keycloak, дающие роль admin.
	adminGroups []string
	// readonlyGroups — группы Keycloak, дающие роль readonly.
	readonlyGroups []string
	// secureCookie — использовать Secure flag для state cookie.
	secureCookie bool
}

// NewAuthHandler создаёт новый AuthHandler.
func NewAuthHandler(
	oidcClient *auth.OIDCClient,
	sessionManager *auth.SessionManager,
	adminGroups, readonlyGroups []string,
	secureCookie bool,
	logger *slog.Logger,
) *AuthHandler {
	return &AuthHandler{
		oidcClient:     oidcClient,
		sessionManager: sessionManager,
		logger:         logger.With(slog.String("component", "ui_auth")),
		adminGroups:    adminGroups,
		readonlyGroups: readonlyGroups,
		secureCookie:   secureCookie,
	}
}

// stateData — данные, сохраняемые в state cookie на время auth flow.
type stateData struct {
	// State — CSRF state parameter.
	State string `json:"state"`
	// CodeVerifier — PKCE code_verifier для обмена code → tokens.
	CodeVerifier string `json:"code_verifier"`
}

// HandleLogin — GET /admin/login
// Генерирует PKCE и state, сохраняет в short-lived cookie,
// redirect на Keycloak authorize endpoint.
func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	// Генерируем PKCE
	pkce, err := auth.GeneratePKCE()
	if err != nil {
		h.logger.Error("Ошибка генерации PKCE", slog.String("error", err.Error()))
		http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
		return
	}

	// Генерируем state (CSRF-защита)
	state, err := auth.GenerateState()
	if err != nil {
		h.logger.Error("Ошибка генерации state", slog.String("error", err.Error()))
		http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
		return
	}

	// Сохраняем state + code_verifier в short-lived cookie
	sd := &stateData{
		State:        state,
		CodeVerifier: pkce.CodeVerifier,
	}
	sdJSON, _ := json.Marshal(sd)
	sdEncoded := base64.URLEncoding.EncodeToString(sdJSON)

	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    sdEncoded,
		Path:     "/admin",
		MaxAge:   stateCookieMaxAge,
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteLaxMode,
	})

	// Формируем redirect URI на основе текущего запроса
	redirectURI := h.buildRedirectURI(r)

	// Redirect на Keycloak authorize endpoint
	authorizeURL := h.oidcClient.AuthorizeURL(redirectURI, state, pkce.CodeChallenge)

	h.logger.Debug("Redirect на Keycloak login",
		slog.String("authorize_url", authorizeURL),
	)

	http.Redirect(w, r, authorizeURL, http.StatusFound)
}

// HandleCallback — GET /admin/callback
// Обменивает authorization code на tokens, создаёт session cookie,
// redirect на /admin/.
func (h *AuthHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	// 1. Проверяем ошибку от Keycloak
	if errCode := r.URL.Query().Get("error"); errCode != "" {
		errDesc := r.URL.Query().Get("error_description")
		h.logger.Warn("Keycloak вернул ошибку авторизации",
			slog.String("error", errCode),
			slog.String("description", errDesc),
		)
		http.Error(w, fmt.Sprintf("Ошибка авторизации: %s — %s", errCode, errDesc), http.StatusBadRequest)
		return
	}

	// 2. Извлекаем authorization code и state
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		http.Error(w, "Отсутствует code или state", http.StatusBadRequest)
		return
	}

	// 3. Извлекаем и валидируем state cookie
	stateCookie, err := r.Cookie(stateCookieName)
	if err != nil {
		h.logger.Warn("State cookie отсутствует", slog.String("error", err.Error()))
		http.Error(w, "Сессия авторизации истекла, попробуйте ещё раз", http.StatusBadRequest)
		return
	}

	sdJSON, err := base64.URLEncoding.DecodeString(stateCookie.Value)
	if err != nil {
		h.logger.Warn("Ошибка декодирования state cookie", slog.String("error", err.Error()))
		http.Error(w, "Некорректный state cookie", http.StatusBadRequest)
		return
	}

	var sd stateData
	if err := json.Unmarshal(sdJSON, &sd); err != nil {
		h.logger.Warn("Ошибка парсинга state cookie", slog.String("error", err.Error()))
		http.Error(w, "Некорректный state cookie", http.StatusBadRequest)
		return
	}

	// 4. Валидируем state (CSRF-защита)
	if sd.State != state {
		h.logger.Warn("State mismatch (возможная CSRF атака)",
			slog.String("expected", sd.State),
			slog.String("received", state),
		)
		http.Error(w, "State mismatch", http.StatusBadRequest)
		return
	}

	// 5. Удаляем state cookie (одноразовый)
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    "",
		Path:     "/admin",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteLaxMode,
	})

	// 6. Обмениваем code на tokens
	redirectURI := h.buildRedirectURI(r)
	tokenResp, err := h.oidcClient.ExchangeCode(code, redirectURI, sd.CodeVerifier)
	if err != nil {
		h.logger.Error("Ошибка обмена code на tokens",
			slog.String("error", err.Error()),
		)
		http.Error(w, "Ошибка аутентификации", http.StatusInternalServerError)
		return
	}

	// 7. Извлекаем данные пользователя из access token (JWT payload)
	sessionData, err := h.buildSessionFromToken(tokenResp)
	if err != nil {
		h.logger.Error("Ошибка извлечения данных из токена",
			slog.String("error", err.Error()),
		)
		http.Error(w, "Ошибка обработки токена", http.StatusInternalServerError)
		return
	}

	// 8. Устанавливаем session cookie
	if err := h.sessionManager.SetSessionCookie(w, sessionData); err != nil {
		h.logger.Error("Ошибка установки session cookie",
			slog.String("error", err.Error()),
		)
		http.Error(w, "Ошибка создания сессии", http.StatusInternalServerError)
		return
	}

	h.logger.Info("Пользователь аутентифицирован",
		slog.String("username", sessionData.Username),
		slog.String("role", sessionData.Role),
	)

	// 9. Redirect на /admin/
	http.Redirect(w, r, "/admin/", http.StatusFound)
}

// HandleLogout — POST /admin/logout
// Очищает session cookie, redirect на Keycloak logout endpoint.
func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	// Очищаем session cookie
	h.sessionManager.ClearSessionCookie(w)

	// Формируем URL для redirect после logout
	postLogoutRedirectURI := h.buildBaseURL(r) + "/admin/login"

	// Redirect на Keycloak logout
	logoutURL := h.oidcClient.LogoutURL("", postLogoutRedirectURI)

	h.logger.Info("Пользователь выполняет logout")

	http.Redirect(w, r, logoutURL, http.StatusFound)
}

// buildRedirectURI формирует callback redirect URI на основе текущего запроса.
func (h *AuthHandler) buildRedirectURI(r *http.Request) string {
	return h.buildBaseURL(r) + "/admin/callback"
}

// buildBaseURL формирует базовый URL (scheme + host) из заголовков запроса.
// Учитывает X-Forwarded-* заголовки от reverse proxy / API Gateway.
func (h *AuthHandler) buildBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	// Проверяем X-Forwarded-Proto (от API Gateway / reverse proxy)
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}

	host := r.Host
	// Проверяем X-Forwarded-Host
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	}

	return scheme + "://" + host
}

// buildSessionFromToken извлекает данные пользователя из JWT access token.
// JWT payload парсится без валидации подписи (доверяем Keycloak на этапе callback).
func (h *AuthHandler) buildSessionFromToken(tokenResp *auth.TokenResponse) (*auth.SessionData, error) {
	// Парсим JWT payload (второй сегмент, base64url-encoded)
	parts := strings.SplitN(tokenResp.AccessToken, ".", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("некорректный формат JWT: ожидалось 3 сегмента")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("ошибка декодирования JWT payload: %w", err)
	}

	var claims jwtClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("ошибка парсинга JWT claims: %w", err)
	}

	// Маппинг групп → роль
	role := rbac.MapGroupsToRole(claims.Groups, h.adminGroups, h.readonlyGroups)

	// Если роль не определена через группы — пробуем через realm_access.roles
	if role == "" && claims.RealmAccess != nil {
		for _, r := range claims.RealmAccess.Roles {
			if rbac.IsValidRole(r) {
				candidate := rbac.HighestRole([]string{role, r})
				role = candidate
			}
		}
	}

	return &auth.SessionData{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Unix(),
		Username:     claims.PreferredUsername,
		Email:        claims.Email,
		Role:         role,
		Groups:       claims.Groups,
	}, nil
}

// jwtClaims — минимальная структура JWT claims для извлечения данных пользователя.
type jwtClaims struct {
	Sub               string       `json:"sub"`
	PreferredUsername  string       `json:"preferred_username"`
	Email             string       `json:"email"`
	Groups            []string     `json:"groups"`
	RealmAccess       *realmAccess `json:"realm_access"`
}

// realmAccess — блок realm_access из Keycloak JWT.
type realmAccess struct {
	Roles []string `json:"roles"`
}
