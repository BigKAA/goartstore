// auth.go — JWT middleware для аутентификации и авторизации Admin Module.
// Извлекает claims из Keycloak JWT, определяет тип субъекта (Admin User / Service Account),
// маппит группы в роли, применяет role overrides из БД.
// Fallback-валидация подписи через JWKS Keycloak (основная — на API Gateway).
package middleware

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"

	apierrors "github.com/bigkaa/goartstore/admin-module/internal/api/errors"
	"github.com/bigkaa/goartstore/admin-module/internal/domain/rbac"
)

// contextKey — тип для ключей контекста (избегаем коллизий).
type contextKey string

const (
	// ContextKeyClaims — полные извлечённые claims в контексте запроса.
	ContextKeyClaims contextKey = "jwt_claims"
)

// SubjectType — тип субъекта JWT.
type SubjectType string

const (
	// SubjectTypeUser — Admin User (аутентифицирован через OIDC).
	SubjectTypeUser SubjectType = "user"
	// SubjectTypeSA — Service Account (аутентифицирован через Client Credentials).
	SubjectTypeSA SubjectType = "service_account"
)

// AuthClaims — извлечённые и обработанные claims из Keycloak JWT.
// Помещаются в контекст запроса для downstream handlers.
type AuthClaims struct {
	// Subject — sub из JWT (Keycloak user ID или SA client UUID).
	Subject string
	// SubjectType — тип субъекта (user или service_account).
	SubjectType SubjectType
	// PreferredUsername — preferred_username из JWT.
	PreferredUsername string
	// Email — email из JWT.
	Email string

	// --- Для Admin User ---

	// Roles — роли из realm_access.roles.
	Roles []string
	// Groups — группы из JWT.
	Groups []string
	// IdpRole — роль, вычисленная из групп IdP (admin, readonly, "").
	IdpRole string
	// RoleOverride — локальное дополнение роли из БД (может быть nil).
	RoleOverride *string
	// EffectiveRole — итоговая роль = max(IdpRole, RoleOverride).
	EffectiveRole string

	// --- Для Service Account ---

	// Scopes — scopes из claim "scope" (space-separated в JWT).
	Scopes []string
	// ClientID — client_id из JWT (для Service Account).
	ClientID string
}

// HasRole проверяет, есть ли у субъекта указанная роль (effective).
func (c *AuthClaims) HasRole(role string) bool {
	return c.EffectiveRole == role
}

// HasAnyRole проверяет, совпадает ли effective роль с одной из указанных.
func (c *AuthClaims) HasAnyRole(roles ...string) bool {
	for _, r := range roles {
		if c.EffectiveRole == r {
			return true
		}
	}
	return false
}

