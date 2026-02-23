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
const testKeyID = "test-key"

// generateTestKey генерирует RSA ключ для тестов.
func generateTestKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, 2048)
}

// generateTestToken генерирует JWT токен для тестов.
func generateTestToken(key *rsa.PrivateKey, claims Claims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = testKeyID
	return token.SignedString(key)
}

// buildJWKSetJSON строит JWKS JSON из RSA публичного ключа.
func buildJWKSetJSON(pub *rsa.PublicKey, kid string) json.RawMessage {
	// Сериализуем публичный ключ в DER
	_ = x509.MarshalPKCS1PublicKey(pub)

	// Кодируем N и E в base64url
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

// newTestJWTAuth создаёт JWTAuth с RSA ключом для тестов.
func newTestJWTAuth(key *rsa.PrivateKey) *JWTAuth {
	jwksJSON := buildJWKSetJSON(&key.PublicKey, testKeyID)
	kf, err := keyfunc.NewJWKSetJSON(jwksJSON)
	if err != nil {
		panic("не удалось создать keyfunc из JWKS JSON: " + err.Error())
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewJWTAuthWithKeyfunc(kf, logger)
}

// TestJWTAuth_ValidToken проверяет валидный JWT.
func TestJWTAuth_ValidToken(t *testing.T) {
	key, err := generateTestKey()
	if err != nil {
		t.Fatal(err)
	}

	auth := newTestJWTAuth(key)
	handler := auth.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sub := SubjectFromContext(r.Context())
		scopes := ScopesFromContext(r.Context())

		if sub != "test-user" {
			t.Errorf("ожидался sub=test-user, получен %s", sub)
		}
		if len(scopes) != 2 || scopes[0] != "files:read" {
			t.Errorf("неожиданные scopes: %v", scopes)
		}

		w.WriteHeader(http.StatusOK)
	}))

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "test-user",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			NotBefore: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		ScopeArray: []string{"files:read", "files:write"},
	}

	tokenString, err := generateTestToken(key, claims)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ожидался статус 200, получен %d, тело: %s", rec.Code, rec.Body.String())
	}
}

// TestJWTAuth_MissingToken проверяет отсутствие Authorization header.
func TestJWTAuth_MissingToken(t *testing.T) {
	key, _ := generateTestKey()
	auth := newTestJWTAuth(key)
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

// TestJWTAuth_ExpiredToken проверяет просроченный токен.
func TestJWTAuth_ExpiredToken(t *testing.T) {
	key, _ := generateTestKey()
	auth := newTestJWTAuth(key)
	handler := auth.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler не должен быть вызван")
	}))

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "test-user",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
		ScopeArray: []string{"files:read"},
	}

	tokenString, _ := generateTestToken(key, claims)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("ожидался статус 401, получен %d", rec.Code)
	}
}

// TestJWTAuth_InvalidFormat проверяет некорректный формат Authorization.
func TestJWTAuth_InvalidFormat(t *testing.T) {
	key, _ := generateTestKey()
	auth := newTestJWTAuth(key)
	handler := auth.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler не должен быть вызван")
	}))

	tests := []struct {
		name   string
		header string
	}{
		{"basic auth", "Basic dXNlcjpwYXNz"},
		{"no bearer prefix", "token123"},
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

// TestRequireScope_HasScope проверяет наличие нужного scope.
func TestRequireScope_HasScope(t *testing.T) {
	handler := RequireScope("files:read")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ctx := context.WithValue(context.Background(), ContextKeyScopes, []string{"files:read", "files:write"})
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ожидался статус 200, получен %d", rec.Code)
	}
}

// TestRequireScope_MissingScope проверяет отсутствие нужного scope.
func TestRequireScope_MissingScope(t *testing.T) {
	handler := RequireScope("storage:write")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler не должен быть вызван")
	}))

	ctx := context.WithValue(context.Background(), ContextKeyScopes, []string{"files:read"})
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("ожидался статус 403, получен %d", rec.Code)
	}
}

// TestRequireScope_NoScopes проверяет отсутствие scopes в контексте.
func TestRequireScope_NoScopes(t *testing.T) {
	handler := RequireScope("files:read")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler не должен быть вызван")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("ожидался статус 403, получен %d", rec.Code)
	}
}

// TestSubjectFromContext проверяет извлечение subject из контекста.
func TestSubjectFromContext_Empty(t *testing.T) {
	if sub := SubjectFromContext(context.Background()); sub != "" {
		t.Errorf("ожидалась пустая строка, получено %q", sub)
	}
}

func TestSubjectFromContext_WithValue(t *testing.T) {
	ctx := context.WithValue(context.Background(), ContextKeySubject, "admin")
	if sub := SubjectFromContext(ctx); sub != "admin" {
		t.Errorf("ожидалось admin, получено %q", sub)
	}
}
