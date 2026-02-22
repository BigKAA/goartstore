package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

// testKeyID — идентификатор ключа для тестов.
const testKeyID = "test-key-am"

// mockRoleProvider — мок для RoleOverrideProvider.
type mockRoleProvider struct {
	overrides map[string]*string
}

func (m *mockRoleProvider) GetRoleOverride(_ context.Context, keycloakUserID string) (*string, error) {
	if m == nil || m.overrides == nil {
		return nil, nil
	}
	override, ok := m.overrides[keycloakUserID]
	if !ok {
		return nil, nil
	}
	return override, nil
}

// generateTestKey генерирует RSA ключ для тестов.
func generateTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// buildJWKSetJSON строит JWKS JSON из RSA публичного ключа.
func buildJWKSetJSON(pub *rsa.PublicKey, kid string) json.RawMessage {
	_ = x509.MarshalPKCS1PublicKey(pub)
	nBytes := pub.N.Bytes()
	nB64 := base64.RawURLEncoding.EncodeToString(nBytes)
	eBytes := big.NewInt(int64(pub.E)).Bytes()
	eB64 := base64.RawURLEncoding.EncodeToString(eBytes)

	jwks := map[string]any{
		"keys": []map[string]any{
			{
				"kty": "RSA",
				"kid": kid,
				"use": "sig",
				"alg": "RS256",
				"n":   nB64,
				"e":   eB64,
			},
		},
	}

	data, _ := json.Marshal(jwks)
	return data
}

// testLogger создаёт logger для тестов.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// newTestJWTAuth создаёт JWTAuth для тестов с Admin User claims.
func newTestJWTAuth(t *testing.T, key *rsa.PrivateKey, roleProvider RoleOverrideProvider) *JWTAuth {
	t.Helper()
	jwksJSON := buildJWKSetJSON(&key.PublicKey, testKeyID)
	kf, err := keyfunc.NewJWKSetJSON(jwksJSON)
	if err != nil {
		t.Fatalf("не удалось создать keyfunc: %v", err)
	}

	return NewJWTAuthWithKeyfunc(
		kf,
		"https://keycloak.test/realms/artsore",
		roleProvider,
		[]string{"artsore-admins"},
		[]string{"artsore-viewers"},
		testLogger(),
	)
}

// generateUserToken генерирует JWT для Admin User.
func generateUserToken(t *testing.T, key *rsa.PrivateKey, sub, username, email string, roles, groups []string, expired bool) string {
	t.Helper()

	exp := time.Now().Add(time.Hour)
	if expired {
		exp = time.Now().Add(-time.Hour)
	}

	claims := jwt.MapClaims{
		"sub":                sub,
		"preferred_username": username,
		"email":              email,
		"iss":                "https://keycloak.test/realms/artsore",
		"exp":                jwt.NewNumericDate(exp),
		"nbf":                jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		"iat":                jwt.NewNumericDate(time.Now()),
	}

	if len(roles) > 0 {
		claims["realm_access"] = map[string]any{
			"roles": roles,
		}
	}
	if len(groups) > 0 {
		claims["groups"] = groups
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = testKeyID
	tokenStr, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return tokenStr
}

// generateSAToken генерирует JWT для Service Account.
func generateSAToken(t *testing.T, key *rsa.PrivateKey, sub, clientID, scope string, expired bool) string {
	t.Helper()

	exp := time.Now().Add(time.Hour)
	if expired {
		exp = time.Now().Add(-time.Hour)
	}

	claims := jwt.MapClaims{
		"sub":       sub,
		"client_id": clientID,
		"scope":     scope,
		"iss":       "https://keycloak.test/realms/artsore",
		"exp":       jwt.NewNumericDate(exp),
		"nbf":       jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		"iat":       jwt.NewNumericDate(time.Now()),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = testKeyID
	tokenStr, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return tokenStr
}

// --- Тесты JWT Middleware ---

// TestJWTAuth_ValidUserToken — валидный JWT Admin User.
func TestJWTAuth_ValidUserToken(t *testing.T) {
	key := generateTestKey(t)
	auth := newTestJWTAuth(t, key, nil)

	handler := auth.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromContext(r.Context())
		if claims == nil {
			t.Fatal("claims не найдены в контексте")
		}

		if claims.Subject != "user-123" {
			t.Errorf("ожидался sub=user-123, получен %s", claims.Subject)
		}
		if claims.SubjectType != SubjectTypeUser {
			t.Errorf("ожидался SubjectType=user, получен %s", claims.SubjectType)
		}
		if claims.PreferredUsername != "admin" {
			t.Errorf("ожидался username=admin, получен %s", claims.PreferredUsername)
		}
		if claims.Email != "admin@test.com" {
			t.Errorf("ожидался email=admin@test.com, получен %s", claims.Email)
		}
		if claims.IdpRole != "admin" {
			t.Errorf("ожидался IdpRole=admin, получен %s", claims.IdpRole)
		}
		if claims.EffectiveRole != "admin" {
			t.Errorf("ожидался EffectiveRole=admin, получен %s", claims.EffectiveRole)
		}

		w.WriteHeader(http.StatusOK)
	}))

	tokenStr := generateUserToken(t, key, "user-123", "admin", "admin@test.com",
		[]string{"admin"}, []string{"artsore-admins"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin-auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ожидался статус 200, получен %d, тело: %s", rec.Code, rec.Body.String())
	}
}