// HasScope проверяет наличие указанного scope.
func (c *AuthClaims) HasScope(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// HasAnyScope проверяет наличие хотя бы одного из указанных scopes.
func (c *AuthClaims) HasAnyScope(scopes ...string) bool {
	for _, scope := range scopes {
		if c.HasScope(scope) {
			return true
		}
	}
	return false
}

// RoleOverrideProvider — интерфейс для получения role override из БД.
// Реализуется repository.RoleOverrideRepository.
type RoleOverrideProvider interface {
	// GetRoleOverride возвращает дополнительную роль для пользователя.
	// Если override не найден — возвращает nil, nil.
	GetRoleOverride(ctx context.Context, keycloakUserID string) (*string, error)
}

// keycloakClaims — raw claims из Keycloak JWT для парсинга.
type keycloakClaims struct {
	jwt.RegisteredClaims
	// PreferredUsername — имя пользователя.
	PreferredUsername string `json:"preferred_username"`
	// Email — электронная почта.
	Email string `json:"email"`
	// RealmAccess — вложенная структура для realm_access.roles.
	RealmAccess *realmAccess `json:"realm_access,omitempty"`
	// Groups — группы пользователя.
	Groups []string `json:"groups,omitempty"`
	// Scope — scopes через пробел (для Service Account).
	Scope string `json:"scope,omitempty"`
	// ClientID — client_id (для Service Account).
	ClientID string `json:"client_id,omitempty"`
	// Azp — authorized party (ID клиента в Keycloak).
	Azp string `json:"azp,omitempty"`
}

// realmAccess — вложенная структура realm_access в Keycloak JWT.
type realmAccess struct {
	Roles []string `json:"roles"`
}

// JWTAuth — middleware для JWT-аутентификации через JWKS Keycloak.
type JWTAuth struct {
	jwks           keyfunc.Keyfunc
	logger         *slog.Logger
	roleProvider   RoleOverrideProvider
	adminGroups    []string
	readonlyGroups []string
	issuer         string
	jwtLeeway      time.Duration
}

// NewJWTAuth создаёт JWT middleware с JWKS из Keycloak.
// jwksURL — URL к JWKS endpoint Keycloak.
// caCertPath — опциональный путь к CA-сертификату для TLS.
// issuer — ожидаемый issuer JWT (обычно https://keycloak/realms/artstore).
// roleProvider — провайдер role overrides из БД (может быть nil).
// adminGroups, readonlyGroups — группы для маппинга в роли.
// jwksClientTimeout — таймаут HTTP-клиента JWKS (AM_JWKS_CLIENT_TIMEOUT).
// jwksRefreshInterval — интервал обновления JWKS-ключей (AM_JWKS_REFRESH_INTERVAL).
// jwtLeeway — допустимое отклонение времени при проверке JWT (AM_JWT_LEEWAY).
func NewJWTAuth(
	jwksURL string,
	caCertPath string,
	issuer string,
	roleProvider RoleOverrideProvider,
	adminGroups, readonlyGroups []string,
	jwksClientTimeout time.Duration,
	jwksRefreshInterval time.Duration,
	jwtLeeway time.Duration,
	logger *slog.Logger,
) (*JWTAuth, error) {
	// HTTP-клиент для JWKS (с кастомным CA или стандартный)
	httpClient := http.DefaultClient
	if caCertPath != "" {
		var err error
		httpClient, err = httpClientWithCA(caCertPath, jwksClientTimeout)
		if err != nil {
			return nil, fmt.Errorf("загрузка CA-сертификата %s: %w", caCertPath, err)
		}
		logger.Info("CA-сертификат для JWKS добавлен в пул доверия",
			slog.String("ca_cert", caCertPath),
		)
	}

	// JWKS Storage с фоновым обновлением.
	// NoErrorReturnFirstHTTPReq — стартуем даже если Keycloak ещё недоступен.
	storage, err := jwkset.NewStorageFromHTTP(jwksURL, jwkset.HTTPClientStorageOptions{
		Client:                    httpClient,
		NoErrorReturnFirstHTTPReq: true,
		RefreshInterval:           jwksRefreshInterval,
		RefreshErrorHandler: func(_ context.Context, err error) {
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
		jwks:           k,
		logger:         logger.With(slog.String("component", "jwt_auth")),
		roleProvider:   roleProvider,
		adminGroups:    adminGroups,
		readonlyGroups: readonlyGroups,
		issuer:         issuer,
		jwtLeeway:      jwtLeeway,
	}, nil
}

// httpClientWithCA создаёт HTTP-клиент с кастомным CA-сертификатом.
// timeout — таймаут HTTP-запросов.
func httpClientWithCA(caCertPath string, timeout time.Duration) (*http.Client, error) {
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
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
			},
		},
	}, nil
}

// NewJWTAuthWithKeyfunc создаёт JWT middleware с предоставленной keyfunc.
// Используется в тестах для подстановки mock JWKS.
func NewJWTAuthWithKeyfunc(
	kf keyfunc.Keyfunc,
	issuer string,
	roleProvider RoleOverrideProvider,
	adminGroups, readonlyGroups []string,
	logger *slog.Logger,
) *JWTAuth {
	return &JWTAuth{
		jwks:           kf,
		logger:         logger.With(slog.String("component", "jwt_auth")),
		roleProvider:   roleProvider,
		adminGroups:    adminGroups,
		readonlyGroups: readonlyGroups,
		issuer:         issuer,
	}
}

