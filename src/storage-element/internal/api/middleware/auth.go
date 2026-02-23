// auth.go — JWT middleware для аутентификации и авторизации.
// Использует RS256 + JWKS для валидации токенов от Admin Module.
// Claims: sub (subject), scopes (массив строк).
// Публичные endpoints (health, info, metrics) — без аутентификации.
package middleware

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/MicahParks/jwkset"
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
// Поддерживает два формата scopes:
//   - Keycloak стандартный: "scope" (пробело-разделённая строка)
//   - Кастомный: "scopes" (массив строк)
type Claims struct {
	jwt.RegisteredClaims
	// ScopeString — стандартный OAuth2 claim (пробело-разделённая строка)
	ScopeString string `json:"scope"`
	// ScopeArray — кастомный claim (массив строк), альтернативный формат
	ScopeArray []string `json:"scopes"`
}

// Scopes возвращает объединённый список scope'ов из обоих форматов.
func (c *Claims) Scopes() []string {
	var result []string
	if c.ScopeString != "" {
		result = append(result, strings.Split(c.ScopeString, " ")...)
	}
	result = append(result, c.ScopeArray...)
	return result
}

// JWTAuth — middleware для JWT-аутентификации через JWKS.
type JWTAuth struct {
	jwks   keyfunc.Keyfunc
	logger *slog.Logger
}

// NewJWTAuth создаёт JWT middleware с JWKS из указанного URL.
// jwksURL — URL к JWKS endpoint Admin Module (например, https://admin:8000/api/v1/auth/jwks).
// caCertPath — опциональный путь к CA-сертификату для проверки TLS JWKS endpoint.
// Автоматически обновляет ключи в фоне с интервалом 15 секунд.
func NewJWTAuth(jwksURL string, caCertPath string, logger *slog.Logger) (*JWTAuth, error) {
	// Создаём HTTP-клиент (с кастомным CA или стандартный)
	httpClient := http.DefaultClient
	if caCertPath != "" {
		var err error
		httpClient, err = httpClientWithCA(caCertPath)
		if err != nil {
			return nil, fmt.Errorf("загрузка CA-сертификата %s: %w", caCertPath, err)
		}
		logger.Info("CA-сертификат добавлен в пул доверия",
			slog.String("ca_cert", caCertPath),
		)
	}

	// Создаём JWKS Storage с кастомным HTTP-клиентом и коротким RefreshInterval.
	// NoErrorReturnFirstHTTPReq позволяет стартовать даже если JWKS endpoint
	// ещё недоступен (например, при одновременном запуске pod-ов).
	storage, err := jwkset.NewStorageFromHTTP(jwksURL, jwkset.HTTPClientStorageOptions{
		Client:                    httpClient,
		NoErrorReturnFirstHTTPReq: true,
		RefreshInterval:           15 * time.Second,
		RefreshErrorHandler: func(ctx context.Context, err error) {
			logger.Error("Ошибка обновления JWKS",
				slog.String("error", err.Error()),
				slog.String("url", jwksURL),
			)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("создание JWKS storage: %w", err)
	}

	k, err := keyfunc.New(keyfunc.Options{
		Storage: storage,
	})
	if err != nil {
		return nil, fmt.Errorf("создание keyfunc: %w", err)
	}

	return &JWTAuth{
		jwks:   k,
		logger: logger.With(slog.String("component", "jwt_auth")),
	}, nil
}

// httpClientWithCA создаёт HTTP-клиент, доверяющий указанному CA-сертификату.
func httpClientWithCA(caCertPath string) (*http.Client, error) {
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, err
	}

	caCertPool, err := x509.SystemCertPool()
	if err != nil {
		caCertPool = x509.NewCertPool()
	}
	caCertPool.AppendCertsFromPEM(caCert)

	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
			},
		},
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
			ctx = context.WithValue(ctx, ContextKeyScopes, claims.Scopes())

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