// TestJWTAuth_ValidSAToken — валидный JWT Service Account.
func TestJWTAuth_ValidSAToken(t *testing.T) {
	key := generateTestKey(t)
	auth := newTestJWTAuth(t, key, nil)

	handler := auth.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromContext(r.Context())
		if claims == nil {
			t.Fatal("claims не найдены в контексте")
		}

		if claims.SubjectType != SubjectTypeSA {
			t.Errorf("ожидался SubjectType=service_account, получен %s", claims.SubjectType)
		}
		if claims.ClientID != "sa_ingester_abc123" {
			t.Errorf("ожидался ClientID=sa_ingester_abc123, получен %s", claims.ClientID)
		}
		if !claims.HasScope("files:read") {
			t.Error("ожидался scope files:read")
		}
		if !claims.HasScope("files:write") {
			t.Error("ожидался scope files:write")
		}
		if claims.HasScope("admin:write") {
			t.Error("не ожидался scope admin:write")
		}

		w.WriteHeader(http.StatusOK)
	}))

	tokenStr := generateSAToken(t, key, "sa-uuid-456", "sa_ingester_abc123",
		"openid files:read files:write", false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ожидался статус 200, получен %d, тело: %s", rec.Code, rec.Body.String())
	}
}

// TestJWTAuth_MissingToken — отсутствие Authorization header.
func TestJWTAuth_MissingToken(t *testing.T) {
	key := generateTestKey(t)
	auth := newTestJWTAuth(t, key, nil)
	handler := auth.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler не должен быть вызван")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("ожидался статус 401, получен %d", rec.Code)
	}
}

// TestJWTAuth_ExpiredToken — просроченный токен.
func TestJWTAuth_ExpiredToken(t *testing.T) {
	key := generateTestKey(t)
	auth := newTestJWTAuth(t, key, nil)
	handler := auth.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler не должен быть вызван")
	}))

	tokenStr := generateUserToken(t, key, "user-123", "admin", "admin@test.com",
		nil, nil, true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("ожидался статус 401, получен %d", rec.Code)
	}
}

// TestJWTAuth_InvalidFormat — некорректный формат Authorization.
func TestJWTAuth_InvalidFormat(t *testing.T) {
	key := generateTestKey(t)
	auth := newTestJWTAuth(t, key, nil)
	handler := auth.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler не должен быть вызван")
	}))

	tests := []struct {
		name   string
		header string
	}{
		{"basic auth", "Basic dXNlcjpwYXNz"},
		{"no bearer prefix", "token123"},
		{"empty bearer", "Bearer "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/files", nil)
			req.Header.Set("Authorization", tt.header)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("ожидался статус 401, получен %d", rec.Code)
			}
		})
	}
}