// Middleware возвращает HTTP middleware для JWT-аутентификации.
// Извлекает Bearer token, валидирует подпись (RS256), извлекает claims,
// определяет тип субъекта, вычисляет effective role и помещает в контекст.
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

			// Парсинг и валидация JWT через JWKS
			rawClaims := &keycloakClaims{}
			parserOpts := []jwt.ParserOption{
				jwt.WithValidMethods([]string{"RS256"}),
				jwt.WithExpirationRequired(),
				jwt.WithLeeway(j.jwtLeeway),
			}
			if j.issuer != "" {
				parserOpts = append(parserOpts, jwt.WithIssuer(j.issuer))
			}

			token, err := jwt.ParseWithClaims(tokenString, rawClaims, j.jwks.KeyfuncCtx(r.Context()), parserOpts...)
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
			subject, err := rawClaims.GetSubject()
			if err != nil || subject == "" {
				apierrors.Unauthorized(w, "Отсутствует sub в токене")
				return
			}

			// Формируем AuthClaims
			authClaims := j.buildAuthClaims(r.Context(), rawClaims)

			// Помещаем claims в контекст
			ctx := context.WithValue(r.Context(), ContextKeyClaims, authClaims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// buildAuthClaims формирует AuthClaims из raw Keycloak claims.
// Определяет тип субъекта, маппит группы → роли, применяет role overrides.
func (j *JWTAuth) buildAuthClaims(ctx context.Context, raw *keycloakClaims) *AuthClaims {
	claims := &AuthClaims{
		Subject:           raw.Subject,
		PreferredUsername: raw.PreferredUsername,
		Email:             raw.Email,
	}

	// Определяем тип субъекта.
	// Service Account в Keycloak имеет client_id в JWT и scope.
	// Admin User имеет groups и realm_access.roles.
	//nolint:nestif // TODO: упростить парсинг claims
	if raw.ClientID != "" && raw.Scope != "" {
		// Service Account (Client Credentials flow)
		claims.SubjectType = SubjectTypeSA
		claims.ClientID = raw.ClientID
		claims.Scopes = parseScopeString(raw.Scope)
	} else {
		// Admin User (Authorization Code flow)
		claims.SubjectType = SubjectTypeUser

		// Роли из realm_access.roles
		if raw.RealmAccess != nil {
			claims.Roles = raw.RealmAccess.Roles
		}

		// Группы
		claims.Groups = raw.Groups

		// Маппинг групп → роль через RBAC
		claims.IdpRole = rbac.MapGroupsToRole(claims.Groups, j.adminGroups, j.readonlyGroups)

		// Если роль не определена через группы, пробуем через realm_access.roles
		if claims.IdpRole == "" && len(claims.Roles) > 0 {
			var mappedRoles []string
			for _, r := range claims.Roles {
				if rbac.IsValidRole(r) {
					mappedRoles = append(mappedRoles, r)
				}
			}
			claims.IdpRole = rbac.HighestRole(mappedRoles)
		}

		// Применяем role override из БД
		if j.roleProvider != nil {
			override, err := j.roleProvider.GetRoleOverride(ctx, claims.Subject)
			if err != nil {
				j.logger.Warn("Ошибка получения role override",
					slog.String("user_id", claims.Subject),
					slog.String("error", err.Error()),
				)
			} else {
				claims.RoleOverride = override
			}
		}

		// Вычисляем effective role
		claims.EffectiveRole = rbac.EffectiveRole(claims.IdpRole, claims.RoleOverride)
	}

	return claims
}

// parseScopeString разбирает строку scopes из JWT (space-separated).
// Keycloak помещает scopes в формате "openid profile files:read files:write".
func parseScopeString(scope string) []string {
	if scope == "" {
		return nil
	}
	parts := strings.Fields(scope)
	return parts
}

// --- RBAC middleware helpers ---

// RequireRole возвращает middleware, требующий одну из указанных ролей.
// Работает только для Admin Users. SA с подходящим scope не пропускаются.
// Должен использоваться ПОСЛЕ JWTAuth.Middleware().
//
//nolint:dupl // TODO: обобщить RequireRole/RequireScope
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				apierrors.Unauthorized(w, "Отсутствуют claims в контексте")
				return
			}

			if claims.SubjectType != SubjectTypeUser {
				apierrors.Forbidden(w, "Доступ разрешён только для пользователей")
				return
			}

			if !claims.HasAnyRole(roles...) {
				apierrors.Forbidden(w, fmt.Sprintf("Недостаточно прав: требуется роль %s", strings.Join(roles, " или ")))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireScope возвращает middleware, требующий один из указанных scopes.
// Работает только для Service Accounts. Users с подходящей ролью не пропускаются.
// Должен использоваться ПОСЛЕ JWTAuth.Middleware().
//
//nolint:dupl // TODO: обобщить RequireRole/RequireScope
func RequireScope(scopes ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				apierrors.Unauthorized(w, "Отсутствуют claims в контексте")
				return
			}

			if claims.SubjectType != SubjectTypeSA {
				apierrors.Forbidden(w, "Доступ разрешён только для Service Accounts")
				return
			}

			if !claims.HasAnyScope(scopes...) {
				apierrors.Forbidden(w, fmt.Sprintf("Недостаточно прав: требуется scope %s", strings.Join(scopes, " или ")))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireRoleOrScope возвращает middleware, пропускающий Admin Users с одной
// из указанных ролей ИЛИ Service Accounts с одним из указанных scopes.
// Это основной middleware для endpoints, доступных обоим типам субъектов.
// Должен использоваться ПОСЛЕ JWTAuth.Middleware().
func RequireRoleOrScope(roles, scopes []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				apierrors.Unauthorized(w, "Отсутствуют claims в контексте")
				return
			}

			switch claims.SubjectType {
			case SubjectTypeUser:
				if claims.HasAnyRole(roles...) {
					next.ServeHTTP(w, r)
					return
				}
				apierrors.Forbidden(w, fmt.Sprintf("Недостаточно прав: требуется роль %s", strings.Join(roles, " или ")))

			case SubjectTypeSA:
				if claims.HasAnyScope(scopes...) {
					next.ServeHTTP(w, r)
					return
				}
				apierrors.Forbidden(w, fmt.Sprintf("Недостаточно прав: требуется scope %s", strings.Join(scopes, " или ")))

			default:
				apierrors.Forbidden(w, "Неизвестный тип субъекта")
			}
		})
	}
}

