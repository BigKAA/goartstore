// Package config — загрузка конфигурации Demo Client из переменных окружения DC_*.
// Паттерн аналогичен Admin Module.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Version — инъектируется через ldflags при сборке:
// -X github.com/bigkaa/goartstore/demo-client/internal/config.Version=v0.1.0
var Version = "dev"

// Config — конфигурация Demo Client.
//
//nolint:gosec // G101: struct field names, not credentials
type Config struct {
	// --- Сервер ---
	Port             int           `json:"port"`               // DC_PORT (default: 8080)
	LogLevel         slog.Level    `json:"log_level"`          // DC_LOG_LEVEL (default: info)
	LogFormat        string        `json:"log_format"`         // DC_LOG_FORMAT: json/text (default: json)
	ShutdownTimeout  time.Duration `json:"shutdown_timeout"`   // DC_SHUTDOWN_TIMEOUT (default: 15s)
	HTTPReadTimeout  time.Duration `json:"http_read_timeout"`  // DC_HTTP_READ_TIMEOUT (default: 30s)
	HTTPWriteTimeout time.Duration `json:"http_write_timeout"` // DC_HTTP_WRITE_TIMEOUT (default: 60s)
	HTTPIdleTimeout  time.Duration `json:"http_idle_timeout"`  // DC_HTTP_IDLE_TIMEOUT (default: 120s)

	// --- OAuth / Token ---
	TokenURL           string        `json:"token_url"`            // DC_TOKEN_URL (обязательный)
	ClientID           string        `json:"client_id"`            // DC_CLIENT_ID (обязательный)
	ClientSecret       string        `json:"client_secret"`        // DC_CLIENT_SECRET (обязательный)
	Scopes             []string      `json:"scopes"`               // DC_SCOPES (default: "files:read files:write")
	TokenRefreshBefore time.Duration `json:"token_refresh_before"` // DC_TOKEN_REFRESH_BEFORE (default: 30s)

	// --- Gateway ---
	GatewayURL     string        `json:"gateway_url"`     // DC_GATEWAY_URL (обязательный)
	RequestTimeout time.Duration `json:"request_timeout"` // DC_REQUEST_TIMEOUT (default: 30s)
	UploadTimeout  time.Duration `json:"upload_timeout"`  // DC_UPLOAD_TIMEOUT (default: 300s)
	MaxUploadSize  int64         `json:"max_upload_size"` // DC_MAX_UPLOAD_SIZE (default: 1073741824, 1GB)

	// --- TLS ---
	CACertPath string `json:"ca_cert_path"` // DC_CA_CERT_PATH (опционально)

	// --- Activity Log ---
	ActivityLogSize int `json:"activity_log_size"` // DC_ACTIVITY_LOG_SIZE (default: 100)
}

