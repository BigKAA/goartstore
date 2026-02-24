// Пакет config — загрузка и валидация конфигурации Storage Element
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

// Config содержит все параметры конфигурации Storage Element.
type Config struct {
	// Порт HTTP-сервера (диапазон 8010-8019)
	Port int
	// Уникальный идентификатор SE (например, "se-moscow-01")
	StorageID string
	// Путь к директории хранения файлов
	DataDir string
	// Путь к директории WAL
	WALDir string
	// Начальный режим работы (edit, rw, ro, ar)
	Mode string
	// Максимальный размер файла в байтах
	MaxFileSize int64
	// Максимальный объём хранилища SE в байтах (обязательный параметр)
	MaxCapacity int64
	// Интервал запуска GC
	GCInterval time.Duration
	// Интервал автоматической сверки
	ReconcileInterval time.Duration
	// URL JWKS endpoint Admin Module
	JWKSUrl string
	// Путь к CA-сертификату для проверки TLS JWKS endpoint (опционально)
	JWKSCACert string
	// Путь к TLS сертификату
	TLSCert string
	// Путь к TLS приватному ключу
	TLSKey string
	// Уровень логирования (debug, info, warn, error)
	LogLevel slog.Level
	// Формат логов (json, text)
	LogFormat string
	// Режим развёртывания: standalone или replicated
	ReplicaMode string
	// Интервал обновления индекса на follower (только replicated)
	IndexRefreshInterval time.Duration
	// Интервал проверки зависимостей topologymetrics
	DephealthCheckInterval time.Duration
	// Имя группы в метриках topologymetrics (SE_DEPHEALTH_GROUP)
	DephealthGroup string
	// Имя зависимости (целевого сервиса) в метриках topologymetrics (SE_DEPHEALTH_DEP_NAME)
	DephealthDepName string
	// Имя владельца пода для метки name в topologymetrics (DEPHEALTH_NAME)
	DephealthName string

	// Таймаут graceful shutdown HTTP-сервера.
	// Должен быть меньше K8s terminationGracePeriodSeconds (по умолчанию 30s),
	// чтобы election.Stop() успел освободить NFS flock до SIGKILL.
	ShutdownTimeout time.Duration
	// Интервал retry захвата flock для follower (только replicated mode).
	// Влияет на скорость failover: меньше = быстрее обнаружение, но больше нагрузка на NFS.
	ElectionRetryInterval time.Duration
}