// TestJWTAuth_RoleOverride — role override повышает роль.
func TestJWTAuth_RoleOverride(t *testing.T) {
	key := generateTestKey(t)
	adminRole := "admin"
	provider := &mockRoleProvider{
		overrides: map[string]*string{
			"user-readonly": &adminRole,
		},
	}
	auth := newTestJWTAuth(t, key, provider)

	handler := auth.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromContext(r.Context())
		if claims == nil {
			t.Fatal("claims не найдены")
		}

		// IdpRole = readonly (из группы artsore-viewers)
		if claims.IdpRole != "readonly" {
			t.Errorf("ожидался IdpRole=readonly, получен %s", claims.IdpRole)
		}
		// RoleOverride = admin
		if claims.RoleOverride == nil || *claims.RoleOverride != "admin" {
			t.Error("ожидался RoleOverride=admin")
		}
		// EffectiveRole = max(readonly, admin) = admin
		if claims.EffectiveRole != "admin" {
			t.Errorf("ожидался EffectiveRole=admin, получен %s", claims.EffectiveRole)
		}

		w.WriteHeader(http.StatusOK)
	}))

	tokenStr := generateUserToken(t, key, "user-readonly", "viewer", "viewer@test.com",
		nil, []string{"artsore-viewers"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin-auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ожидался статус 200, получен %d", rec.Code)
	}
}

// TestJWTAuth_RoleOverrideCannotDemote — override не понижает роль.
func TestJWTAuth_RoleOverrideCannotDemote(t *testing.T) {
	key := generateTestKey(t)
	readonlyRole := "readonly"
	provider := &mockRoleProvider{
		overrides: map[string]*string{
			"user-admin": &readonlyRole,
		},
	}
	auth := newTestJWTAuth(t, key, provider)

	handler := auth.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromContext(r.Context())
		if claims == nil {
			t.Fatal("claims не найдены")
		}

		// IdpRole = admin, RoleOverride = readonly
		// EffectiveRole = max(admin, readonly) = admin (не понижается)
		if claims.EffectiveRole != "admin" {
			t.Errorf("ожидался EffectiveRole=admin, получен %s", claims.EffectiveRole)
		}

		w.WriteHeader(http.StatusOK)
	}))

	tokenStr := generateUserToken(t, key, "user-admin", "admin", "admin@test.com",
		nil, []string{"artsore-admins"}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin-auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ожидался статус 200, получен %d", rec.Code)
	}
}

// TestJWTAuth_GroupMapping — маппинг групп в роли.
func TestJWTAuth_GroupMapping(t *testing.T) {
	tests := []struct {
		name         string
		groups       []string
		expectedRole string
	}{
		{"admin group", []string{"artsore-admins"}, "admin"},
		{"readonly group", []string{"artsore-viewers"}, "readonly"},
		{"both groups", []string{"artsore-admins", "artsore-viewers"}, "admin"},
		{"no groups", []string{}, ""},
		{"unknown group", []string{"other-group"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := generateTestKey(t)
			auth := newTestJWTAuth(t, key, nil)

			handler := auth.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				claims := ClaimsFromContext(r.Context())
				if claims == nil {
					t.Fatal("claims не найдены")
				}
				if claims.IdpRole != tt.expectedRole {
					t.Errorf("ожидался IdpRole=%q, получен %q", tt.expectedRole, claims.IdpRole)
				}
				w.WriteHeader(http.StatusOK)
			}))

			tokenStr := generateUserToken(t, key, "user-123", "user", "user@test.com",
				nil, tt.groups, false)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin-auth/me", nil)
			req.Header.Set("Authorization", "Bearer "+tokenStr)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("ожидался статус 200, получен %d", rec.Code)
			}
		})
	}
}

// --- Тесты RBAC middleware ---

// TestRequireRole_HasRole — пользователь с нужной ролью.
func TestRequireRole_HasRole(t *testing.T) {
	handler := RequireRole("admin", "readonly")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	claims := &AuthClaims{
		SubjectType:   SubjectTypeUser,
		EffectiveRole: "admin",
	}
	ctx := context.WithValue(context.Background(), ContextKeyClaims, claims)
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ожидался статус 200, получен %d", rec.Code)
	}
}

