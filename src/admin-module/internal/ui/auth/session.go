// Пакет auth — аутентификация и управление сессиями Admin UI.
// Шифрование сессий AES-256-GCM, OIDC-клиент для Keycloak (PKCE).
package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Имя cookie для зашифрованной сессии UI.
const SessionCookieName = "artstore_session"

// Максимальный возраст cookie сессии (24 часа).
const SessionCookieMaxAge = 24 * 60 * 60

// SessionData — данные сессии Admin UI, хранящиеся в зашифрованном cookie.
type SessionData struct {
	// AccessToken — JWT access token от Keycloak.
	AccessToken string `json:"access_token"`
	// RefreshToken — refresh token для обновления access token.
	RefreshToken string `json:"refresh_token"`
	// ExpiresAt — время истечения access token (Unix timestamp).
	ExpiresAt int64 `json:"expires_at"`
	// Username — preferred_username из JWT.
	Username string `json:"username"`
	// Email — email пользователя из JWT.
	Email string `json:"email"`
	// Role — effective роль пользователя (admin, readonly).
	Role string `json:"role"`
	// Groups — группы пользователя из JWT.
	Groups []string `json:"groups,omitempty"`
}

// IsExpired проверяет, истёк ли access token.
// Возвращает true если до истечения менее 30 секунд (буфер для refresh).
func (s *SessionData) IsExpired() bool {
	return time.Now().Unix() >= s.ExpiresAt-30
}

// SessionManager — менеджер сессий Admin UI.
// Шифрует/дешифрует SessionData в HTTP cookies через AES-256-GCM.
type SessionManager struct {
	// gcm — AEAD cipher для шифрования/дешифрования.
	gcm cipher.AEAD
	// secure — использовать Secure flag для cookie (true для HTTPS).
	secure bool
}

// NewSessionManager создаёт новый менеджер сессий.
// key — 32-байтовый ключ для AES-256-GCM.
// Если key пустой — генерируется случайный ключ (непостоянный между рестартами).
func NewSessionManager(key string, secure bool) (*SessionManager, error) {
	var keyBytes []byte

	if key == "" {
		// Автогенерация ключа (32 bytes = AES-256)
		keyBytes = make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, keyBytes); err != nil {
			return nil, fmt.Errorf("ошибка генерации ключа сессии: %w", err)
		}
	} else {
		// Декодируем base64-ключ или используем как raw bytes
		var err error
		keyBytes, err = base64.StdEncoding.DecodeString(key)
		if err != nil || len(keyBytes) != 32 {
			// Если не base64 — хешируем строку до 32 bytes через SHA-256
			// (для удобства конфигурации)
			keyBytes = sha256Key(key)
		}
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания GCM: %w", err)
	}

	return &SessionManager{
		gcm:    gcm,
		secure: secure,
	}, nil
}

// Encrypt шифрует SessionData и возвращает base64-строку.
func (sm *SessionManager) Encrypt(data *SessionData) (string, error) {
	plaintext, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("ошибка сериализации сессии: %w", err)
	}

	// Генерируем уникальный nonce для каждого шифрования
	nonce := make([]byte, sm.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("ошибка генерации nonce: %w", err)
	}

	// Шифруем с аутентификацией (nonce prepended к ciphertext)
	ciphertext := sm.gcm.Seal(nonce, nonce, plaintext, nil)

	return base64.URLEncoding.EncodeToString(ciphertext), nil
}

// Decrypt дешифрует base64-строку обратно в SessionData.
func (sm *SessionManager) Decrypt(encrypted string) (*SessionData, error) {
	ciphertext, err := base64.URLEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, fmt.Errorf("ошибка декодирования base64: %w", err)
	}

	nonceSize := sm.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("зашифрованные данные слишком короткие")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := sm.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка дешифрования сессии: %w", err)
	}

	var data SessionData
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return nil, fmt.Errorf("ошибка десериализации сессии: %w", err)
	}

	return &data, nil
}

// SetSessionCookie устанавливает зашифрованный session cookie в ответ.
func (sm *SessionManager) SetSessionCookie(w http.ResponseWriter, data *SessionData) error {
	encrypted, err := sm.Encrypt(data)
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    encrypted,
		Path:     "/admin",
		MaxAge:   SessionCookieMaxAge,
		HttpOnly: true,
		Secure:   sm.secure,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// GetSessionFromRequest извлекает и дешифрует SessionData из cookie запроса.
// Возвращает nil, nil если cookie отсутствует.
func (sm *SessionManager) GetSessionFromRequest(r *http.Request) (*SessionData, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return nil, nil
		}
		return nil, err
	}

	return sm.Decrypt(cookie.Value)
}

// ClearSessionCookie удаляет session cookie из ответа (logout).
func (sm *SessionManager) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/admin",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   sm.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// sha256Key хеширует строковый ключ в 32 bytes через SHA-256.
func sha256Key(key string) []byte {
	h := sha256.Sum256([]byte(key))
	return h[:]
}
