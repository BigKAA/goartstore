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

// Config содержит минимальный набор параметров конфигурации Ingester Module.
// Полная конфигурация (JWT, Admin Module, SE и др.) будет добавлена в Phase 2.
type Config struct {
	// --- Сервер ---

	// Порт HTTP-сервера (диапазон 8020-8029)
	Port int
	// Уровень логирования (debug, info, warn, error)
	LogLevel slog.Level
	// Формат логов (json, text)
	LogFormat string

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
//
//nolint:unused // используется в Phase 2
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
//
//nolint:unused // используется в Phase 2
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
//
//nolint:unused // используется в Phase 2
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
//
//nolint:unused // используется в Phase 2
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
