// Пакет config — загрузка и валидация конфигурации Ingester Module
// из переменных окружения.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Версия приложения, задаётся при сборке через -ldflags.
var Version = "dev"

// Config содержит все параметры конфигурации Ingester Module.
// IM не имеет собственной БД (stateless, вся регистрация через Admin Module API).
type Config struct {
	// --- Сервер ---

	// Порт HTTP-сервера (диапазон 8020-8029)
	Port int
	// Уровень логирования (debug, info, warn, error)
	LogLevel slog.Level
	// Формат логов (json, text)
	LogFormat string

	// --- TLS ---

	// Путь к CA-сертификату для TLS-соединений (опционально).
	// Используется для JWKS и Admin Module HTTP-клиента.
	CACertPath string
	// Пропускать проверку TLS-сертификатов при health check (только для dev!)
	TLSSkipVerify bool

	// --- JWT/JWKS ---

	// URL JWKS endpoint Keycloak
	JWKSURL string
	// Issuer JWT (ожидаемый issuer в токене)
	JWTIssuer string
	// Интервал обновления JWKS-ключей (по умолчанию 15s)
	JWKSRefreshInterval time.Duration
	// Допустимое отклонение времени при проверке JWT (по умолчанию 5s)
	JWTLeeway time.Duration

	// --- Маппинг групп -> ролей ---

	// Группы Keycloak, дающие роль admin (через запятую)
	RoleAdminGroups []string
	// Группы Keycloak, дающие роль readonly (через запятую)
	RoleReadonlyGroups []string

	// --- Admin Module HTTP-клиент ---

	// URL Admin Module (например, http://admin-module:8000)
	AdminURL string
	// Таймаут HTTP-клиента Admin Module (по умолчанию 10s)
	AdminTimeout time.Duration

	// --- Keycloak OAuth2 (Client Credentials для SA) ---

	// URL Keycloak token endpoint для client_credentials grant
	// (например, http://keycloak:8080/realms/artstore/protocol/openid-connect/token)
	TokenURL string
	// Client ID для client_credentials grant
	ClientID string
	// Client Secret для client_credentials grant
	ClientSecret string //nolint:gosec // G117: поле конфигурации, не содержит секрет напрямую

	// --- SE Upload ---

	// Таймаут HTTP-клиента для загрузки в SE (по умолчанию 10m)
	SEUploadTimeout time.Duration
	// Путь к CA-сертификату для соединений к SE (опционально)
	SECACertPath string

	// --- Upload ---

	// Максимальный размер файла в байтах (по умолчанию 1073741824 = 1GB)
	MaxFileSize int64
	// TTL по умолчанию для temporary файлов в днях (по умолчанию 30)
	DefaultTTLDays int
	// Максимальное количество retry при 507 (по умолчанию 3)
	MaxRetries int

	// --- Topologymetrics ---

	// Интервал проверки зависимостей topologymetrics (по умолчанию 15s)
	DephealthCheckInterval time.Duration
	// Имя группы в метриках topologymetrics
	DephealthGroup string
	// Имя владельца пода для метки name в topologymetrics
	DephealthName string
	// Флаг isEntry: при true добавляет лейбл isentry=yes
	DephealthIsEntry bool

	// --- HTTP Client Timeouts ---

	// Глобальный таймаут HTTP-клиентов (по умолчанию 30s)
	HTTPClientTimeout time.Duration
	// Таймаут HTTP-клиента JWKS (fallback -> HTTPClientTimeout)
	JWKSClientTimeout time.Duration

	// --- HTTP Server Timeouts ---

	// Таймаут чтения HTTP-сервера (по умолчанию 30s)
	HTTPReadTimeout time.Duration
	// Таймаут записи HTTP-сервера (по умолчанию 600s — увеличен для upload)
	HTTPWriteTimeout time.Duration
	// Таймаут простоя HTTP-сервера (по умолчанию 120s)
	HTTPIdleTimeout time.Duration

	// --- Graceful shutdown ---

	// Таймаут graceful shutdown HTTP-сервера (по умолчанию 10s)
	ShutdownTimeout time.Duration
}

