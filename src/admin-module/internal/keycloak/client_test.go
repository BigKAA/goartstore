package keycloak

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// testLogger создаёт logger для тестов.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// setupMockKeycloak создаёт mock HTTP-сервер Keycloak.
// tokenHandler обрабатывает запросы на получение токена.
// adminHandler обрабатывает запросы к Admin REST API.
func setupMockKeycloak(t *testing.T, tokenHandler, adminHandler http.HandlerFunc) (*httptest.Server, *Client) {
	t.Helper()

	mux := http.NewServeMux()

	// Token endpoint
	mux.HandleFunc("/realms/artsore/protocol/openid-connect/token", func(w http.ResponseWriter, r *http.Request) {
		if tokenHandler != nil {
			tokenHandler(w, r)
			return
		}
		// Дефолтный ответ: валидный токен на 300 секунд
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: "test-access-token",
			TokenType:   "Bearer",
			ExpiresIn:   300,
		})
	})

	// Admin REST API
	mux.HandleFunc("/admin/realms/artsore/", func(w http.ResponseWriter, r *http.Request) {
		if adminHandler != nil {
			adminHandler(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client := New(
		server.URL,
		"artsore",
		"admin-module",
		"test-secret",
		server.Client(),
		testLogger(),
	)

	return server, client
}

// TestClient_TokenCaching проверяет кэширование токена.
func TestClient_TokenCaching(t *testing.T) {
	tokenRequests := 0

	_, client := setupMockKeycloak(t,
		func(w http.ResponseWriter, r *http.Request) {
			tokenRequests++
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(TokenResponse{
				AccessToken: "cached-token",
				TokenType:   "Bearer",
				ExpiresIn:   300,
			})
		},
		nil,
	)

	ctx := context.Background()

	// Первый запрос — получение токена
	token1, err := client.getToken(ctx)
	if err != nil {
		t.Fatalf("Ошибка получения токена: %v", err)
	}
	if token1 != "cached-token" {
		t.Errorf("ожидался cached-token, получен %s", token1)
	}

	// Второй запрос — из кэша (не должен вызывать HTTP)
	token2, err := client.getToken(ctx)
	if err != nil {
		t.Fatalf("Ошибка получения токена: %v", err)
	}
	if token2 != "cached-token" {
		t.Errorf("ожидался cached-token, получен %s", token2)
	}

	if tokenRequests != 1 {
		t.Errorf("ожидался 1 запрос токена, было %d", tokenRequests)
	}
}

// TestClient_TokenRefresh проверяет обновление истёкшего токена.
func TestClient_TokenRefresh(t *testing.T) {
	tokenRequests := 0

	_, client := setupMockKeycloak(t,
		func(w http.ResponseWriter, r *http.Request) {
			tokenRequests++
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(TokenResponse{
				AccessToken: "refreshed-token",
				TokenType:   "Bearer",
				ExpiresIn:   300,
			})
		},
		nil,
	)

	// Устанавливаем «просроченный» токен в кэш
	client.accessToken = "old-token"
	client.tokenExpiry = time.Now().Add(-time.Second)

	ctx := context.Background()
	token, err := client.getToken(ctx)
	if err != nil {
		t.Fatalf("Ошибка обновления токена: %v", err)
	}
	if token != "refreshed-token" {
		t.Errorf("ожидался refreshed-token, получен %s", token)
	}
	if tokenRequests != 1 {
		t.Errorf("ожидался 1 запрос токена, было %d", tokenRequests)
	}
}

// TestClient_TokenRefreshBefore30s проверяет обновление за 30 секунд до истечения.
func TestClient_TokenRefreshBefore30s(t *testing.T) {
	tokenRequests := 0

	_, client := setupMockKeycloak(t,
		func(w http.ResponseWriter, r *http.Request) {
			tokenRequests++
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(TokenResponse{
				AccessToken: "new-token",
				TokenType:   "Bearer",
				ExpiresIn:   300,
			})
		},
		nil,
	)

	// Токен истекает через 20 секунд — должен обновиться (< 30s)
	client.accessToken = "expiring-token"
	client.tokenExpiry = time.Now().Add(20 * time.Second)

	ctx := context.Background()
	token, err := client.getToken(ctx)
	if err != nil {
		t.Fatalf("Ошибка обновления токена: %v", err)
	}
	if token != "new-token" {
		t.Errorf("ожидался new-token, получен %s", token)
	}
}

// TestClient_ClientCredentialsFlow проверяет формат запроса Client Credentials.
func TestClient_ClientCredentialsFlow(t *testing.T) {
	_, client := setupMockKeycloak(t,
		func(w http.ResponseWriter, r *http.Request) {
			// Проверяем метод
			if r.Method != http.MethodPost {
				t.Errorf("ожидался POST, получен %s", r.Method)
			}
			// Проверяем Content-Type
			ct := r.Header.Get("Content-Type")
			if ct != "application/x-www-form-urlencoded" {
				t.Errorf("ожидался Content-Type application/x-www-form-urlencoded, получен %s", ct)
			}
			// Проверяем параметры
			if err := r.ParseForm(); err != nil {
				t.Fatalf("Ошибка парсинга формы: %v", err)
			}
			if r.Form.Get("grant_type") != "client_credentials" {
				t.Errorf("ожидался grant_type=client_credentials, получен %s", r.Form.Get("grant_type"))
			}
			if r.Form.Get("client_id") != "admin-module" {
				t.Errorf("ожидался client_id=admin-module, получен %s", r.Form.Get("client_id"))
			}
			if r.Form.Get("client_secret") != "test-secret" {
				t.Errorf("ожидался client_secret=test-secret, получен %s", r.Form.Get("client_secret"))
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(TokenResponse{
				AccessToken: "ok",
				TokenType:   "Bearer",
				ExpiresIn:   300,
			})
		},
		nil,
	)

	_, err := client.getToken(context.Background())
	if err != nil {
		t.Fatalf("Ошибка: %v", err)
	}
}

// TestClient_TokenError проверяет обработку ошибки получения токена.
func TestClient_TokenError(t *testing.T) {
	_, client := setupMockKeycloak(t,
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"invalid_client"}`))
		},
		nil,
	)

	_, err := client.getToken(context.Background())
	if err == nil {
		t.Fatal("ожидалась ошибка, получен nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("ожидалась ошибка со статусом 401, получена: %v", err)
	}
}

// TestClient_ListUsers проверяет ListUsers.
func TestClient_ListUsers(t *testing.T) {
	_, client := setupMockKeycloak(t, nil,
		func(w http.ResponseWriter, r *http.Request) {
			// Проверяем Authorization header
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-access-token" {
				t.Errorf("ожидался Bearer test-access-token, получен %s", auth)
			}

			if strings.HasSuffix(r.URL.Path, "/users") || strings.Contains(r.URL.Path, "/users?") {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]KeycloakUser{
					{ID: "user-1", Username: "admin", Email: "admin@test.com", Enabled: true},
					{ID: "user-2", Username: "viewer", Email: "viewer@test.com", Enabled: true},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		},
	)

	users, err := client.ListUsers(context.Background(), "", 0, 100)
	if err != nil {
		t.Fatalf("Ошибка ListUsers: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("ожидалось 2 пользователя, получено %d", len(users))
	}
	if users[0].Username != "admin" {
		t.Errorf("ожидался username=admin, получен %s", users[0].Username)
	}
}

// TestClient_GetUser проверяет GetUser.
func TestClient_GetUser(t *testing.T) {
	_, client := setupMockKeycloak(t, nil,
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/users/user-123") {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(KeycloakUser{
					ID:       "user-123",
					Username: "admin",
					Email:    "admin@test.com",
					Enabled:  true,
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		},
	)

	user, err := client.GetUser(context.Background(), "user-123")
	if err != nil {
		t.Fatalf("Ошибка GetUser: %v", err)
	}
	if user.ID != "user-123" {
		t.Errorf("ожидался ID=user-123, получен %s", user.ID)
	}
}

// TestClient_GetUserGroups проверяет GetUserGroups.
func TestClient_GetUserGroups(t *testing.T) {
	_, client := setupMockKeycloak(t, nil,
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/users/user-123/groups") {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]KeycloakGroup{
					{ID: "g-1", Name: "artsore-admins", Path: "/artsore-admins"},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		},
	)

	groups, err := client.GetUserGroups(context.Background(), "user-123")
	if err != nil {
		t.Fatalf("Ошибка GetUserGroups: %v", err)
	}
	if len(groups) != 1 {
		t.Errorf("ожидалась 1 группа, получено %d", len(groups))
	}
	if groups[0].Name != "artsore-admins" {
		t.Errorf("ожидалось имя artsore-admins, получено %s", groups[0].Name)
	}
}

// TestClient_CreateClient проверяет CreateClient.
func TestClient_CreateClient(t *testing.T) {
	_, client := setupMockKeycloak(t, nil,
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/clients") {
				// Проверяем тело запроса
				var req clientCreateRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatalf("Ошибка декодирования: %v", err)
				}
				if req.ClientID != "sa_test_abc" {
					t.Errorf("ожидался clientId=sa_test_abc, получен %s", req.ClientID)
				}
				if !req.ServiceAccountsEnabled {
					t.Error("ожидался serviceAccountsEnabled=true")
				}

				w.Header().Set("Location", "https://keycloak/admin/realms/artsore/clients/kc-internal-id")
				w.WriteHeader(http.StatusCreated)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		},
	)

	id, err := client.CreateClient(context.Background(), "sa_test_abc", "Test SA", "Тестовый SA", []string{"files:read"})
	if err != nil {
		t.Fatalf("Ошибка CreateClient: %v", err)
	}
	if id != "kc-internal-id" {
		t.Errorf("ожидался ID=kc-internal-id, получен %s", id)
	}
}

// TestClient_DeleteClient проверяет DeleteClient.
func TestClient_DeleteClient(t *testing.T) {
	_, client := setupMockKeycloak(t, nil,
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/clients/kc-id") {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		},
	)

	err := client.DeleteClient(context.Background(), "kc-id")
	if err != nil {
		t.Fatalf("Ошибка DeleteClient: %v", err)
	}
}

// TestClient_GetClientSecret проверяет GetClientSecret.
func TestClient_GetClientSecret(t *testing.T) {
	_, client := setupMockKeycloak(t, nil,
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/client-secret") {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(KeycloakClientSecret{
					Type:  "secret",
					Value: "super-secret-123",
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		},
	)

	secret, err := client.GetClientSecret(context.Background(), "kc-id")
	if err != nil {
		t.Fatalf("Ошибка GetClientSecret: %v", err)
	}
	if secret != "super-secret-123" {
		t.Errorf("ожидался super-secret-123, получен %s", secret)
	}
}

// TestClient_RegenerateClientSecret проверяет RegenerateClientSecret.
func TestClient_RegenerateClientSecret(t *testing.T) {
	_, client := setupMockKeycloak(t, nil,
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/client-secret") {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(KeycloakClientSecret{
					Type:  "secret",
					Value: "new-secret-456",
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		},
	)

	secret, err := client.RegenerateClientSecret(context.Background(), "kc-id")
	if err != nil {
		t.Fatalf("Ошибка RegenerateClientSecret: %v", err)
	}
	if secret != "new-secret-456" {
		t.Errorf("ожидался new-secret-456, получен %s", secret)
	}
}

// TestClient_RealmInfo проверяет RealmInfo.
func TestClient_RealmInfo(t *testing.T) {
	_, client := setupMockKeycloak(t, nil,
		func(w http.ResponseWriter, r *http.Request) {
			// Realm info запрос идёт к /admin/realms/artsore (без доп. пути)
			path := strings.TrimPrefix(r.URL.Path, "/admin/realms/artsore")
			if path == "" || path == "/" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(RealmRepresentation{
					Realm:   "artsore",
					Enabled: true,
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		},
	)

	realm, err := client.RealmInfo(context.Background())
	if err != nil {
		t.Fatalf("Ошибка RealmInfo: %v", err)
	}
	if realm.Realm != "artsore" {
		t.Errorf("ожидался realm=artsore, получен %s", realm.Realm)
	}
	if !realm.Enabled {
		t.Error("ожидался enabled=true")
	}
}

// TestClient_CheckReady проверяет CheckReady.
func TestClient_CheckReady(t *testing.T) {
	_, client := setupMockKeycloak(t, nil,
		func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimPrefix(r.URL.Path, "/admin/realms/artsore")
			if path == "" || path == "/" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(RealmRepresentation{
					Realm:   "artsore",
					Enabled: true,
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		},
	)

	status, msg := client.CheckReady()
	if status != "ok" {
		t.Errorf("ожидался status=ok, получен %s: %s", status, msg)
	}
}

// TestClient_CheckReady_Fail проверяет CheckReady при недоступности.
func TestClient_CheckReady_Fail(t *testing.T) {
	client := New(
		"http://localhost:1", // Несуществующий адрес
		"artsore",
		"admin-module",
		"secret",
		&http.Client{Timeout: 100 * time.Millisecond},
		testLogger(),
	)

	status, _ := client.CheckReady()
	if status != "fail" {
		t.Errorf("ожидался status=fail, получен %s", status)
	}
}

// TestClient_TokenProvider проверяет TokenProvider.
func TestClient_TokenProvider(t *testing.T) {
	_, client := setupMockKeycloak(t, nil, nil)

	provider := client.TokenProvider()
	token, err := provider(context.Background())
	if err != nil {
		t.Fatalf("Ошибка TokenProvider: %v", err)
	}
	if token != "test-access-token" {
		t.Errorf("ожидался test-access-token, получен %s", token)
	}
}

// TestCreatedAtTime проверяет конвертацию timestamp.
func TestCreatedAtTime(t *testing.T) {
	user := &KeycloakUser{
		CreatedAt: 1708617600000, // 2024-02-22T16:00:00Z в миллисекундах
	}
	ts := user.CreatedAtTime()
	if ts.Year() != 2024 || ts.Month() != time.February || ts.Day() != 22 {
		t.Errorf("неожиданная дата: %v", ts)
	}
}

// TestClient_ListClients проверяет ListClients с фильтром.
func TestClient_ListClients(t *testing.T) {
	_, client := setupMockKeycloak(t, nil,
		func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/clients") && r.Method == http.MethodGet {
				// Проверяем фильтр
				q := r.URL.Query()
				clientID := q.Get("clientId")
				if clientID != "sa_" {
					t.Errorf("ожидался фильтр clientId=sa_, получен %s", clientID)
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]KeycloakClient{
					{ID: "c-1", ClientID: "sa_ingester_abc", Enabled: true, ServiceAccountsEnabled: true},
				})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		},
	)

	clients, err := client.ListClients(context.Background(), "sa_", 0, 100)
	if err != nil {
		t.Fatalf("Ошибка ListClients: %v", err)
	}
	if len(clients) != 1 {
		t.Errorf("ожидался 1 клиент, получено %d", len(clients))
	}
}