// TestRequireRole_MissingRole — пользователь без нужной роли.
func TestRequireRole_MissingRole(t *testing.T) {
	handler := RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler не должен быть вызван")
	}))

	claims := &AuthClaims{
		SubjectType:   SubjectTypeUser,
		EffectiveRole: "readonly",
	}
	ctx := context.WithValue(context.Background(), ContextKeyClaims, claims)
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("ожидался статус 403, получен %d", rec.Code)
	}
}

// TestRequireRole_SADenied — SA не проходит RequireRole.
func TestRequireRole_SADenied(t *testing.T) {
	handler := RequireRole("admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler не должен быть вызван")
	}))

	claims := &AuthClaims{
		SubjectType: SubjectTypeSA,
		Scopes:      []string{"admin:write"},
	}
	ctx := context.WithValue(context.Background(), ContextKeyClaims, claims)
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("ожидался статус 403, получен %d", rec.Code)
	}
}

// TestRequireScope_HasScope — SA с нужным scope.
func TestRequireScope_HasScope(t *testing.T) {
	handler := RequireScope("files:read")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	claims := &AuthClaims{
		SubjectType: SubjectTypeSA,
		Scopes:      []string{"files:read", "files:write"},
	}
	ctx := context.WithValue(context.Background(), ContextKeyClaims, claims)
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ожидался статус 200, получен %d", rec.Code)
	}
}

// TestRequireScope_MissingScope — SA без нужного scope.
func TestRequireScope_MissingScope(t *testing.T) {
	handler := RequireScope("storage:write")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler не должен быть вызван")
	}))

	claims := &AuthClaims{
		SubjectType: SubjectTypeSA,
		Scopes:      []string{"files:read"},
	}
	ctx := context.WithValue(context.Background(), ContextKeyClaims, claims)
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("ожидался статус 403, получен %d", rec.Code)
	}
}

// TestRequireScope_UserDenied — User не проходит RequireScope.
func TestRequireScope_UserDenied(t *testing.T) {
	handler := RequireScope("files:read")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler не должен быть вызван")
	}))

	claims := &AuthClaims{
		SubjectType:   SubjectTypeUser,
		EffectiveRole: "admin",
	}
	ctx := context.WithValue(context.Background(), ContextKeyClaims, claims)
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("ожидался статус 403, получен %d", rec.Code)
	}
}

// TestRequireRoleOrScope_UserWithRole — User с ролью проходит RequireRoleOrScope.
func TestRequireRoleOrScope_UserWithRole(t *testing.T) {
	handler := RequireRoleOrScope(
		[]string{"admin", "readonly"},
		[]string{"storage:read"},
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	claims := &AuthClaims{
		SubjectType:   SubjectTypeUser,
		EffectiveRole: "readonly",
	}
	ctx := context.WithValue(context.Background(), ContextKeyClaims, claims)
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ожидался статус 200, получен %d", rec.Code)
	}
}

// TestRequireRoleOrScope_SAWithScope — SA с scope проходит RequireRoleOrScope.
func TestRequireRoleOrScope_SAWithScope(t *testing.T) {
	handler := RequireRoleOrScope(
		[]string{"admin"},
		[]string{"storage:read"},
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	claims := &AuthClaims{
		SubjectType: SubjectTypeSA,
		Scopes:      []string{"storage:read", "storage:write"},
	}
	ctx := context.WithValue(context.Background(), ContextKeyClaims, claims)
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ожидался статус 200, получен %d", rec.Code)
	}
}

// TestRequireRoleOrScope_Denied — ни роль, ни scope не совпадают.
func TestRequireRoleOrScope_Denied(t *testing.T) {
	handler := RequireRoleOrScope(
		[]string{"admin"},
		[]string{"storage:write"},
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler не должен быть вызван")
	}))

	// User без роли admin
	claims := &AuthClaims{
		SubjectType:   SubjectTypeUser,
		EffectiveRole: "readonly",
	}
	ctx := context.WithValue(context.Background(), ContextKeyClaims, claims)
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("ожидался статус 403, получен %d", rec.Code)
	}
}

// TestRequireRoleOrScope_NoClaims — отсутствие claims в контексте.
func TestRequireRoleOrScope_NoClaims(t *testing.T) {
	handler := RequireRoleOrScope(
		[]string{"admin"},
		[]string{"files:read"},
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler не должен быть вызван")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("ожидался статус 401, получен %d", rec.Code)
	}
}

