// Пакет config — загрузка и валидация конфигурации Admin Module
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

// Config содержит все параметры конфигурации Admin Module.
type Config struct {
	// --- Сервер ---

	// Порт HTTP-сервера (диапазон 8000-8009)
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

	// --- Keycloak ---

	// URL Keycloak (например, https://keycloak.kryukov.lan)
	KeycloakURL string
	// Имя realm в Keycloak
	KeycloakRealm string
	// Client ID для доступа к Keycloak Admin API
	KeycloakClientID string
	// Client Secret для доступа к Keycloak Admin API
	KeycloakClientSecret string
	// Префикс client_id для SA (по умолчанию "sa_")
	KeycloakSAPrefix string

	// --- JWT (fallback-валидация, основная на API Gateway) ---

	// Issuer JWT (авто-вычисляется из KeycloakURL, если не задан)
	JWTIssuer string
	// URL JWKS endpoint (авто-вычисляется из KeycloakURL, если не задан)
	JWTJWKSURL string
	// Claim для ролей в JWT
	JWTRolesClaim string
	// Claim для групп в JWT
	JWTGroupsClaim string

	// --- Синхронизация ---

	// Интервал проверки зависимостей topologymetrics
	DephealthCheckInterval time.Duration
	// Интервал синхронизации файлового реестра с SE
	SyncInterval time.Duration
	// Размер страницы при постраничной синхронизации файлов
	SyncPageSize int
	// Интервал синхронизации SA с Keycloak
	SASyncInterval time.Duration
	// Путь к CA-сертификату для TLS-соединений с SE (опционально)
	SECACertPath string

	// --- Маппинг групп → ролей ---

	// Группы Keycloak, дающие роль admin (через запятую)
	RoleAdminGroups []string
	// Группы Keycloak, дающие роль readonly (через запятую)
	RoleReadonlyGroups []string

	// --- Graceful shutdown ---

	// Таймаут graceful shutdown HTTP-сервера
	ShutdownTimeout time.Duration
}

