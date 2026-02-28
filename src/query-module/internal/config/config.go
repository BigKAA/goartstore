// Пакет config — загрузка и валидация конфигурации Query Module
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

// Config содержит все параметры конфигурации Query Module.
// Phase 1: минимальный набор (порт, логирование, shutdown).
// Phase 2: добавятся параметры БД, JWT, таймауты, кэш.
type Config struct {
	// --- Сервер ---

	// Порт HTTP-сервера (диапазон 8030-8039)
	Port int
	// Уровень логирования (debug, info, warn, error)
	LogLevel slog.Level
	// Формат логов (json, text)
	LogFormat string

	// --- HTTP Server Timeouts ---

	// Таймаут чтения HTTP-сервера (по умолчанию 30s)
	HTTPReadTimeout time.Duration
	// Таймаут записи HTTP-сервера (по умолчанию 60s)
	HTTPWriteTimeout time.Duration
	// Таймаут простоя HTTP-сервера (по умолчанию 120s)
	HTTPIdleTimeout time.Duration

	// --- Graceful shutdown ---

	// Таймаут graceful shutdown (по умолчанию 5s)
	ShutdownTimeout time.Duration
}

// Load загружает конфигурацию из переменных окружения.
// Возвращает ошибку, если обязательные переменные не заданы
// или значения некорректны.
func Load() (*Config, error) {
	cfg := &Config{}
	var err error

	// --- Сервер ---

	// QM_PORT — порт HTTP-сервера (по умолчанию 8030)
	cfg.Port, err = getEnvInt("QM_PORT", 8030)
	if err != nil {
		return nil, fmt.Errorf("QM_PORT: %w", err)
	}

	// QM_LOG_LEVEL — уровень логирования (по умолчанию info)
	logLevel := getEnvDefault("QM_LOG_LEVEL", "info")
	cfg.LogLevel, err = parseLogLevel(logLevel)
	if err != nil {
		return nil, fmt.Errorf("QM_LOG_LEVEL: %w", err)
	}

	// QM_LOG_FORMAT — формат логов (по умолчанию json)
	cfg.LogFormat = getEnvDefault("QM_LOG_FORMAT", "json")
	if cfg.LogFormat != "json" && cfg.LogFormat != "text" {
		return nil, fmt.Errorf("QM_LOG_FORMAT: недопустимый формат %q, допустимые: json, text", cfg.LogFormat)
	}

	// --- HTTP Server Timeouts ---

	// QM_HTTP_READ_TIMEOUT — таймаут чтения (по умолчанию 30s)
	cfg.HTTPReadTimeout, err = getEnvDuration("QM_HTTP_READ_TIMEOUT", 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("QM_HTTP_READ_TIMEOUT: %w", err)
	}

	// QM_HTTP_WRITE_TIMEOUT — таймаут записи (по умолчанию 60s)
	cfg.HTTPWriteTimeout, err = getEnvDuration("QM_HTTP_WRITE_TIMEOUT", 60*time.Second)
	if err != nil {
		return nil, fmt.Errorf("QM_HTTP_WRITE_TIMEOUT: %w", err)
	}

	// QM_HTTP_IDLE_TIMEOUT — таймаут простоя (по умолчанию 120s)
	cfg.HTTPIdleTimeout, err = getEnvDuration("QM_HTTP_IDLE_TIMEOUT", 120*time.Second)
	if err != nil {
		return nil, fmt.Errorf("QM_HTTP_IDLE_TIMEOUT: %w", err)
	}

	// --- Graceful shutdown ---

	// QM_SHUTDOWN_TIMEOUT — таймаут graceful shutdown (по умолчанию 5s)
	cfg.ShutdownTimeout, err = getEnvDuration("QM_SHUTDOWN_TIMEOUT", 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("QM_SHUTDOWN_TIMEOUT: %w", err)
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
func getEnvRequired(key string) (string, error) { //nolint:unused // используется в Phase 2+
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
func getEnvDurationFallback(key string, fallbackVal time.Duration) (time.Duration, error) { //nolint:unused // используется в Phase 2+
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
func getEnvBool(key string, defaultVal bool) (bool, error) { //nolint:unused // используется в Phase 2+
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