// --- Тесты context helpers ---

// TestClaimsFromContext_Empty — пустой контекст.
func TestClaimsFromContext_Empty(t *testing.T) {
	if claims := ClaimsFromContext(context.Background()); claims != nil {
		t.Errorf("ожидался nil, получено %+v", claims)
	}
}

// TestSubjectFromContext — извлечение subject.
func TestSubjectFromContext(t *testing.T) {
	claims := &AuthClaims{Subject: "user-123"}
	ctx := context.WithValue(context.Background(), ContextKeyClaims, claims)

	if sub := SubjectFromContext(ctx); sub != "user-123" {
		t.Errorf("ожидался user-123, получен %q", sub)
	}
}

// TestSubjectFromContext_Empty — пустой контекст.
func TestSubjectFromContext_Empty(t *testing.T) {
	if sub := SubjectFromContext(context.Background()); sub != "" {
		t.Errorf("ожидалась пустая строка, получено %q", sub)
	}
}

// TestParseScopeString — парсинг scope строки.
func TestParseScopeString(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"openid files:read files:write", []string{"openid", "files:read", "files:write"}},
		{"files:read", []string{"files:read"}},
		{"", nil},
		{"  openid  ", []string{"openid"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseScopeString(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("ожидалось %d scopes, получено %d: %v", len(tt.expected), len(result), result)
				return
			}
			for i, s := range result {
				if s != tt.expected[i] {
					t.Errorf("scope[%d]: ожидалось %q, получено %q", i, tt.expected[i], s)
				}
			}
		})
	}
}

// TestAuthClaims_HasRole — проверка HasRole.
func TestAuthClaims_HasRole(t *testing.T) {
	claims := &AuthClaims{EffectiveRole: "admin"}

	if !claims.HasRole("admin") {
		t.Error("ожидался HasRole(admin) = true")
	}
	if claims.HasRole("readonly") {
		t.Error("ожидался HasRole(readonly) = false")
	}
}

// TestAuthClaims_HasScope — проверка HasScope.
func TestAuthClaims_HasScope(t *testing.T) {
	claims := &AuthClaims{Scopes: []string{"files:read", "files:write"}}

	if !claims.HasScope("files:read") {
		t.Error("ожидался HasScope(files:read) = true")
	}
	if claims.HasScope("admin:write") {
		t.Error("ожидался HasScope(admin:write) = false")
	}
}

// TestJWTAuth_WrongIssuer — токен с неверным issuer.
func TestJWTAuth_WrongIssuer(t *testing.T) {
	key := generateTestKey(t)
	auth := newTestJWTAuth(t, key, nil)
	handler := auth.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler не должен быть вызван")
	}))

	// Генерируем токен с другим issuer
	exp := time.Now().Add(time.Hour)
	claims := jwt.MapClaims{
		"sub":                "user-123",
		"preferred_username": "admin",
		"iss":                "https://other-keycloak.test/realms/other",
		"exp":                jwt.NewNumericDate(exp),
		"nbf":                jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		"iat":                jwt.NewNumericDate(time.Now()),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = testKeyID
	tokenStr, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin-auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("ожидался статус 401, получен %d", rec.Code)
	}
}

// TestJWTAuth_RolesFromRealmAccess — роли из realm_access при отсутствии групп.
func TestJWTAuth_RolesFromRealmAccess(t *testing.T) {
	key := generateTestKey(t)
	auth := newTestJWTAuth(t, key, nil)

	handler := auth.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromContext(r.Context())
		if claims == nil {
			t.Fatal("claims не найдены")
		}
		// Без групп, но с realm_access.roles=["admin"] → IdpRole=admin
		if claims.IdpRole != "admin" {
			t.Errorf("ожидался IdpRole=admin, получен %s", claims.IdpRole)
		}
		w.WriteHeader(http.StatusOK)
	}))

	// Токен без groups, но с realm_access.roles
	tokenStr := generateUserToken(t, key, "user-123", "admin", "admin@test.com",
		[]string{"admin", "default-roles-artsore"}, nil, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin-auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ожидался статус 200, получен %d", rec.Code)
	}
}
