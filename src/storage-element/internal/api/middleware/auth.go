// auth.go — JWT middleware для аутентификации и авторизации.
// Использует RS256 + JWKS для валидации токенов от Admin Module.
// Claims: sub (subject), scopes (массив строк).
// Публичные endpoints (health, info, metrics) — без аутентификации.
package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"

	apierrors "github.com/arturkryukov/artsore/storage-element/internal/api/errors"
)

// contextKey — тип для ключей контекста (избегаем коллизий).
type contextKey string

const (
	// ContextKeySubject — ключ для sub из JWT в контексте запроса.
	ContextKeySubject contextKey = "jwt_subject"
	// ContextKeyScopes — ключ для scopes из JWT в контексте запроса.
	ContextKeyScopes contextKey = "jwt_scopes"
)

// Claims — структура JWT claims для Storage Element.
type Claims struct {
	jwt.RegisteredClaims
	// Scopes — массив scope'ов (например, ["files:read", "files:write"])
	Scopes []string `json:"scopes"`
}

// JWTAuth — middleware для JWT-аутентификации через JWKS.
type JWTAuth struct {
	jwks   keyfunc.Keyfunc
	logger *slog.Logger
}

// NewJWTAuth создаёт JWT middleware с JWKS из указанного URL.
// jwksURL — URL к JWKS endpoint Admin Module (например, https://admin:8000/api/v1/auth/jwks).
// Автоматически обновляет ключи в фоне.
func NewJWTAuth(jwksURL string, logger *slog.Logger) (*JWTAuth, error) {
	// Создаём JWKS keyfunc с автообновлением
	k, err := keyfunc.NewDefault([]string{jwksURL})
	if err != nil {
		return nil, err
	}

	return &JWTAuth{
		jwks:   k,
		logger: logger.With(slog.String("component", "jwt_auth")),
	}, nil
}

// NewJWTAuthWithKeyfunc создаёт JWT middleware с предоставленной keyfunc.
// Используется в тестах для подстановки mock JWKS.
func NewJWTAuthWithKeyfunc(kf keyfunc.Keyfunc, logger *slog.Logger) *JWTAuth {
	return &JWTAuth{
		jwks:   kf,
		logger: logger.With(slog.String("component", "jwt_auth")),
	}
}

// Middleware возвращает HTTP middleware для JWT-аутентификации.
// Извлекает Bearer token из заголовка Authorization, валидирует подпись (RS256),
// проверяет exp/nbf, помещает sub и scopes в контекст запроса.
func (j *JWTAuth) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Извлекаем Bearer token
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				apierrors.Unauthorized(w, "Отсутствует заголовок Authorization")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				apierrors.Unauthorized(w, "Неверный формат Authorization: ожидается Bearer <token>")
				return
			}

			tokenString := parts[1]
			if tokenString == "" {
				apierrors.Unauthorized(w, "Пустой Bearer token")
				return
			}

			// Парсинг и валидация JWT
			claims := &Claims{}
			token, err := jwt.ParseWithClaims(tokenString, claims, j.jwks.KeyfuncCtx(r.Context()),
				jwt.WithValidMethods([]string{"RS256"}),
				jwt.WithExpirationRequired(),
				jwt.WithLeeway(5*time.Second),
			)
			if err != nil {
				j.logger.Debug("JWT валидация не пройдена",
					slog.String("error", err.Error()),
					slog.String("remote_addr", r.RemoteAddr),
				)
				apierrors.Unauthorized(w, "Невалидный или просроченный токен")
				return
			}

			if !token.Valid {
				apierrors.Unauthorized(w, "Невалидный токен")
				return
			}

			// Извлекаем sub
			subject, err := claims.GetSubject()
			if err != nil || subject == "" {
				apierrors.Unauthorized(w, "Отсутствует sub в токене")
				return
			}

			// Помещаем claims в контекст
			ctx := context.WithValue(r.Context(), ContextKeySubject, subject)
			ctx = context.WithValue(ctx, ContextKeyScopes, claims.Scopes)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireScope возвращает middleware, проверяющий наличие указанного scope.
// Если scope отсутствует — возвращает 403 Forbidden.
// Должен использоваться ПОСЛЕ JWTAuth.Middleware().
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scopes, ok := r.Context().Value(ContextKeyScopes).([]string)
			if !ok {
				apierrors.Forbidden(w, "Отсутствуют scopes в токене")
				return
			}

			for _, s := range scopes {
				if s == scope {
					next.ServeHTTP(w, r)
					return
				}
			}

			apierrors.Forbidden(w, "Недостаточно прав: требуется scope "+scope)
		})
	}
}

// SubjectFromContext извлекает sub из контекста запроса.
// Возвращает пустую строку, если sub не найден.
func SubjectFromContext(ctx context.Context) string {
	subject, _ := ctx.Value(ContextKeySubject).(string)
	return subject
}

// ScopesFromContext извлекает scopes из контекста запроса.
// Возвращает nil, если scopes не найдены.
func ScopesFromContext(ctx context.Context) []string {
	scopes, _ := ctx.Value(ContextKeyScopes).([]string)
	return scopes
}

// Close освобождает ресурсы JWKS (останавливает фоновое обновление).
func (j *JWTAuth) Close() {
	// keyfunc v3 не требует явного закрытия для NewDefault
}