// Load загружает конфигурацию из переменных окружения, валидирует
// обязательные поля и возвращает Config или ошибку.
//
//nolint:cyclop,gocognit // единая функция загрузки конфигурации
func Load() (*Config, error) {
	cfg := &Config{}
	var err error

	// --- Сервер ---

	// IM_PORT — порт HTTP-сервера (по умолчанию 8020)
	cfg.Port, err = getEnvInt("IM_PORT", 8020)
	if err != nil {
		return nil, fmt.Errorf("IM_PORT: %w", err)
	}
	if cfg.Port < 8020 || cfg.Port > 8029 {
		return nil, fmt.Errorf("IM_PORT: значение %d вне допустимого диапазона 8020-8029", cfg.Port)
	}

	// IM_LOG_LEVEL — уровень логирования (по умолчанию info)
	cfg.LogLevel, err = parseLogLevel(getEnvDefault("IM_LOG_LEVEL", "info"))
	if err != nil {
		return nil, fmt.Errorf("IM_LOG_LEVEL: %w", err)
	}

	// IM_LOG_FORMAT — формат логов (по умолчанию json)
	cfg.LogFormat = getEnvDefault("IM_LOG_FORMAT", "json")
	if cfg.LogFormat != "json" && cfg.LogFormat != "text" {
		return nil, fmt.Errorf("IM_LOG_FORMAT: недопустимое значение %q, допустимые: json, text", cfg.LogFormat)
	}

	// --- TLS ---

	// IM_CA_CERT_PATH — путь к CA-сертификату (опционально)
	cfg.CACertPath = getEnvDefault("IM_CA_CERT_PATH", "")

	// IM_TLS_SKIP_VERIFY — пропускать TLS-проверку (по умолчанию false, только для dev)
	cfg.TLSSkipVerify, err = getEnvBool("IM_TLS_SKIP_VERIFY", false)
	if err != nil {
		return nil, fmt.Errorf("IM_TLS_SKIP_VERIFY: %w", err)
	}

	// --- JWT/JWKS ---

	// IM_JWKS_URL — URL JWKS endpoint Keycloak (обязательный)
	cfg.JWKSURL, err = getEnvRequired("IM_JWKS_URL")
	if err != nil {
		return nil, err
	}

	// IM_JWT_ISSUER — issuer JWT (опционально)
	cfg.JWTIssuer = getEnvDefault("IM_JWT_ISSUER", "")

	// IM_JWKS_REFRESH_INTERVAL — интервал обновления JWKS-ключей (по умолчанию 15s)
	cfg.JWKSRefreshInterval, err = getEnvDuration("IM_JWKS_REFRESH_INTERVAL", 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("IM_JWKS_REFRESH_INTERVAL: %w", err)
	}
	if cfg.JWKSRefreshInterval <= 0 {
		return nil, fmt.Errorf("IM_JWKS_REFRESH_INTERVAL: значение должно быть > 0")
	}

	// IM_JWT_LEEWAY — допустимое отклонение времени при проверке JWT (по умолчанию 5s)
	cfg.JWTLeeway, err = getEnvDuration("IM_JWT_LEEWAY", 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("IM_JWT_LEEWAY: %w", err)
	}
	if cfg.JWTLeeway < 0 {
		return nil, fmt.Errorf("IM_JWT_LEEWAY: значение должно быть >= 0")
	}

	// --- Маппинг групп -> ролей ---

	// IM_ROLE_ADMIN_GROUPS — группы для роли admin (по умолчанию "artstore-admins")
	cfg.RoleAdminGroups = parseCSV(getEnvDefault("IM_ROLE_ADMIN_GROUPS", "artstore-admins"))

	// IM_ROLE_READONLY_GROUPS — группы для роли readonly (по умолчанию "artstore-viewers")
	cfg.RoleReadonlyGroups = parseCSV(getEnvDefault("IM_ROLE_READONLY_GROUPS", "artstore-viewers"))

	// --- Admin Module HTTP-клиент ---

	// IM_ADMIN_URL — URL Admin Module (обязательный)
	cfg.AdminURL, err = getEnvRequired("IM_ADMIN_URL")
	if err != nil {
		return nil, err
	}
	cfg.AdminURL = strings.TrimRight(cfg.AdminURL, "/")

	// IM_ADMIN_TIMEOUT — таймаут HTTP-клиента Admin Module (по умолчанию 10s)
	cfg.AdminTimeout, err = getEnvDuration("IM_ADMIN_TIMEOUT", 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("IM_ADMIN_TIMEOUT: %w", err)
	}
	if cfg.AdminTimeout <= 0 {
		return nil, fmt.Errorf("IM_ADMIN_TIMEOUT: значение должно быть > 0")
	}

	// --- Keycloak OAuth2 ---

	// IM_TOKEN_URL — URL Keycloak token endpoint (обязательный)
	cfg.TokenURL, err = getEnvRequired("IM_TOKEN_URL")
	if err != nil {
		return nil, err
	}
	cfg.TokenURL = strings.TrimRight(cfg.TokenURL, "/")

	// IM_CLIENT_ID — Client ID для client_credentials grant (обязательный)
	cfg.ClientID, err = getEnvRequired("IM_CLIENT_ID")
	if err != nil {
		return nil, err
	}

	// IM_CLIENT_SECRET — Client Secret (обязательный)
	cfg.ClientSecret, err = getEnvRequired("IM_CLIENT_SECRET")
	if err != nil {
		return nil, err
	}

	// --- SE Upload ---

	// IM_SE_UPLOAD_TIMEOUT — таймаут загрузки в SE (по умолчанию 10m)
	cfg.SEUploadTimeout, err = getEnvDuration("IM_SE_UPLOAD_TIMEOUT", 10*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("IM_SE_UPLOAD_TIMEOUT: %w", err)
	}
	if cfg.SEUploadTimeout <= 0 {
		return nil, fmt.Errorf("IM_SE_UPLOAD_TIMEOUT: значение должно быть > 0")
	}

	// IM_SE_CA_CERT_PATH — путь к CA-сертификату для SE (опционально)
	cfg.SECACertPath = getEnvDefault("IM_SE_CA_CERT_PATH", "")

	// --- Upload ---

	// IM_MAX_FILE_SIZE — максимальный размер файла в байтах (по умолчанию 1GB)
	cfg.MaxFileSize, err = getEnvInt64("IM_MAX_FILE_SIZE", 1073741824)
	if err != nil {
		return nil, fmt.Errorf("IM_MAX_FILE_SIZE: %w", err)
	}
	if cfg.MaxFileSize <= 0 {
		return nil, fmt.Errorf("IM_MAX_FILE_SIZE: значение должно быть > 0")
	}

	// IM_DEFAULT_TTL_DAYS — TTL по умолчанию для temporary файлов (по умолчанию 30)
	cfg.DefaultTTLDays, err = getEnvInt("IM_DEFAULT_TTL_DAYS", 30)
	if err != nil {
		return nil, fmt.Errorf("IM_DEFAULT_TTL_DAYS: %w", err)
	}
	if cfg.DefaultTTLDays < 1 || cfg.DefaultTTLDays > 365 {
		return nil, fmt.Errorf("IM_DEFAULT_TTL_DAYS: значение %d вне допустимого диапазона 1-365", cfg.DefaultTTLDays)
	}

	// IM_MAX_RETRIES — максимальное количество retry при 507 (по умолчанию 3)
	cfg.MaxRetries, err = getEnvInt("IM_MAX_RETRIES", 3)
	if err != nil {
		return nil, fmt.Errorf("IM_MAX_RETRIES: %w", err)
	}
	if cfg.MaxRetries < 0 || cfg.MaxRetries > 10 {
		return nil, fmt.Errorf("IM_MAX_RETRIES: значение %d вне допустимого диапазона 0-10", cfg.MaxRetries)
	}

	// --- Topologymetrics ---

	// IM_DEPHEALTH_CHECK_INTERVAL — интервал проверки зависимостей (по умолчанию 15s)
	cfg.DephealthCheckInterval, err = getEnvDuration("IM_DEPHEALTH_CHECK_INTERVAL", 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("IM_DEPHEALTH_CHECK_INTERVAL: %w", err)
	}

	// IM_DEPHEALTH_GROUP — имя группы (по умолчанию пустая строка)
	cfg.DephealthGroup = getEnvDefault("IM_DEPHEALTH_GROUP", "")

	// DEPHEALTH_NAME — имя владельца пода (без префикса модуля)
	cfg.DephealthName = getEnvDefault("DEPHEALTH_NAME", "ingester-module")

	// DEPHEALTH_ISENTRY — при true добавляет лейбл isentry=yes (по умолчанию false)
	cfg.DephealthIsEntry, err = getEnvBool("DEPHEALTH_ISENTRY", false)
	if err != nil {
		return nil, fmt.Errorf("DEPHEALTH_ISENTRY: %w", err)
	}

	// --- HTTP Client Timeouts ---

	// IM_HTTP_CLIENT_TIMEOUT — глобальный таймаут HTTP-клиентов (по умолчанию 30s)
	cfg.HTTPClientTimeout, err = getEnvDuration("IM_HTTP_CLIENT_TIMEOUT", 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("IM_HTTP_CLIENT_TIMEOUT: %w", err)
	}
	if cfg.HTTPClientTimeout <= 0 {
		return nil, fmt.Errorf("IM_HTTP_CLIENT_TIMEOUT: значение должно быть > 0")
	}

	// IM_JWKS_CLIENT_TIMEOUT — таймаут HTTP-клиента JWKS (fallback -> HTTPClientTimeout)
	cfg.JWKSClientTimeout, err = getEnvDurationFallback("IM_JWKS_CLIENT_TIMEOUT", cfg.HTTPClientTimeout)
	if err != nil {
		return nil, fmt.Errorf("IM_JWKS_CLIENT_TIMEOUT: %w", err)
	}

	// --- HTTP Server Timeouts ---

	// IM_HTTP_READ_TIMEOUT — таймаут чтения HTTP-сервера (по умолчанию 30s)
	cfg.HTTPReadTimeout, err = getEnvDuration("IM_HTTP_READ_TIMEOUT", 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("IM_HTTP_READ_TIMEOUT: %w", err)
	}
	if cfg.HTTPReadTimeout <= 0 {
		return nil, fmt.Errorf("IM_HTTP_READ_TIMEOUT: значение должно быть > 0")
	}

	// IM_HTTP_WRITE_TIMEOUT — таймаут записи HTTP-сервера (по умолчанию 600s, увеличен для upload)
	cfg.HTTPWriteTimeout, err = getEnvDuration("IM_HTTP_WRITE_TIMEOUT", 600*time.Second)
	if err != nil {
		return nil, fmt.Errorf("IM_HTTP_WRITE_TIMEOUT: %w", err)
	}
	if cfg.HTTPWriteTimeout <= 0 {
		return nil, fmt.Errorf("IM_HTTP_WRITE_TIMEOUT: значение должно быть > 0")
	}

	// IM_HTTP_IDLE_TIMEOUT — таймаут простоя HTTP-сервера (по умолчанию 120s)
	cfg.HTTPIdleTimeout, err = getEnvDuration("IM_HTTP_IDLE_TIMEOUT", 120*time.Second)
	if err != nil {
		return nil, fmt.Errorf("IM_HTTP_IDLE_TIMEOUT: %w", err)
	}
	if cfg.HTTPIdleTimeout <= 0 {
		return nil, fmt.Errorf("IM_HTTP_IDLE_TIMEOUT: значение должно быть > 0")
	}

	// --- Graceful shutdown ---

	// IM_SHUTDOWN_TIMEOUT — таймаут graceful shutdown (по умолчанию 10s)
	cfg.ShutdownTimeout, err = getEnvDuration("IM_SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("IM_SHUTDOWN_TIMEOUT: %w", err)
	}

	return cfg, nil
}

