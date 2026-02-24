package replica

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// mockRoleProvider — mock реализация RoleProvider для тестов.
type mockRoleProvider struct {
	role       Role
	leaderAddr string
}

func (m *mockRoleProvider) CurrentRole() Role   { return m.role }
func (m *mockRoleProvider) IsLeader() bool       { return m.role == RoleLeader }
func (m *mockRoleProvider) LeaderAddr() string   { return m.leaderAddr }

// localHandler — обработчик, который возвращает "local" в теле ответа.
func localHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("local"))
	})
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// TestProxy_LeaderPassesThrough — leader обрабатывает запросы локально.
func TestProxy_LeaderPassesThrough(t *testing.T) {
	provider := &mockRoleProvider{role: RoleLeader, leaderAddr: "leader:8010"}
	proxy := NewLeaderProxy(provider, true, testLogger())

	handler := proxy.Middleware(localHandler())

	// POST запрос — leader обрабатывает локально
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/upload", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Ожидался статус 200, получен %d", rec.Code)
	}

	body := rec.Body.String()
	if body != "local" {
		t.Errorf("Ожидался ответ 'local', получен %q", body)
	}
}

// TestProxy_FollowerGETLocal — follower обрабатывает GET запросы локально.
func TestProxy_FollowerGETLocal(t *testing.T) {
	provider := &mockRoleProvider{role: RoleFollower, leaderAddr: "leader:8010"}
	proxy := NewLeaderProxy(provider, true, testLogger())

	handler := proxy.Middleware(localHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Ожидался статус 200, получен %d", rec.Code)
	}

	body := rec.Body.String()
	if body != "local" {
		t.Errorf("Ожидался ответ 'local', получен %q", body)
	}
}

// TestProxy_FollowerPOSTProxy — follower проксирует POST к leader.
func TestProxy_FollowerPOSTProxy(t *testing.T) {
	// Запускаем mock leader сервер
	leaderServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("from-leader"))
	}))
	defer leaderServer.Close()

	// Извлекаем host из URL (без scheme)
	leaderAddr := strings.TrimPrefix(leaderServer.URL, "https://")

	provider := &mockRoleProvider{role: RoleFollower, leaderAddr: leaderAddr}
	proxy := NewLeaderProxy(provider, true, testLogger())

	handler := proxy.Middleware(localHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/upload", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Ожидался статус 201, получен %d", rec.Code)
	}

	body, _ := io.ReadAll(rec.Result().Body)
	if string(body) != "from-leader" {
		t.Errorf("Ожидался ответ 'from-leader', получен %q", string(body))
	}
}

// TestProxy_FollowerLeaderUnknown — follower с неизвестным leader возвращает 503.
func TestProxy_FollowerLeaderUnknown(t *testing.T) {
	provider := &mockRoleProvider{role: RoleFollower, leaderAddr: ""}
	proxy := NewLeaderProxy(provider, true, testLogger())

	handler := proxy.Middleware(localHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mode/transition", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Ожидался статус 503, получен %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, CodeLeaderUnknown) {
		t.Errorf("Ожидался код ошибки %q в ответе, получен %q", CodeLeaderUnknown, body)
	}
}

// TestProxy_FollowerDELETEProxy — follower проксирует DELETE к leader.
func TestProxy_FollowerDELETEProxy(t *testing.T) {
	leaderServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Ожидался DELETE, получен %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer leaderServer.Close()

	leaderAddr := strings.TrimPrefix(leaderServer.URL, "https://")

	provider := &mockRoleProvider{role: RoleFollower, leaderAddr: leaderAddr}
	proxy := NewLeaderProxy(provider, true, testLogger())

	handler := proxy.Middleware(localHandler())

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/files/123", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("Ожидался статус 204, получен %d", rec.Code)
	}
}