// --- Context helpers ---

// ClaimsFromContext извлекает AuthClaims из контекста запроса.
// Возвращает nil, если claims не найдены.
func ClaimsFromContext(ctx context.Context) *AuthClaims {
	claims, _ := ctx.Value(ContextKeyClaims).(*AuthClaims)
	return claims
}

// SubjectFromContext извлекает sub из контекста запроса.
// Возвращает пустую строку, если claims не найдены.
func SubjectFromContext(ctx context.Context) string {
	claims := ClaimsFromContext(ctx)
	if claims == nil {
		return ""
	}
	return claims.Subject
}

// EffectiveRoleFromContext извлекает effective role из контекста запроса.
// Возвращает пустую строку, если claims не найдены.
func EffectiveRoleFromContext(ctx context.Context) string {
	claims := ClaimsFromContext(ctx)
	if claims == nil {
		return ""
	}
	return claims.EffectiveRole
}

// --- ReadinessChecker для Keycloak ---

// KeycloakReadinessChecker — проверка доступности Keycloak через JWKS.
type KeycloakReadinessChecker struct {
	jwksURL string
	client  *http.Client
}

// NewKeycloakReadinessChecker создаёт checker доступности Keycloak.
// readinessTimeout — таймаут проверки готовности (AM_KEYCLOAK_READINESS_TIMEOUT).
func NewKeycloakReadinessChecker(jwksURL, caCertPath string, readinessTimeout time.Duration) (*KeycloakReadinessChecker, error) {
	client := &http.Client{Timeout: readinessTimeout}
	if caCertPath != "" {
		var err error
		client, err = httpClientWithCA(caCertPath, readinessTimeout)
		if err != nil {
			return nil, fmt.Errorf("загрузка CA для readiness checker: %w", err)
		}
	}

	return &KeycloakReadinessChecker{
		jwksURL: jwksURL,
		client:  client,
	}, nil
}

const statusFail = "fail"

// CheckReady проверяет доступность JWKS endpoint Keycloak.
func (k *KeycloakReadinessChecker) CheckReady() (status, message string) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, k.jwksURL, http.NoBody)
	if err != nil {
		return statusFail, "ошибка создания запроса: " + err.Error()
	}
	resp, err := k.client.Do(req) //nolint:gosec // G704: URL из конфигурации Keycloak
	if err != nil {
		return statusFail, fmt.Sprintf("Keycloak JWKS недоступен: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return statusFail, fmt.Sprintf("Keycloak JWKS вернул статус %d", resp.StatusCode)
	}

	// Проверяем, что ответ — валидный JSON с ключами
	var jwksResp struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jwksResp); err != nil {
		return "degraded", fmt.Sprintf("Keycloak JWKS: невалидный JSON: %v", err)
	}

	if len(jwksResp.Keys) == 0 {
		return "degraded", "Keycloak JWKS: нет ключей"
	}

	return "ok", fmt.Sprintf("JWKS доступен, ключей: %d", len(jwksResp.Keys))
}

// Close освобождает ресурсы JWT middleware.
func (j *JWTAuth) Close() {
	// keyfunc v3 не требует явного закрытия
}