// Load загружает конфигурацию из переменных окружения, валидирует
// обязательные поля и возвращает Config или ошибку.
func Load() (*Config, error) {
	cfg := &Config{}

	// SE_PORT — порт HTTP-сервера (по умолчанию 8010)
	port, err := getEnvInt("SE_PORT", 8010)
	if err != nil {
		return nil, fmt.Errorf("SE_PORT: %w", err)
	}
	if port < 8010 || port > 8019 {
		return nil, fmt.Errorf("SE_PORT: значение %d вне допустимого диапазона 8010-8019", port)
	}
	cfg.Port = port

	// SE_STORAGE_ID — обязательный
	cfg.StorageID, err = getEnvRequired("SE_STORAGE_ID")
	if err != nil {
		return nil, err
	}

	// SE_DATA_DIR — обязательный
	cfg.DataDir, err = getEnvRequired("SE_DATA_DIR")
	if err != nil {
		return nil, err
	}

	// SE_WAL_DIR — обязательный
	cfg.WALDir, err = getEnvRequired("SE_WAL_DIR")
	if err != nil {
		return nil, err
	}

	// SE_MODE — режим работы (по умолчанию "edit")
	cfg.Mode = getEnvDefault("SE_MODE", "edit")
	validModes := map[string]bool{"edit": true, "rw": true, "ro": true, "ar": true}
	if !validModes[cfg.Mode] {
		return nil, fmt.Errorf("SE_MODE: недопустимое значение %q, допустимые: edit, rw, ro, ar", cfg.Mode)
	}

	// SE_MAX_FILE_SIZE — максимальный размер файла (по умолчанию 1 GB)
	maxFileSize, err := getEnvInt64("SE_MAX_FILE_SIZE", 1073741824)
	if err != nil {
		return nil, fmt.Errorf("SE_MAX_FILE_SIZE: %w", err)
	}
	if maxFileSize <= 0 {
		return nil, fmt.Errorf("SE_MAX_FILE_SIZE: значение должно быть положительным")
	}
	cfg.MaxFileSize = maxFileSize

	// SE_MAX_CAPACITY — обязательный, максимальный объём хранилища в байтах
	cfg.MaxCapacity, err = getEnvInt64Required("SE_MAX_CAPACITY")
	if err != nil {
		return nil, err
	}
	if cfg.MaxCapacity < cfg.MaxFileSize {
		return nil, fmt.Errorf("SE_MAX_CAPACITY: значение %d должно быть >= SE_MAX_FILE_SIZE (%d)",
			cfg.MaxCapacity, cfg.MaxFileSize)
	}

	// SE_GC_INTERVAL — интервал GC (по умолчанию 1h)
	cfg.GCInterval, err = getEnvDuration("SE_GC_INTERVAL", time.Hour)
	if err != nil {
		return nil, fmt.Errorf("SE_GC_INTERVAL: %w", err)
	}

	// SE_RECONCILE_INTERVAL — интервал сверки (по умолчанию 6h)
	cfg.ReconcileInterval, err = getEnvDuration("SE_RECONCILE_INTERVAL", 6*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("SE_RECONCILE_INTERVAL: %w", err)
	}

	// SE_JWKS_URL — обязательный
	cfg.JWKSUrl, err = getEnvRequired("SE_JWKS_URL")
	if err != nil {
		return nil, err
	}

	// SE_JWKS_CA_CERT — путь к CA-сертификату для JWKS endpoint (опционально)
	cfg.JWKSCACert = getEnvDefault("SE_JWKS_CA_CERT", "")

	// SE_TLS_CERT — обязательный
	cfg.TLSCert, err = getEnvRequired("SE_TLS_CERT")
	if err != nil {
		return nil, err
	}

	// SE_TLS_KEY — обязательный
	cfg.TLSKey, err = getEnvRequired("SE_TLS_KEY")
	if err != nil {
		return nil, err
	}

	// SE_LOG_LEVEL — уровень логирования (по умолчанию info)
	cfg.LogLevel, err = parseLogLevel(getEnvDefault("SE_LOG_LEVEL", "info"))
	if err != nil {
		return nil, fmt.Errorf("SE_LOG_LEVEL: %w", err)
	}

	// SE_LOG_FORMAT — формат логов (по умолчанию json)
	cfg.LogFormat = getEnvDefault("SE_LOG_FORMAT", "json")
	if cfg.LogFormat != "json" && cfg.LogFormat != "text" {
		return nil, fmt.Errorf("SE_LOG_FORMAT: недопустимое значение %q, допустимые: json, text", cfg.LogFormat)
	}

	// SE_REPLICA_MODE — режим развёртывания (по умолчанию standalone)
	cfg.ReplicaMode = getEnvDefault("SE_REPLICA_MODE", "standalone")
	if cfg.ReplicaMode != "standalone" && cfg.ReplicaMode != "replicated" {
		return nil, fmt.Errorf("SE_REPLICA_MODE: недопустимое значение %q, допустимые: standalone, replicated", cfg.ReplicaMode)
	}

	// SE_INDEX_REFRESH_INTERVAL — интервал обновления индекса на follower (по умолчанию 30s)
	cfg.IndexRefreshInterval, err = getEnvDuration("SE_INDEX_REFRESH_INTERVAL", 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("SE_INDEX_REFRESH_INTERVAL: %w", err)
	}

	// SE_DEPHEALTH_CHECK_INTERVAL — интервал проверки зависимостей (по умолчанию 15s)
	cfg.DephealthCheckInterval, err = getEnvDuration("SE_DEPHEALTH_CHECK_INTERVAL", 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("SE_DEPHEALTH_CHECK_INTERVAL: %w", err)
	}

	// SE_DEPHEALTH_GROUP — имя группы в метриках topologymetrics (по умолчанию "storage-element")
	cfg.DephealthGroup = getEnvDefault("SE_DEPHEALTH_GROUP", "storage-element")

	// SE_DEPHEALTH_DEP_NAME — имя зависимости в метриках topologymetrics (по умолчанию "admin-jwks")
	cfg.DephealthDepName = getEnvDefault("SE_DEPHEALTH_DEP_NAME", "admin-jwks")

	// DEPHEALTH_NAME — имя владельца пода для метки name в topologymetrics (без префикса модуля)
	cfg.DephealthName = getEnvDefault("DEPHEALTH_NAME", "")

	// SE_SHUTDOWN_TIMEOUT — таймаут graceful shutdown HTTP-сервера (по умолчанию 5s).
	// Должен быть меньше K8s terminationGracePeriodSeconds, чтобы оставить время
	// на освобождение NFS flock (election.Stop()) до SIGKILL.
	cfg.ShutdownTimeout, err = getEnvDuration("SE_SHUTDOWN_TIMEOUT", 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("SE_SHUTDOWN_TIMEOUT: %w", err)
	}

	// SE_ELECTION_RETRY_INTERVAL — интервал retry захвата flock для follower (по умолчанию 5s).
	// Влияет на скорость failover после смерти leader:
	//   - Меньший интервал → быстрее failover, но выше нагрузка на NFS
	//   - NFS v4 lease timeout (~90s по умолчанию) ограничивает минимальное время failover
	cfg.ElectionRetryInterval, err = getEnvDuration("SE_ELECTION_RETRY_INTERVAL", 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("SE_ELECTION_RETRY_INTERVAL: %w", err)
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

// getEnvInt64 возвращает int64 значение переменной окружения или значение по умолчанию.
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

// getEnvInt64Required возвращает обязательное int64 значение переменной окружения.
// Возвращает ошибку, если переменная не задана или значение некорректное (<=0).
func getEnvInt64Required(key string) (int64, error) {
	val := os.Getenv(key)
	if val == "" {
		return 0, fmt.Errorf("%s: обязательная переменная окружения не задана", key)
	}
	n, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: некорректное целое число: %q", key, val)
	}
	if n <= 0 {
		return 0, fmt.Errorf("%s: значение должно быть положительным, получено %d", key, n)
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
		return 0, fmt.Errorf("некорректная длительность: %q (используйте формат Go: 30s, 1h, 6h)", val)
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
