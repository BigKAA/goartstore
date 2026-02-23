package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
)

// TestGeneratePKCE проверяет генерацию PKCE code_verifier и code_challenge.
func TestGeneratePKCE(t *testing.T) {
	params, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("Ошибка генерации PKCE: %v", err)
	}

	// code_verifier должен быть 43 символа (32 bytes → base64url без padding)
	if len(params.CodeVerifier) != 43 {
		t.Errorf("CodeVerifier length: want 43, got %d", len(params.CodeVerifier))
	}

	// code_challenge должен быть base64url(SHA-256(code_verifier))
	hash := sha256.Sum256([]byte(params.CodeVerifier))
	expectedChallenge := base64.RawURLEncoding.EncodeToString(hash[:])
	if params.CodeChallenge != expectedChallenge {
		t.Errorf("CodeChallenge не совпадает с SHA-256(code_verifier)")
	}
}

// TestGeneratePKCEUniqueness проверяет, что каждый вызов генерирует уникальные значения.
func TestGeneratePKCEUniqueness(t *testing.T) {
	params1, _ := GeneratePKCE()
	params2, _ := GeneratePKCE()

	if params1.CodeVerifier == params2.CodeVerifier {
		t.Error("Два вызова GeneratePKCE вернули одинаковые code_verifier")
	}
}

// TestGenerateState проверяет генерацию state parameter.
func TestGenerateState(t *testing.T) {
	state1, err := GenerateState()
	if err != nil {
		t.Fatalf("Ошибка генерации state: %v", err)
	}

	if state1 == "" {
		t.Error("State не должен быть пустым")
	}

	state2, _ := GenerateState()
	if state1 == state2 {
		t.Error("Два вызова GenerateState вернули одинаковые значения")
	}
}

// TestOIDCClientAuthorizeURL проверяет формирование authorize URL.
func TestOIDCClientAuthorizeURL(t *testing.T) {
	client := NewOIDCClient(OIDCConfig{
		KeycloakURL: "https://keycloak.example.com",
		Realm:       "artstore",
		ClientID:    "artstore-admin-ui",
	})

	authURL := client.AuthorizeURL(
		"http://localhost:8000/admin/callback",
		"test-state-123",
		"test-challenge-456",
	)

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("Ошибка парсинга URL: %v", err)
	}

	// Проверяем базовый URL
	expectedBase := "https://keycloak.example.com/realms/artstore/protocol/openid-connect/auth"
	if !strings.HasPrefix(authURL, expectedBase) {
		t.Errorf("URL должен начинаться с %s, получено: %s", expectedBase, authURL)
	}

	// Проверяем query parameters
	params := parsed.Query()
	tests := map[string]string{
		"client_id":             "artstore-admin-ui",
		"response_type":        "code",
		"redirect_uri":         "http://localhost:8000/admin/callback",
		"state":                "test-state-123",
		"code_challenge":       "test-challenge-456",
		"code_challenge_method": "S256",
	}

	for key, want := range tests {
		got := params.Get(key)
		if got != want {
			t.Errorf("Parameter %s: want %q, got %q", key, want, got)
		}
	}

	// Scope должен содержать openid profile email groups
	scope := params.Get("scope")
	for _, s := range []string{"openid", "profile", "email", "groups"} {
		if !strings.Contains(scope, s) {
			t.Errorf("Scope должен содержать %q, scope=%q", s, scope)
		}
	}
}

// TestOIDCClientLogoutURL проверяет формирование logout URL.
func TestOIDCClientLogoutURL(t *testing.T) {
	client := NewOIDCClient(OIDCConfig{
		KeycloakURL: "https://keycloak.example.com",
		Realm:       "artstore",
		ClientID:    "artstore-admin-ui",
	})

	logoutURL := client.LogoutURL("id-token-hint", "http://localhost:8000/admin/login")

	parsed, err := url.Parse(logoutURL)
	if err != nil {
		t.Fatalf("Ошибка парсинга URL: %v", err)
	}

	params := parsed.Query()
	if params.Get("client_id") != "artstore-admin-ui" {
		t.Errorf("client_id: want artstore-admin-ui, got %s", params.Get("client_id"))
	}
	if params.Get("id_token_hint") != "id-token-hint" {
		t.Errorf("id_token_hint: want id-token-hint, got %s", params.Get("id_token_hint"))
	}
	if params.Get("post_logout_redirect_uri") != "http://localhost:8000/admin/login" {
		t.Errorf("post_logout_redirect_uri не совпадает")
	}
}