// SetupLogger настраивает глобальный slog-логгер на основе конфигурации.
func SetupLogger(cfg *Config) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}

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

// --- Вспомогательные функции ---

// getEnvRequired возвращает значение переменной окружения или ошибку, если она не задана.
func getEnvRequired(key string) (string, error) {
	val := os.Getenv(key)
	if val == "" {
		return "", fmt.Errorf("%s: обязательная переменная окружения не задана", key)
	}
	return val, nil
}

// getEnvDefault возвращает значение переменной окружения или значение по умолчанию.
func getEnvDefault(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}

// getEnvInt возвращает целочисленное значение переменной окружения или значение по умолчанию.
func getEnvInt(key string, defaultVal int) (int, error) {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal, nil
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("некорректное целое число: %q", val)
	}
	return n, nil
}

// getEnvInt64 возвращает int64 из переменной окружения или значение по умолчанию.
func getEnvInt64(key string, defaultVal int64) (int64, error) {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal, nil
	}
	n, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("некорректное целое число: %q", val)
	}
	return n, nil
}

// getEnvDuration возвращает time.Duration из переменной окружения или значение по умолчанию.
func getEnvDuration(key string, defaultVal time.Duration) (time.Duration, error) {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal, nil
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return 0, fmt.Errorf("некорректная длительность: %q (используйте формат Go: 30s, 1h, 15m)", val)
	}
	return d, nil
}