// Load загружает конфигурацию из переменных окружения, валидирует
// обязательные поля и возвращает Config или ошибку.
func Load() (*Config, error) {
	cfg := &Config{}
	var err error

	// --- Сервер ---

	// AM_PORT — порт HTTP-сервера (по умолчанию 8000)
	cfg.Port, err = getEnvInt("AM_PORT", 8000)
	if err != nil {
		return nil, fmt.Errorf("AM_PORT: %w", err)
	}
	if cfg.Port < 8000 || cfg.Port > 8009 {
		return nil, fmt.Errorf("AM_PORT: значение %d вне допустимого диапазона 8000-8009", cfg.Port)
	}

	// AM_LOG_LEVEL — уровень логирования (по умолчанию info)
	cfg.LogLevel, err = parseLogLevel(getEnvDefault("AM_LOG_LEVEL", "info"))
	if err != nil {
		return nil, fmt.Errorf("AM_LOG_LEVEL: %w", err)
	}

	// AM_LOG_FORMAT — формат логов (по умолчанию json)
	cfg.LogFormat = getEnvDefault("AM_LOG_FORMAT", "json")
	if cfg.LogFormat != "json" && cfg.LogFormat != "text" {
		return nil, fmt.Errorf("AM_LOG_FORMAT: недопустимое значение %q, допустимые: json, text", cfg.LogFormat)
	}

	// --- PostgreSQL ---

	// AM_DB_HOST — обязательный
	cfg.DBHost, err = getEnvRequired("AM_DB_HOST")
	if err != nil {
		return nil, err
	}

	// AM_DB_PORT — порт PostgreSQL (по умолчанию 5432)
	cfg.DBPort, err = getEnvInt("AM_DB_PORT", 5432)
	if err != nil {
		return nil, fmt.Errorf("AM_DB_PORT: %w", err)
	}

	// AM_DB_NAME — обязательный
	cfg.DBName, err = getEnvRequired("AM_DB_NAME")
	if err != nil {
		return nil, err
	}

	// AM_DB_USER — обязательный
	cfg.DBUser, err = getEnvRequired("AM_DB_USER")
	if err != nil {
		return nil, err
	}

	// AM_DB_PASSWORD — обязательный
	cfg.DBPassword, err = getEnvRequired("AM_DB_PASSWORD")
	if err != nil {
		return nil, err
	}

	// AM_DB_SSL_MODE — режим SSL (по умолчанию disable)
	cfg.DBSSLMode = getEnvDefault("AM_DB_SSL_MODE", "disable")
	validSSLModes := map[string]bool{
		"disable": true, "require": true, "verify-ca": true, "verify-full": true,
	}
	if !validSSLModes[cfg.DBSSLMode] {
		return nil, fmt.Errorf("AM_DB_SSL_MODE: недопустимое значение %q, допустимые: disable, require, verify-ca, verify-full", cfg.DBSSLMode)
	}

	// --- Keycloak ---

	// AM_KEYCLOAK_URL — обязательный
	cfg.KeycloakURL, err = getEnvRequired("AM_KEYCLOAK_URL")
	if err != nil {
		return nil, err
	}
	// Убираем trailing slash
	cfg.KeycloakURL = strings.TrimRight(cfg.KeycloakURL, "/")

	// AM_KEYCLOAK_REALM — realm (по умолчанию artsore)
	cfg.KeycloakRealm = getEnvDefault("AM_KEYCLOAK_REALM", "artsore")

	// AM_KEYCLOAK_CLIENT_ID — обязательный
	cfg.KeycloakClientID, err = getEnvRequired("AM_KEYCLOAK_CLIENT_ID")
	if err != nil {
		return nil, err
	}

	// AM_KEYCLOAK_CLIENT_SECRET — обязательный
	cfg.KeycloakClientSecret, err = getEnvRequired("AM_KEYCLOAK_CLIENT_SECRET")
	if err != nil {
		return nil, err
	}

	// AM_KEYCLOAK_SA_PREFIX — префикс SA (по умолчанию "sa_")
	cfg.KeycloakSAPrefix = getEnvDefault("AM_KEYCLOAK_SA_PREFIX", "sa_")

	// --- JWT ---

	// AM_JWT_ISSUER — авто-вычисляется из KeycloakURL, если не задан
	cfg.JWTIssuer = getEnvDefault("AM_JWT_ISSUER",
		fmt.Sprintf("%s/realms/%s", cfg.KeycloakURL, cfg.KeycloakRealm))

	// AM_JWT_JWKS_URL — авто-вычисляется из KeycloakURL, если не задан
	cfg.JWTJWKSURL = getEnvDefault("AM_JWT_JWKS_URL",
		fmt.Sprintf("%s/realms/%s/protocol/openid-connect/certs", cfg.KeycloakURL, cfg.KeycloakRealm))

	// AM_JWT_ROLES_CLAIM — claim для ролей (по умолчанию realm_access.roles)
	cfg.JWTRolesClaim = getEnvDefault("AM_JWT_ROLES_CLAIM", "realm_access.roles")

	// AM_JWT_GROUPS_CLAIM — claim для групп (по умолчанию groups)
	cfg.JWTGroupsClaim = getEnvDefault("AM_JWT_GROUPS_CLAIM", "groups")

	// --- Синхронизация ---

	// AM_DEPHEALTH_CHECK_INTERVAL — интервал проверки зависимостей (по умолчанию 15s)
	cfg.DephealthCheckInterval, err = getEnvDuration("AM_DEPHEALTH_CHECK_INTERVAL", 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("AM_DEPHEALTH_CHECK_INTERVAL: %w", err)
	}

	// AM_SYNC_INTERVAL — интервал синхронизации файлового реестра (по умолчанию 1h)
	cfg.SyncInterval, err = getEnvDuration("AM_SYNC_INTERVAL", time.Hour)
	if err != nil {
		return nil, fmt.Errorf("AM_SYNC_INTERVAL: %w", err)
	}

	// AM_SYNC_PAGE_SIZE — размер страницы синхронизации (по умолчанию 1000)
	cfg.SyncPageSize, err = getEnvInt("AM_SYNC_PAGE_SIZE", 1000)
	if err != nil {
		return nil, fmt.Errorf("AM_SYNC_PAGE_SIZE: %w", err)
	}
	if cfg.SyncPageSize < 1 || cfg.SyncPageSize > 10000 {
		return nil, fmt.Errorf("AM_SYNC_PAGE_SIZE: значение %d вне допустимого диапазона 1-10000", cfg.SyncPageSize)
	}

	// AM_SA_SYNC_INTERVAL — интервал синхронизации SA (по умолчанию 15m)
	cfg.SASyncInterval, err = getEnvDuration("AM_SA_SYNC_INTERVAL", 15*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("AM_SA_SYNC_INTERVAL: %w", err)
	}

	// AM_SE_CA_CERT_PATH — путь к CA-сертификату SE (опционально)
	cfg.SECACertPath = getEnvDefault("AM_SE_CA_CERT_PATH", "")

	// --- Маппинг групп → ролей ---

	// AM_ROLE_ADMIN_GROUPS — группы для роли admin (по умолчанию "artsore-admins")
	cfg.RoleAdminGroups = parseCSV(getEnvDefault("AM_ROLE_ADMIN_GROUPS", "artsore-admins"))

	// AM_ROLE_READONLY_GROUPS — группы для роли readonly (по умолчанию "artsore-viewers")
	cfg.RoleReadonlyGroups = parseCSV(getEnvDefault("AM_ROLE_READONLY_GROUPS", "artsore-viewers"))

	// --- Graceful shutdown ---

	// AM_SHUTDOWN_TIMEOUT — таймаут graceful shutdown (по умолчанию 5s)
	cfg.ShutdownTimeout, err = getEnvDuration("AM_SHUTDOWN_TIMEOUT", 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("AM_SHUTDOWN_TIMEOUT: %w", err)
	}

	return cfg, nil
}

// DatabaseDSN возвращает строку подключения к PostgreSQL.
func (c *Config) DatabaseDSN() string {
	return fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBName, c.DBUser, c.DBPassword, c.DBSSLMode,
	)
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