// Load — загрузка конфигурации из переменных окружения DC_*.
// Возвращает ошибку если обязательные переменные не заданы.
func Load() (*Config, error) {
	cfg := &Config{}
	var err error

	// --- Сервер ---
	cfg.Port, err = getEnvInt("DC_PORT", 8080)
	if err != nil {
		return nil, fmt.Errorf("DC_PORT: %w", err)
	}

	cfg.LogLevel, err = parseLogLevel(getEnvDefault("DC_LOG_LEVEL", "info"))
	if err != nil {
		return nil, fmt.Errorf("DC_LOG_LEVEL: %w", err)
	}

	cfg.LogFormat = getEnvDefault("DC_LOG_FORMAT", "json")

	cfg.ShutdownTimeout, err = getEnvDuration("DC_SHUTDOWN_TIMEOUT", 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("DC_SHUTDOWN_TIMEOUT: %w", err)
	}

	cfg.HTTPReadTimeout, err = getEnvDuration("DC_HTTP_READ_TIMEOUT", 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("DC_HTTP_READ_TIMEOUT: %w", err)
	}

	cfg.HTTPWriteTimeout, err = getEnvDuration("DC_HTTP_WRITE_TIMEOUT", 60*time.Second)
	if err != nil {
		return nil, fmt.Errorf("DC_HTTP_WRITE_TIMEOUT: %w", err)
	}

	cfg.HTTPIdleTimeout, err = getEnvDuration("DC_HTTP_IDLE_TIMEOUT", 120*time.Second)
	if err != nil {
		return nil, fmt.Errorf("DC_HTTP_IDLE_TIMEOUT: %w", err)
	}

	// --- OAuth / Token ---
	cfg.TokenURL, err = getEnvRequired("DC_TOKEN_URL")
	if err != nil {
		return nil, err
	}

	cfg.ClientID, err = getEnvRequired("DC_CLIENT_ID")
	if err != nil {
		return nil, err
	}

	cfg.ClientSecret, err = getEnvRequired("DC_CLIENT_SECRET")
	if err != nil {
		return nil, err
	}

	scopesStr := getEnvDefault("DC_SCOPES", "files:read files:write")
	cfg.Scopes = parseSpaceSeparated(scopesStr)

	cfg.TokenRefreshBefore, err = getEnvDuration("DC_TOKEN_REFRESH_BEFORE", 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("DC_TOKEN_REFRESH_BEFORE: %w", err)
	}

	// --- Gateway ---
	cfg.GatewayURL, err = getEnvRequired("DC_GATEWAY_URL")
	if err != nil {
		return nil, err
	}

	cfg.RequestTimeout, err = getEnvDuration("DC_REQUEST_TIMEOUT", 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("DC_REQUEST_TIMEOUT: %w", err)
	}

	cfg.UploadTimeout, err = getEnvDuration("DC_UPLOAD_TIMEOUT", 300*time.Second)
	if err != nil {
		return nil, fmt.Errorf("DC_UPLOAD_TIMEOUT: %w", err)
	}

	cfg.MaxUploadSize, err = getEnvInt64("DC_MAX_UPLOAD_SIZE", 1073741824) // 1GB
	if err != nil {
		return nil, fmt.Errorf("DC_MAX_UPLOAD_SIZE: %w", err)
	}

	// --- TLS ---
	cfg.CACertPath = getEnvDefault("DC_CA_CERT_PATH", "")

	// --- Activity Log ---
	cfg.ActivityLogSize, err = getEnvInt("DC_ACTIVITY_LOG_SIZE", 100)
	if err != nil {
		return nil, fmt.Errorf("DC_ACTIVITY_LOG_SIZE: %w", err)
	}

	return cfg, nil
}

// SetupLogger — настройка глобального slog логгера (JSON или text формат).
func SetupLogger(cfg *Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}

	var handler slog.Handler
	if cfg.LogFormat == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

// --- Вспомогательные функции загрузки env ---

// getEnvRequired — обязательная переменная, ошибка если не задана.
func getEnvRequired(key string) (string, error) {
	val := os.Getenv(key)
	if val == "" {
		return "", fmt.Errorf("обязательная переменная %s не задана", key)
	}
	return val, nil
}

// getEnvDefault — переменная со значением по умолчанию.
func getEnvDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// getEnvInt — целое число из env.
func getEnvInt(key string, defaultVal int) (int, error) {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal, nil
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("некорректное целое число: %s=%q", key, val)
	}
	return n, nil
}

// getEnvInt64 — int64 из env.
func getEnvInt64(key string, defaultVal int64) (int64, error) {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal, nil
	}
	n, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("некорректное целое число: %s=%q", key, val)
	}
	return n, nil
}

// getEnvDuration — time.Duration из env (формат Go: 30s, 1h, 15m).
func getEnvDuration(key string, defaultVal time.Duration) (time.Duration, error) {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal, nil
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return 0, fmt.Errorf("некорректная длительность: %s=%q", key, val)
	}
	return d, nil
}

// parseLogLevel — парсинг уровня логирования из строки.
func parseLogLevel(level string) (slog.Level, error) {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("неизвестный уровень логирования: %q", level)
	}
}

// parseSpaceSeparated — разделение строки по пробелам (trim, пропуск пустых).
func parseSpaceSeparated(s string) []string {
	parts := strings.Fields(s)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			result = append(result, p)
		}
	}
	return result
}