// getEnvDurationFallback возвращает time.Duration из переменной окружения.
// Если переменная не задана, используется fallbackVal (обычно глобальный таймаут).
// Если задана — парсится и валидируется (> 0).
func getEnvDurationFallback(key string, fallbackVal time.Duration) (time.Duration, error) {
	val := os.Getenv(key)
	if val == "" {
		return fallbackVal, nil
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return 0, fmt.Errorf("некорректная длительность: %q (используйте формат Go: 30s, 1h, 15m)", val)
	}
	if d <= 0 {
		return 0, fmt.Errorf("значение должно быть > 0")
	}
	return d, nil
}

// getEnvBool возвращает булево значение переменной окружения или значение по умолчанию.
func getEnvBool(key string, defaultVal bool) (bool, error) {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal, nil
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return false, fmt.Errorf("некорректное булево значение: %q (допустимые: true, false, 1, 0)", val)
	}
	return b, nil
}

// parseLogLevel преобразует строку уровня логирования в slog.Level.
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
		return slog.LevelInfo, fmt.Errorf("недопустимый уровень %q, допустимые: debug, info, warn, error", level)
	}
}

// parseCSV разбирает строку, разделённую запятыми, на срез строк.
// Пробелы вокруг элементов убираются, пустые элементы игнорируются.
func parseCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
