// JWKS Mock Server — минималистичный сервис для тестовой среды SE.
// Имитирует JWKS endpoint Admin Module: генерирует RSA ключевую пару при старте,
// отдаёт JWKS по GET /jwks и подписывает JWT по POST /token.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// --- Конфигурация ---

// config хранит конфигурацию сервиса из env-переменных.
type config struct {
	Port    string // MOCK_PORT — порт HTTP-сервера (default: 8080)
	TLSCert string // MOCK_TLS_CERT — путь к TLS сертификату (пусто — HTTP)
	TLSKey  string // MOCK_TLS_KEY — путь к TLS приватному ключу (пусто — HTTP)
	KeySize int    // MOCK_KEY_SIZE — размер RSA ключа (default: 2048)
}

// loadConfig загружает конфигурацию из переменных окружения.
func loadConfig() config {
	cfg := config{
		Port:    envOrDefault("MOCK_PORT", "8080"),
		TLSCert: os.Getenv("MOCK_TLS_CERT"),
		TLSKey:  os.Getenv("MOCK_TLS_KEY"),
		KeySize: 2048,
	}

	if v := os.Getenv("MOCK_KEY_SIZE"); v != "" {
		if size, err := strconv.Atoi(v); err == nil && size >= 1024 {
			cfg.KeySize = size
		}
	}

	return cfg
}

// envOrDefault возвращает значение env-переменной или default.
func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// --- JWKS ---

// jwksKey представляет один ключ в JWKS (RFC 7517).
type jwksKey struct {
	Kty string `json:"kty"` // Тип ключа (RSA)
	Kid string `json:"kid"` // Идентификатор ключа
	Use string `json:"use"` // Назначение (sig)
	Alg string `json:"alg"` // Алгоритм (RS256)
	N   string `json:"n"`   // Модуль RSA (base64url)
	E   string `json:"e"`   // Экспонента RSA (base64url)
}

// jwksResponse — ответ GET /jwks.
type jwksResponse struct {
	Keys []jwksKey `json:"keys"`
}

// buildJWKS формирует JWKS ответ из публичного RSA ключа.
func buildJWKS(pub *rsa.PublicKey) jwksResponse {
	return jwksResponse{
		Keys: []jwksKey{
			{
				Kty: "RSA",
				Kid: "test-key-1",
				Use: "sig",
				Alg: "RS256",
				N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			},
		},
	}
}

// --- Token ---

// tokenRequest — тело запроса POST /token.
type tokenRequest struct {
	Sub        string   `json:"sub"`         // Subject (обязательно)
	Scopes     []string `json:"scopes"`      // Массив scope'ов (обязательно)
	TTLSeconds int      `json:"ttl_seconds"` // Время жизни токена в секундах (default: 3600)
}

// tokenResponse — ответ POST /token.
type tokenResponse struct {
	Token string `json:"token"`
}

// mockClaims — JWT claims, совместимые с SE auth middleware.
type mockClaims struct {
	jwt.RegisteredClaims
	Scopes []string `json:"scopes"`
}

// --- Handlers ---

// server объединяет состояние сервиса: RSA ключ и JWKS.
type server struct {
	privateKey   *rsa.PrivateKey
	jwksResponse []byte // Кэшированный JSON JWKS ответ
	logger       *slog.Logger
}

// handleJWKS обрабатывает GET /jwks — возвращает JWKS с публичным ключом.
func (s *server) handleJWKS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(s.jwksResponse)
}

// handleToken обрабатывает POST /token — генерирует подписанный JWT.
func (s *server) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Парсинг запроса
	var req tokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Невалидный JSON: "+err.Error())
		return
	}

	// Валидация обязательных полей
	if req.Sub == "" {
		writeError(w, http.StatusBadRequest, "Поле 'sub' обязательно")
		return
	}
	if len(req.Scopes) == 0 {
		writeError(w, http.StatusBadRequest, "Поле 'scopes' обязательно (массив строк)")
		return
	}

	// TTL по умолчанию — 3600 секунд (1 час)
	ttl := req.TTLSeconds
	if ttl <= 0 {
		ttl = 3600
	}

	now := time.Now()

	// Формируем JWT claims, совместимые с SE auth middleware
	claims := mockClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   req.Sub,
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(ttl) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "jwks-mock",
		},
		Scopes: req.Scopes,
	}

	// Подписываем JWT (RS256, kid=test-key-1)
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-key-1"

	tokenString, err := token.SignedString(s.privateKey)
	if err != nil {
		s.logger.Error("Ошибка подписи JWT", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "Ошибка генерации токена")
		return
	}

	s.logger.Info("Токен выдан",
		slog.String("sub", req.Sub),
		slog.Int("scopes_count", len(req.Scopes)),
		slog.Int("ttl_seconds", ttl),
	)

	// Ответ
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(tokenResponse{Token: tokenString})
}

// handleHealth обрабатывает GET /health — проверка готовности сервиса.
func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// writeError отправляет JSON-ошибку клиенту.
func writeError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}

// --- Main ---

func main() {
	// Загрузка конфигурации
	cfg := loadConfig()

	// Настройка логгера (JSON)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Генерация RSA ключевой пары
	logger.Info("Генерация RSA ключевой пары", slog.Int("key_size", cfg.KeySize))
	privateKey, err := rsa.GenerateKey(rand.Reader, cfg.KeySize)
	if err != nil {
		logger.Error("Ошибка генерации RSA ключа", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Формируем и кэшируем JWKS ответ
	jwks := buildJWKS(&privateKey.PublicKey)
	jwksBytes, err := json.Marshal(jwks)
	if err != nil {
		logger.Error("Ошибка сериализации JWKS", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Создаём сервер
	srv := &server{
		privateKey:   privateKey,
		jwksResponse: jwksBytes,
		logger:       logger,
	}

	// Маршруты
	mux := http.NewServeMux()
	mux.HandleFunc("/jwks", srv.handleJWKS)
	mux.HandleFunc("/token", srv.handleToken)
	mux.HandleFunc("/health", srv.handleHealth)

	addr := ":" + cfg.Port

	// Запуск: TLS или HTTP
	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		logger.Info("Запуск JWKS Mock Server (HTTPS)",
			slog.String("addr", addr),
			slog.String("tls_cert", cfg.TLSCert),
		)
		if err := http.ListenAndServeTLS(addr, cfg.TLSCert, cfg.TLSKey, mux); err != nil {
			logger.Error("Ошибка сервера", slog.String("error", err.Error()))
			os.Exit(1)
		}
	} else {
		logger.Info("Запуск JWKS Mock Server (HTTP)", slog.String("addr", addr))
		fmt.Fprintf(os.Stderr, "ВНИМАНИЕ: TLS не настроен, работаем по HTTP\n")
		if err := http.ListenAndServe(addr, mux); err != nil {
			logger.Error("Ошибка сервера", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}
}
