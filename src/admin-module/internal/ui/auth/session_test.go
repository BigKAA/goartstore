package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestSessionEncryptDecryptRoundTrip проверяет шифрование и дешифрование SessionData.
func TestSessionEncryptDecryptRoundTrip(t *testing.T) {
	sm, err := NewSessionManager("", false)
	if err != nil {
		t.Fatalf("Ошибка создания SessionManager: %v", err)
	}

	original := &SessionData{
		AccessToken:  "test-access-token-12345",
		RefreshToken: "test-refresh-token-67890",
		ExpiresAt:    time.Now().Add(5 * time.Minute).Unix(),
		Username:     "admin",
		Email:        "admin@example.com",
		Role:         "admin",
		Groups:       []string{"artstore-admins"},
	}

	// Шифруем
	encrypted, err := sm.Encrypt(original)
	if err != nil {
		t.Fatalf("Ошибка шифрования: %v", err)
	}

	if encrypted == "" {
		t.Fatal("Зашифрованная строка пустая")
	}

	// Дешифруем
	decrypted, err := sm.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Ошибка дешифрования: %v", err)
	}

	// Сравниваем поля
	if decrypted.AccessToken != original.AccessToken {
		t.Errorf("AccessToken: want %q, got %q", original.AccessToken, decrypted.AccessToken)
	}
	if decrypted.RefreshToken != original.RefreshToken {
		t.Errorf("RefreshToken: want %q, got %q", original.RefreshToken, decrypted.RefreshToken)
	}
	if decrypted.ExpiresAt != original.ExpiresAt {
		t.Errorf("ExpiresAt: want %d, got %d", original.ExpiresAt, decrypted.ExpiresAt)
	}
	if decrypted.Username != original.Username {
		t.Errorf("Username: want %q, got %q", original.Username, decrypted.Username)
	}
	if decrypted.Email != original.Email {
		t.Errorf("Email: want %q, got %q", original.Email, decrypted.Email)
	}
	if decrypted.Role != original.Role {
		t.Errorf("Role: want %q, got %q", original.Role, decrypted.Role)
	}
	if len(decrypted.Groups) != len(original.Groups) {
		t.Errorf("Groups length: want %d, got %d", len(original.Groups), len(decrypted.Groups))
	}
}

// TestSessionManagerWithStringKey проверяет инициализацию с произвольной строкой-ключом.
func TestSessionManagerWithStringKey(t *testing.T) {
	sm, err := NewSessionManager("my-secret-key-for-testing", false)
	if err != nil {
		t.Fatalf("Ошибка создания SessionManager с string-ключом: %v", err)
	}

	data := &SessionData{
		AccessToken: "token123",
		Username:    "user",
		Role:        "readonly",
	}

	encrypted, err := sm.Encrypt(data)
	if err != nil {
		t.Fatalf("Ошибка шифрования: %v", err)
	}

	decrypted, err := sm.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Ошибка дешифрования: %v", err)
	}

	if decrypted.AccessToken != data.AccessToken {
		t.Errorf("AccessToken: want %q, got %q", data.AccessToken, decrypted.AccessToken)
	}
}

// TestSessionDecryptWithWrongKey проверяет, что дешифрование чужим ключом не работает.
func TestSessionDecryptWithWrongKey(t *testing.T) {
	sm1, _ := NewSessionManager("key-one", false)
	sm2, _ := NewSessionManager("key-two", false)

	data := &SessionData{AccessToken: "secret"}
	encrypted, err := sm1.Encrypt(data)
	if err != nil {
		t.Fatalf("Ошибка шифрования: %v", err)
	}

	// Попытка дешифрования другим ключом должна завершиться ошибкой
	_, err = sm2.Decrypt(encrypted)
	if err == nil {
		t.Error("Ожидалась ошибка при дешифровании чужим ключом")
	}
}

// TestSessionIsExpired проверяет логику проверки истечения токена.
func TestSessionIsExpired(t *testing.T) {
	// Токен, истёкший в прошлом
	expired := &SessionData{
		ExpiresAt: time.Now().Add(-1 * time.Minute).Unix(),
	}
	if !expired.IsExpired() {
		t.Error("Ожидалось IsExpired()=true для истёкшего токена")
	}

	// Токен, истекающий через минуту (но буфер 30с — ещё не expired)
	fresh := &SessionData{
		ExpiresAt: time.Now().Add(1 * time.Minute).Unix(),
	}
	if fresh.IsExpired() {
		t.Error("Ожидалось IsExpired()=false для свежего токена")
	}

	// Токен, истекающий через 20 секунд (буфер 30с — expired)
	almostExpired := &SessionData{
		ExpiresAt: time.Now().Add(20 * time.Second).Unix(),
	}
	if !almostExpired.IsExpired() {
		t.Error("Ожидалось IsExpired()=true для токена в буферной зоне")
	}
}

// TestSessionCookieSetAndGet проверяет установку и извлечение cookie.
func TestSessionCookieSetAndGet(t *testing.T) {
	sm, _ := NewSessionManager("test-key", false)

	data := &SessionData{
		AccessToken: "access-123",
		Username:    "admin",
		Role:        "admin",
		ExpiresAt:   time.Now().Add(5 * time.Minute).Unix(),
	}

	// Устанавливаем cookie
	w := httptest.NewRecorder()
	if err := sm.SetSessionCookie(w, data); err != nil {
		t.Fatalf("Ошибка установки cookie: %v", err)
	}

	// Извлекаем cookie из response и создаём request с ним
	resp := w.Result()
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		t.Fatal("Cookie не установлен")
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	req.AddCookie(cookies[0])

	// Читаем сессию из request
	got, err := sm.GetSessionFromRequest(req)
	if err != nil {
		t.Fatalf("Ошибка чтения сессии из cookie: %v", err)
	}
	if got == nil {
		t.Fatal("Сессия не найдена")
	}
	if got.AccessToken != data.AccessToken {
		t.Errorf("AccessToken: want %q, got %q", data.AccessToken, got.AccessToken)
	}
	if got.Username != data.Username {
		t.Errorf("Username: want %q, got %q", data.Username, got.Username)
	}

	// Проверяем атрибуты cookie
	cookie := cookies[0]
	if cookie.Name != SessionCookieName {
		t.Errorf("Cookie name: want %q, got %q", SessionCookieName, cookie.Name)
	}
	if cookie.Path != "/admin" {
		t.Errorf("Cookie path: want %q, got %q", "/admin", cookie.Path)
	}
	if !cookie.HttpOnly {
		t.Error("Cookie должен быть HttpOnly")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Error("Cookie должен быть SameSite=Lax")
	}
}

// TestSessionCookieMissing проверяет, что отсутствие cookie возвращает nil, nil.
func TestSessionCookieMissing(t *testing.T) {
	sm, _ := NewSessionManager("test-key", false)

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	data, err := sm.GetSessionFromRequest(req)
	if err != nil {
		t.Fatalf("Ожидалось nil error, получено: %v", err)
	}
	if data != nil {
		t.Error("Ожидалось nil data при отсутствии cookie")
	}
}

// TestClearSessionCookie проверяет очистку session cookie.
func TestClearSessionCookie(t *testing.T) {
	sm, _ := NewSessionManager("test-key", false)

	w := httptest.NewRecorder()
	sm.ClearSessionCookie(w)

	resp := w.Result()
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		t.Fatal("Cookie очистки не установлен")
	}

	cookie := cookies[0]
	if cookie.MaxAge != -1 {
		t.Errorf("MaxAge: want -1, got %d", cookie.MaxAge)
	}
	if cookie.Value != "" {
		t.Error("Value должен быть пустым")
	}
}
