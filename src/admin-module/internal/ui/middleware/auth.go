// Пакет middleware — HTTP middleware для Admin UI.
// auth.go — проверка UI-сессии (cookie-based), авто-refresh токенов.
package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/arturkryukov/artstore/admin-module/internal/ui/auth"
)

// contextKey — тип для ключей контекста UI (избегаем коллизий с API middleware).
type contextKey string

const (
	// ContextKeyUISession — данные UI-сессии в контексте запроса.
	ContextKeyUISession contextKey = "ui_session"
)

// UIAuth — middleware для проверки аутентификации UI-пользователей.
// Извлекает сессию из зашифрованного cookie, при необходимости обновляет
// access token через Keycloak, redirect на /admin/login при отсутствии сессии.
type UIAuth struct {
	sessionManager *auth.SessionManager
	oidcClient     *auth.OIDCClient
	logger         *slog.Logger
}

// NewUIAuth создаёт новый UIAuth middleware.
func NewUIAuth(
	sessionManager *auth.SessionManager,
	oidcClient *auth.OIDCClient,
	logger *slog.Logger,
) *UIAuth {
	return &UIAuth{
		sessionManager: sessionManager,
		oidcClient:     oidcClient,
		logger:         logger.With(slog.String("component", "ui_auth_middleware")),
	}
}

// Middleware возвращает HTTP middleware для проверки UI-сессии.
// Применяется к маршрутам /admin/*, кроме /admin/login, /admin/callback, /admin/logout.
func (ua *UIAuth) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. Извлекаем сессию из cookie
			session, err := ua.sessionManager.GetSessionFromRequest(r)
			if err != nil {
				ua.logger.Debug("Ошибка чтения UI-сессии",
					slog.String("error", err.Error()),
					slog.String("remote_addr", r.RemoteAddr),
				)
				// Повреждённый cookie — очищаем и redirect на login
				ua.sessionManager.ClearSessionCookie(w)
				http.Redirect(w, r, "/admin/login", http.StatusFound)
				return
			}

			// 2. Если сессия отсутствует — redirect на login
			if session == nil {
				http.Redirect(w, r, "/admin/login", http.StatusFound)
				return
			}

			// 3. Проверяем срок действия access token
			if session.IsExpired() {
				// Пробуем обновить через refresh token
				refreshed, refreshErr := ua.refreshSession(session)
				if refreshErr != nil {
					ua.logger.Info("Не удалось обновить сессию, redirect на login",
						slog.String("username", session.Username),
						slog.String("error", refreshErr.Error()),
					)
					ua.sessionManager.ClearSessionCookie(w)
					http.Redirect(w, r, "/admin/login", http.StatusFound)
					return
				}

				// Обновляем session cookie с новыми токенами
				if err := ua.sessionManager.SetSessionCookie(w, refreshed); err != nil {
					ua.logger.Error("Ошибка обновления session cookie",
						slog.String("error", err.Error()),
					)
					ua.sessionManager.ClearSessionCookie(w)
					http.Redirect(w, r, "/admin/login", http.StatusFound)
					return
				}

				session = refreshed
				ua.logger.Debug("Сессия обновлена через refresh token",
					slog.String("username", session.Username),
				)
			}

			// 4. Помещаем сессию в контекст
			ctx := context.WithValue(r.Context(), ContextKeyUISession, session)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// refreshSession обновляет access token через Keycloak refresh token.
// Возвращает обновлённую SessionData или ошибку.
func (ua *UIAuth) refreshSession(session *auth.SessionData) (*auth.SessionData, error) {
	tokenResp, err := ua.oidcClient.RefreshTokens(session.RefreshToken)
	if err != nil {
		return nil, err
	}

	// Обновляем данные сессии, сохраняя username/role/groups
	return &auth.SessionData{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Unix(),
		Username:     session.Username,
		Email:        session.Email,
		Role:         session.Role,
		Groups:       session.Groups,
	}, nil
}

// SessionFromContext извлекает SessionData из контекста запроса.
// Возвращает nil если сессия не найдена (не прошёл через UIAuth middleware).
func SessionFromContext(ctx context.Context) *auth.SessionData {
	session, ok := ctx.Value(ContextKeyUISession).(*auth.SessionData)
	if !ok {
		return nil
	}
	return session
}
