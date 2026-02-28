// Пакет config — загрузка и валидация конфигурации Query Module
// из переменных окружения.
package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Версия приложения, задаётся при сборке через -ldflags.
var Version = "dev"

// Config содержит все параметры конфигурации Query Module.
type Config struct {
	// --- Сервер ---

	// Порт HTTP-сервера (диапазон 8030-8039)
	Port int
	// Уровень логирования (debug, info, warn, error)
	LogLevel slog.Level
	// Формат логов (json, text)
	LogFormat string

	// --- PostgreSQL ---

	// Хост PostgreSQL
	DBHost string
	// Порт PostgreSQL
	DBPort int
	// Имя базы данных
	DBName string
	// Имя пользователя PostgreSQL
	DBUser string
	// Пароль пользователя PostgreSQL
	DBPassword string
	// Режим SSL: disable, require, verify-ca, verify-full
	DBSSLMode string
	// Максимальное количество подключений в пуле (по умолчанию 10)
	DBMaxConns int

	// --- TLS ---

	// Путь к CA-сертификату для TLS-соединений (опционально).
	// Используется для JWKS и Admin Module HTTP-клиента.
	CACertPath string

	// --- JWT/JWKS ---

	// URL JWKS endpoint Keycloak
	JWKSURL string
	// Issuer JWT (ожидаемый issuer в токене)
	JWTIssuer string
	// Интервал обновления JWKS-ключей (по умолчанию 15s)
	JWKSRefreshInterval time.Duration
	// Допустимое отклонение времени при проверке JWT (по умолчанию 5s)
	JWTLeeway time.Duration

	// --- Маппинг групп → ролей ---

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

	// Client ID для client_credentials grant
	ClientID string
	// Client Secret для client_credentials grant
	ClientSecret string //nolint:gosec // G117: поле конфигурации, не содержит секрет напрямую

	// --- SE Download ---

	// Таймаут HTTP-клиента для скачивания из SE (по умолчанию 5m)
	SEDownloadTimeout time.Duration
	// Путь к CA-сертификату для соединений к SE (опционально)
	SECACertPath string

	// --- LRU Cache ---

	// TTL кэша метаданных файлов (по умолчанию 5m)
	CacheTTL time.Duration
	// Максимальный размер LRU-кэша (по умолчанию 10000)
	CacheMaxSize int

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
	// Таймаут HTTP-клиента JWKS (fallback → HTTPClientTimeout)
	JWKSClientTimeout time.Duration

	// --- HTTP Server Timeouts ---

	// Таймаут чтения HTTP-сервера (по умолчанию 30s)
	HTTPReadTimeout time.Duration
	// Таймаут записи HTTP-сервера (по умолчанию 60s)
	HTTPWriteTimeout time.Duration
	// Таймаут простоя HTTP-сервера (по умолчанию 120s)
	HTTPIdleTimeout time.Duration

	// --- Graceful shutdown ---

	// Таймаут graceful shutdown HTTP-сервера (по умолчанию 5s)
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

	// QM_PORT — порт HTTP-сервера (по умолчанию 8030)
	cfg.Port, err = getEnvInt("QM_PORT", 8030)
	if err != nil {
		return nil, fmt.Errorf("QM_PORT: %w", err)
	}
	if cfg.Port < 8030 || cfg.Port > 8039 {
		return nil, fmt.Errorf("QM_PORT: значение %d вне допустимого диапазона 8030-8039", cfg.Port)
	}

	// QM_LOG_LEVEL — уровень логирования (по умолчанию info)
	cfg.LogLevel, err = parseLogLevel(getEnvDefault("QM_LOG_LEVEL", "info"))
	if err != nil {
		return nil, fmt.Errorf("QM_LOG_LEVEL: %w", err)
	}

	// QM_LOG_FORMAT — формат логов (по умолчанию json)
	cfg.LogFormat = getEnvDefault("QM_LOG_FORMAT", "json")
	if cfg.LogFormat != "json" && cfg.LogFormat != "text" {
		return nil, fmt.Errorf("QM_LOG_FORMAT: недопустимое значение %q, допустимые: json, text", cfg.LogFormat)
	}

	// --- PostgreSQL ---

	// QM_DB_HOST — обязательный
	cfg.DBHost, err = getEnvRequired("QM_DB_HOST")
	if err != nil {
		return nil, err
	}

	// QM_DB_PORT — порт PostgreSQL (по умолчанию 5432)
	cfg.DBPort, err = getEnvInt("QM_DB_PORT", 5432)
	if err != nil {
		return nil, fmt.Errorf("QM_DB_PORT: %w", err)
	}

	// QM_DB_NAME — обязательный
	cfg.DBName, err = getEnvRequired("QM_DB_NAME")
	if err != nil {
		return nil, err
	}

	// QM_DB_USER — обязательный
	cfg.DBUser, err = getEnvRequired("QM_DB_USER")
	if err != nil {
		return nil, err
	}

	// QM_DB_PASSWORD — обязательный
	cfg.DBPassword, err = getEnvRequired("QM_DB_PASSWORD")
	if err != nil {
		return nil, err
	}

	// QM_DB_SSL_MODE — режим SSL (по умолчанию disable)
	cfg.DBSSLMode = getEnvDefault("QM_DB_SSL_MODE", "disable")
	validSSLModes := map[string]bool{
		"disable": true, "require": true, "verify-ca": true, "verify-full": true,
	}
	if !validSSLModes[cfg.DBSSLMode] {
		return nil, fmt.Errorf("QM_DB_SSL_MODE: недопустимое значение %q, допустимые: disable, require, verify-ca, verify-full", cfg.DBSSLMode)
	}

	// QM_DB_MAX_CONNS — максимальное количество подключений в пуле (по умолчанию 10)
	cfg.DBMaxConns, err = getEnvInt("QM_DB_MAX_CONNS", 10)
	if err != nil {
		return nil, fmt.Errorf("QM_DB_MAX_CONNS: %w", err)
	}
	if cfg.DBMaxConns < 1 || cfg.DBMaxConns > 100 {
		return nil, fmt.Errorf("QM_DB_MAX_CONNS: значение %d вне допустимого диапазона 1-100", cfg.DBMaxConns)
	}

	// --- TLS ---

	// QM_CA_CERT_PATH — путь к CA-сертификату (опционально)
	cfg.CACertPath = getEnvDefault("QM_CA_CERT_PATH", "")

	// --- JWT/JWKS ---

	// QM_JWKS_URL — обязательный (URL JWKS endpoint Keycloak)
	cfg.JWKSURL, err = getEnvRequired("QM_JWKS_URL")
	if err != nil {
		return nil, err
	}

	// QM_JWT_ISSUER — issuer JWT (опционально, авто-вычисляется из JWKS URL)
	cfg.JWTIssuer = getEnvDefault("QM_JWT_ISSUER", "")

	// QM_JWKS_REFRESH_INTERVAL — интервал обновления JWKS-ключей (по умолчанию 15s)
	cfg.JWKSRefreshInterval, err = getEnvDuration("QM_JWKS_REFRESH_INTERVAL", 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("QM_JWKS_REFRESH_INTERVAL: %w", err)
	}
	if cfg.JWKSRefreshInterval <= 0 {
		return nil, fmt.Errorf("QM_JWKS_REFRESH_INTERVAL: значение должно быть > 0")
	}

	// QM_JWT_LEEWAY — допустимое отклонение времени при проверке JWT (по умолчанию 5s)
	cfg.JWTLeeway, err = getEnvDuration("QM_JWT_LEEWAY", 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("QM_JWT_LEEWAY: %w", err)
	}
	if cfg.JWTLeeway < 0 {
		return nil, fmt.Errorf("QM_JWT_LEEWAY: значение должно быть >= 0")
	}

	// --- Маппинг групп → ролей ---

	// QM_ROLE_ADMIN_GROUPS — группы для роли admin (по умолчанию "artstore-admins")
	cfg.RoleAdminGroups = parseCSV(getEnvDefault("QM_ROLE_ADMIN_GROUPS", "artstore-admins"))

	// QM_ROLE_READONLY_GROUPS — группы для роли readonly (по умолчанию "artstore-viewers")
	cfg.RoleReadonlyGroups = parseCSV(getEnvDefault("QM_ROLE_READONLY_GROUPS", "artstore-viewers"))

	// --- Admin Module HTTP-клиент ---

	// QM_ADMIN_URL — URL Admin Module (обязательный)
	cfg.AdminURL, err = getEnvRequired("QM_ADMIN_URL")
	if err != nil {
		return nil, err
	}
	cfg.AdminURL = strings.TrimRight(cfg.AdminURL, "/")

	// QM_ADMIN_TIMEOUT — таймаут HTTP-клиента Admin Module (по умолчанию 10s)
	cfg.AdminTimeout, err = getEnvDuration("QM_ADMIN_TIMEOUT", 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("QM_ADMIN_TIMEOUT: %w", err)
	}
	if cfg.AdminTimeout <= 0 {
		return nil, fmt.Errorf("QM_ADMIN_TIMEOUT: значение должно быть > 0")
	}

	// --- Keycloak OAuth2 ---

	// QM_CLIENT_ID — Client ID для client_credentials grant (обязательный)
	cfg.ClientID, err = getEnvRequired("QM_CLIENT_ID")
	if err != nil {
		return nil, err
	}

	// QM_CLIENT_SECRET — Client Secret (обязательный)
	cfg.ClientSecret, err = getEnvRequired("QM_CLIENT_SECRET")
	if err != nil {
		return nil, err
	}

	// --- SE Download ---

	// QM_SE_DOWNLOAD_TIMEOUT — таймаут скачивания из SE (по умолчанию 5m)
	cfg.SEDownloadTimeout, err = getEnvDuration("QM_SE_DOWNLOAD_TIMEOUT", 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("QM_SE_DOWNLOAD_TIMEOUT: %w", err)
	}
	if cfg.SEDownloadTimeout <= 0 {
		return nil, fmt.Errorf("QM_SE_DOWNLOAD_TIMEOUT: значение должно быть > 0")
	}

	// QM_SE_CA_CERT_PATH — путь к CA-сертификату для SE (опционально)
	cfg.SECACertPath = getEnvDefault("QM_SE_CA_CERT_PATH", "")

	// --- LRU Cache ---

	// QM_CACHE_TTL — TTL кэша метаданных (по умолчанию 5m)
	cfg.CacheTTL, err = getEnvDuration("QM_CACHE_TTL", 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("QM_CACHE_TTL: %w", err)
	}
	if cfg.CacheTTL <= 0 {
		return nil, fmt.Errorf("QM_CACHE_TTL: значение должно быть > 0")
	}

	// QM_CACHE_MAX_SIZE — максимальный размер LRU-кэша (по умолчанию 10000)
	cfg.CacheMaxSize, err = getEnvInt("QM_CACHE_MAX_SIZE", 10000)
	if err != nil {
		return nil, fmt.Errorf("QM_CACHE_MAX_SIZE: %w", err)
	}
	if cfg.CacheMaxSize < 1 || cfg.CacheMaxSize > 1000000 {
		return nil, fmt.Errorf("QM_CACHE_MAX_SIZE: значение %d вне допустимого диапазона 1-1000000", cfg.CacheMaxSize)
	}

	// --- Topologymetrics ---

	// QM_DEPHEALTH_CHECK_INTERVAL — интервал проверки зависимостей (по умолчанию 15s)
	cfg.DephealthCheckInterval, err = getEnvDuration("QM_DEPHEALTH_CHECK_INTERVAL", 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("QM_DEPHEALTH_CHECK_INTERVAL: %w", err)
	}

	// QM_DEPHEALTH_GROUP — имя группы (по умолчанию "query-module")
	cfg.DephealthGroup = getEnvDefault("QM_DEPHEALTH_GROUP", "query-module")

	// DEPHEALTH_NAME — имя владельца пода (без префикса модуля)
	cfg.DephealthName = getEnvDefault("DEPHEALTH_NAME", "")

	// DEPHEALTH_ISENTRY — при true добавляет лейбл isentry=yes (по умолчанию false)
	cfg.DephealthIsEntry, err = getEnvBool("DEPHEALTH_ISENTRY", false)
	if err != nil {
		return nil, fmt.Errorf("DEPHEALTH_ISENTRY: %w", err)
	}

	// --- HTTP Client Timeouts ---

	// QM_HTTP_CLIENT_TIMEOUT — глобальный таймаут HTTP-клиентов (по умолчанию 30s)
	cfg.HTTPClientTimeout, err = getEnvDuration("QM_HTTP_CLIENT_TIMEOUT", 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("QM_HTTP_CLIENT_TIMEOUT: %w", err)
	}
	if cfg.HTTPClientTimeout <= 0 {
		return nil, fmt.Errorf("QM_HTTP_CLIENT_TIMEOUT: значение должно быть > 0")
	}

	// QM_JWKS_CLIENT_TIMEOUT — таймаут HTTP-клиента JWKS (fallback → HTTPClientTimeout)
	cfg.JWKSClientTimeout, err = getEnvDurationFallback("QM_JWKS_CLIENT_TIMEOUT", cfg.HTTPClientTimeout)
	if err != nil {
		return nil, fmt.Errorf("QM_JWKS_CLIENT_TIMEOUT: %w", err)
	}

	// --- HTTP Server Timeouts ---

	// QM_HTTP_READ_TIMEOUT — таймаут чтения HTTP-сервера (по умолчанию 30s)
	cfg.HTTPReadTimeout, err = getEnvDuration("QM_HTTP_READ_TIMEOUT", 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("QM_HTTP_READ_TIMEOUT: %w", err)
	}
	if cfg.HTTPReadTimeout <= 0 {
		return nil, fmt.Errorf("QM_HTTP_READ_TIMEOUT: значение должно быть > 0")
	}

	// QM_HTTP_WRITE_TIMEOUT — таймаут записи HTTP-сервера (по умолчанию 60s)
	cfg.HTTPWriteTimeout, err = getEnvDuration("QM_HTTP_WRITE_TIMEOUT", 60*time.Second)
	if err != nil {
		return nil, fmt.Errorf("QM_HTTP_WRITE_TIMEOUT: %w", err)
	}
	if cfg.HTTPWriteTimeout <= 0 {
		return nil, fmt.Errorf("QM_HTTP_WRITE_TIMEOUT: значение должно быть > 0")
	}

	// QM_HTTP_IDLE_TIMEOUT — таймаут простоя HTTP-сервера (по умолчанию 120s)
	cfg.HTTPIdleTimeout, err = getEnvDuration("QM_HTTP_IDLE_TIMEOUT", 120*time.Second)
	if err != nil {
		return nil, fmt.Errorf("QM_HTTP_IDLE_TIMEOUT: %w", err)
	}
	if cfg.HTTPIdleTimeout <= 0 {
		return nil, fmt.Errorf("QM_HTTP_IDLE_TIMEOUT: значение должно быть > 0")
	}

	// --- Graceful shutdown ---

	// QM_SHUTDOWN_TIMEOUT — таймаут graceful shutdown (по умолчанию 5s)
	cfg.ShutdownTimeout, err = getEnvDuration("QM_SHUTDOWN_TIMEOUT", 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("QM_SHUTDOWN_TIMEOUT: %w", err)
	}

	return cfg, nil
}

// DatabaseDSN возвращает строку подключения к PostgreSQL (keyword/value формат).
func (c *Config) DatabaseDSN() string {
	return fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s pool_max_conns=%d",
		c.DBHost, c.DBPort, c.DBName, c.DBUser, c.DBPassword, c.DBSSLMode, c.DBMaxConns,
	)
}

// DatabaseURL возвращает URL подключения к PostgreSQL (для golang-migrate и topologymetrics).
func (c *Config) DatabaseURL() string {
	u := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.DBUser, c.DBPassword),
		Host:     fmt.Sprintf("%s:%d", c.DBHost, c.DBPort),
		Path:     c.DBName,
		RawQuery: fmt.Sprintf("sslmode=%s", c.DBSSLMode),
	}
	return u.String()
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
