package config

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

// setEnvVars устанавливает переменные окружения для теста и возвращает
// функцию очистки. Всегда вызывать defer cleanup().
func setEnvVars(t *testing.T, vars map[string]string) func() {
	t.Helper()

	// Сохраняем оригинальные значения
	originals := make(map[string]string)
	origSet := make(map[string]bool)
	for k := range vars {
		if v, ok := os.LookupEnv(k); ok {
			originals[k] = v
			origSet[k] = true
		}
	}

	// Устанавливаем новые
	for k, v := range vars {
		os.Setenv(k, v)
	}

	return func() {
		for k := range vars {
			if origSet[k] {
				os.Setenv(k, originals[k])
			} else {
				os.Unsetenv(k)
			}
		}
	}
}

// clearAllSEEnvVars очищает все переменные окружения SE_* для чистого теста.
func clearAllSEEnvVars(t *testing.T) func() {
	t.Helper()
	keys := []string{
		"SE_PORT", "SE_STORAGE_ID", "SE_DATA_DIR", "SE_WAL_DIR",
		"SE_MODE", "SE_MAX_FILE_SIZE", "SE_MAX_CAPACITY",
		"SE_GC_INTERVAL", "SE_RECONCILE_INTERVAL",
		"SE_JWKS_URL", "SE_TLS_CERT", "SE_TLS_KEY", "SE_LOG_LEVEL",
		"SE_LOG_FORMAT", "SE_REPLICA_MODE", "SE_INDEX_REFRESH_INTERVAL",
		"SE_DEPHEALTH_CHECK_INTERVAL",
		// Новые параметры Phase 3
		"SE_TLS_SKIP_VERIFY", "SE_CA_CERT_PATH",
		"SE_HTTP_CLIENT_TIMEOUT", "SE_JWKS_CLIENT_TIMEOUT",
		"SE_HTTP_READ_TIMEOUT", "SE_HTTP_WRITE_TIMEOUT", "SE_HTTP_IDLE_TIMEOUT",
		"SE_JWKS_REFRESH_INTERVAL", "SE_JWT_LEEWAY",
	}
	originals := make(map[string]string)
	origSet := make(map[string]bool)
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			originals[k] = v
			origSet[k] = true
		}
		os.Unsetenv(k)
	}
	return func() {
		for _, k := range keys {
			if origSet[k] {
				os.Setenv(k, originals[k])
			} else {
				os.Unsetenv(k)
			}
		}
	}
}

// requiredEnvVars возвращает минимальный набор обязательных переменных.
func requiredEnvVars() map[string]string {
	return map[string]string{
		"SE_STORAGE_ID":   "se-test-01",
		"SE_DATA_DIR":     "/tmp/data",
		"SE_WAL_DIR":      "/tmp/wal",
		"SE_JWKS_URL":     "https://admin.example.com/.well-known/jwks.json",
		"SE_TLS_CERT":     "/tmp/tls.crt",
		"SE_TLS_KEY":      "/tmp/tls.key",
		"SE_MAX_CAPACITY": "10737418240", // 10 GB
	}
}

func TestLoad_DefaultValues(t *testing.T) {
	cleanup := clearAllSEEnvVars(t)
	defer cleanup()

	cleanupVars := setEnvVars(t, requiredEnvVars())
	defer cleanupVars()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	// Проверяем значения по умолчанию
	if cfg.Port != 8010 {
		t.Errorf("Port: ожидалось 8010, получено %d", cfg.Port)
	}
	if cfg.Mode != "edit" {
		t.Errorf("Mode: ожидалось 'edit', получено %q", cfg.Mode)
	}
	if cfg.MaxFileSize != 1073741824 {
		t.Errorf("MaxFileSize: ожидалось 1073741824, получено %d", cfg.MaxFileSize)
	}
	if cfg.MaxCapacity != 10737418240 {
		t.Errorf("MaxCapacity: ожидалось 10737418240, получено %d", cfg.MaxCapacity)
	}
	if cfg.GCInterval != time.Hour {
		t.Errorf("GCInterval: ожидалось 1h, получено %v", cfg.GCInterval)
	}
	if cfg.ReconcileInterval != 6*time.Hour {
		t.Errorf("ReconcileInterval: ожидалось 6h, получено %v", cfg.ReconcileInterval)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel: ожидалось INFO, получено %v", cfg.LogLevel)
	}
	if cfg.LogFormat != "json" {
		t.Errorf("LogFormat: ожидалось 'json', получено %q", cfg.LogFormat)
	}
	if cfg.ReplicaMode != "standalone" {
		t.Errorf("ReplicaMode: ожидалось 'standalone', получено %q", cfg.ReplicaMode)
	}
	if cfg.IndexRefreshInterval != 30*time.Second {
		t.Errorf("IndexRefreshInterval: ожидалось 30s, получено %v", cfg.IndexRefreshInterval)
	}
	if cfg.DephealthCheckInterval != 15*time.Second {
		t.Errorf("DephealthCheckInterval: ожидалось 15s, получено %v", cfg.DephealthCheckInterval)
	}

	// --- Новые параметры Phase 3 ---

	if cfg.TLSSkipVerify != false {
		t.Errorf("TLSSkipVerify: ожидалось false, получено %v", cfg.TLSSkipVerify)
	}
	if cfg.CACertPath != "" {
		t.Errorf("CACertPath: ожидалось пустую строку, получено %q", cfg.CACertPath)
	}
	if cfg.HTTPClientTimeout != 30*time.Second {
		t.Errorf("HTTPClientTimeout: ожидалось 30s, получено %v", cfg.HTTPClientTimeout)
	}
	if cfg.JWKSClientTimeout != 30*time.Second {
		t.Errorf("JWKSClientTimeout: ожидалось 30s (fallback на глобальный), получено %v", cfg.JWKSClientTimeout)
	}
	if cfg.HTTPReadTimeout != 30*time.Second {
		t.Errorf("HTTPReadTimeout: ожидалось 30s, получено %v", cfg.HTTPReadTimeout)
	}
	if cfg.HTTPWriteTimeout != 60*time.Second {
		t.Errorf("HTTPWriteTimeout: ожидалось 60s, получено %v", cfg.HTTPWriteTimeout)
	}
	if cfg.HTTPIdleTimeout != 120*time.Second {
		t.Errorf("HTTPIdleTimeout: ожидалось 120s, получено %v", cfg.HTTPIdleTimeout)
	}
	if cfg.JWKSRefreshInterval != 15*time.Second {
		t.Errorf("JWKSRefreshInterval: ожидалось 15s, получено %v", cfg.JWKSRefreshInterval)
	}
	if cfg.JWTLeeway != 5*time.Second {
		t.Errorf("JWTLeeway: ожидалось 5s, получено %v", cfg.JWTLeeway)
	}
}

func TestLoad_AllCustomValues(t *testing.T) {
	cleanup := clearAllSEEnvVars(t)
	defer cleanup()

	vars := requiredEnvVars()
	vars["SE_PORT"] = "8015"
	vars["SE_MODE"] = "rw"
	vars["SE_MAX_FILE_SIZE"] = "536870912"
	vars["SE_MAX_CAPACITY"] = "5368709120" // 5 GB
	vars["SE_GC_INTERVAL"] = "30m"
	vars["SE_RECONCILE_INTERVAL"] = "12h"
	vars["SE_LOG_LEVEL"] = "debug"
	vars["SE_LOG_FORMAT"] = "text"
	vars["SE_REPLICA_MODE"] = "replicated"
	vars["SE_INDEX_REFRESH_INTERVAL"] = "10s"
	vars["SE_DEPHEALTH_CHECK_INTERVAL"] = "5s"
	// Новые параметры Phase 3
	vars["SE_TLS_SKIP_VERIFY"] = "true"
	vars["SE_CA_CERT_PATH"] = "/tmp/ca.crt"
	vars["SE_HTTP_CLIENT_TIMEOUT"] = "15s"
	vars["SE_JWKS_CLIENT_TIMEOUT"] = "10s"
	vars["SE_HTTP_READ_TIMEOUT"] = "20s"
	vars["SE_HTTP_WRITE_TIMEOUT"] = "45s"
	vars["SE_HTTP_IDLE_TIMEOUT"] = "90s"
	vars["SE_JWKS_REFRESH_INTERVAL"] = "30s"
	vars["SE_JWT_LEEWAY"] = "10s"

	cleanupVars := setEnvVars(t, vars)
	defer cleanupVars()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	if cfg.Port != 8015 {
		t.Errorf("Port: ожидалось 8015, получено %d", cfg.Port)
	}
	if cfg.StorageID != "se-test-01" {
		t.Errorf("StorageID: ожидалось 'se-test-01', получено %q", cfg.StorageID)
	}
	if cfg.Mode != "rw" {
		t.Errorf("Mode: ожидалось 'rw', получено %q", cfg.Mode)
	}
	if cfg.MaxFileSize != 536870912 {
		t.Errorf("MaxFileSize: ожидалось 536870912, получено %d", cfg.MaxFileSize)
	}
	if cfg.MaxCapacity != 5368709120 {
		t.Errorf("MaxCapacity: ожидалось 5368709120, получено %d", cfg.MaxCapacity)
	}
	if cfg.GCInterval != 30*time.Minute {
		t.Errorf("GCInterval: ожидалось 30m, получено %v", cfg.GCInterval)
	}
	if cfg.ReconcileInterval != 12*time.Hour {
		t.Errorf("ReconcileInterval: ожидалось 12h, получено %v", cfg.ReconcileInterval)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel: ожидалось DEBUG, получено %v", cfg.LogLevel)
	}
	if cfg.LogFormat != "text" {
		t.Errorf("LogFormat: ожидалось 'text', получено %q", cfg.LogFormat)
	}
	if cfg.ReplicaMode != "replicated" {
		t.Errorf("ReplicaMode: ожидалось 'replicated', получено %q", cfg.ReplicaMode)
	}
	if cfg.IndexRefreshInterval != 10*time.Second {
		t.Errorf("IndexRefreshInterval: ожидалось 10s, получено %v", cfg.IndexRefreshInterval)
	}
	if cfg.DephealthCheckInterval != 5*time.Second {
		t.Errorf("DephealthCheckInterval: ожидалось 5s, получено %v", cfg.DephealthCheckInterval)
	}

	// --- Новые параметры Phase 3 ---

	if cfg.TLSSkipVerify != true {
		t.Errorf("TLSSkipVerify: ожидалось true, получено %v", cfg.TLSSkipVerify)
	}
	if cfg.CACertPath != "/tmp/ca.crt" {
		t.Errorf("CACertPath: ожидалось '/tmp/ca.crt', получено %q", cfg.CACertPath)
	}
	if cfg.HTTPClientTimeout != 15*time.Second {
		t.Errorf("HTTPClientTimeout: ожидалось 15s, получено %v", cfg.HTTPClientTimeout)
	}
	if cfg.JWKSClientTimeout != 10*time.Second {
		t.Errorf("JWKSClientTimeout: ожидалось 10s, получено %v", cfg.JWKSClientTimeout)
	}
	if cfg.HTTPReadTimeout != 20*time.Second {
		t.Errorf("HTTPReadTimeout: ожидалось 20s, получено %v", cfg.HTTPReadTimeout)
	}
	if cfg.HTTPWriteTimeout != 45*time.Second {
		t.Errorf("HTTPWriteTimeout: ожидалось 45s, получено %v", cfg.HTTPWriteTimeout)
	}
	if cfg.HTTPIdleTimeout != 90*time.Second {
		t.Errorf("HTTPIdleTimeout: ожидалось 90s, получено %v", cfg.HTTPIdleTimeout)
	}
	if cfg.JWKSRefreshInterval != 30*time.Second {
		t.Errorf("JWKSRefreshInterval: ожидалось 30s, получено %v", cfg.JWKSRefreshInterval)
	}
	if cfg.JWTLeeway != 10*time.Second {
		t.Errorf("JWTLeeway: ожидалось 10s, получено %v", cfg.JWTLeeway)
	}
}

func TestLoad_MissingRequiredVars(t *testing.T) {
	requiredKeys := []string{
		"SE_STORAGE_ID", "SE_DATA_DIR", "SE_WAL_DIR",
		"SE_JWKS_URL", "SE_TLS_CERT", "SE_TLS_KEY",
		"SE_MAX_CAPACITY",
	}

	for _, missing := range requiredKeys {
		t.Run(missing, func(t *testing.T) {
			cleanup := clearAllSEEnvVars(t)
			defer cleanup()

			vars := requiredEnvVars()
			delete(vars, missing)
			cleanupVars := setEnvVars(t, vars)
			defer cleanupVars()

			_, err := Load()
			if err == nil {
				t.Errorf("ожидалась ошибка при отсутствии %s", missing)
			}
		})
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"ниже диапазона", "8009"},
		{"выше диапазона", "8020"},
		{"не число", "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := clearAllSEEnvVars(t)
			defer cleanup()

			vars := requiredEnvVars()
			vars["SE_PORT"] = tt.value
			cleanupVars := setEnvVars(t, vars)
			defer cleanupVars()

			_, err := Load()
			if err == nil {
				t.Errorf("ожидалась ошибка для SE_PORT=%s", tt.value)
			}
		})
	}
}

func TestLoad_InvalidMode(t *testing.T) {
	cleanup := clearAllSEEnvVars(t)
	defer cleanup()

	vars := requiredEnvVars()
	vars["SE_MODE"] = "invalid"
	cleanupVars := setEnvVars(t, vars)
	defer cleanupVars()

	_, err := Load()
	if err == nil {
		t.Error("ожидалась ошибка для невалидного SE_MODE")
	}
}

func TestLoad_InvalidMaxFileSize(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"не число", "abc"},
		{"нулевое", "0"},
		{"отрицательное", "-100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := clearAllSEEnvVars(t)
			defer cleanup()

			vars := requiredEnvVars()
			vars["SE_MAX_FILE_SIZE"] = tt.value
			cleanupVars := setEnvVars(t, vars)
			defer cleanupVars()

			_, err := Load()
			if err == nil {
				t.Errorf("ожидалась ошибка для SE_MAX_FILE_SIZE=%s", tt.value)
			}
		})
	}
}

func TestLoad_InvalidMaxCapacity(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"не число", "abc"},
		{"нулевое", "0"},
		{"отрицательное", "-100"},
		{"меньше MaxFileSize", "100"}, // MaxFileSize по умолчанию 1 GB
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := clearAllSEEnvVars(t)
			defer cleanup()

			vars := requiredEnvVars()
			vars["SE_MAX_CAPACITY"] = tt.value
			cleanupVars := setEnvVars(t, vars)
			defer cleanupVars()

			_, err := Load()
			if err == nil {
				t.Errorf("ожидалась ошибка для SE_MAX_CAPACITY=%s", tt.value)
			}
		})
	}
}

func TestLoad_InvalidDuration(t *testing.T) {
	durationVars := []string{
		"SE_GC_INTERVAL", "SE_RECONCILE_INTERVAL",
		"SE_INDEX_REFRESH_INTERVAL", "SE_DEPHEALTH_CHECK_INTERVAL",
		"SE_HTTP_CLIENT_TIMEOUT", "SE_HTTP_READ_TIMEOUT",
		"SE_HTTP_WRITE_TIMEOUT", "SE_HTTP_IDLE_TIMEOUT",
		"SE_JWKS_REFRESH_INTERVAL", "SE_JWT_LEEWAY",
	}

	for _, varName := range durationVars {
		t.Run(varName, func(t *testing.T) {
			cleanup := clearAllSEEnvVars(t)
			defer cleanup()

			vars := requiredEnvVars()
			vars[varName] = "not-a-duration"
			cleanupVars := setEnvVars(t, vars)
			defer cleanupVars()

			_, err := Load()
			if err == nil {
				t.Errorf("ожидалась ошибка для невалидного %s", varName)
			}
		})
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	cleanup := clearAllSEEnvVars(t)
	defer cleanup()

	vars := requiredEnvVars()
	vars["SE_LOG_LEVEL"] = "invalid"
	cleanupVars := setEnvVars(t, vars)
	defer cleanupVars()

	_, err := Load()
	if err == nil {
		t.Error("ожидалась ошибка для невалидного SE_LOG_LEVEL")
	}
}

func TestLoad_InvalidLogFormat(t *testing.T) {
	cleanup := clearAllSEEnvVars(t)
	defer cleanup()

	vars := requiredEnvVars()
	vars["SE_LOG_FORMAT"] = "yaml"
	cleanupVars := setEnvVars(t, vars)
	defer cleanupVars()

	_, err := Load()
	if err == nil {
		t.Error("ожидалась ошибка для невалидного SE_LOG_FORMAT")
	}
}

func TestLoad_InvalidReplicaMode(t *testing.T) {
	cleanup := clearAllSEEnvVars(t)
	defer cleanup()

	vars := requiredEnvVars()
	vars["SE_REPLICA_MODE"] = "clustered"
	cleanupVars := setEnvVars(t, vars)
	defer cleanupVars()

	_, err := Load()
	if err == nil {
		t.Error("ожидалась ошибка для невалидного SE_REPLICA_MODE")
	}
}

func TestLoad_ValidModes(t *testing.T) {
	modes := []string{"edit", "rw", "ro", "ar"}

	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			cleanup := clearAllSEEnvVars(t)
			defer cleanup()

			vars := requiredEnvVars()
			vars["SE_MODE"] = mode
			cleanupVars := setEnvVars(t, vars)
			defer cleanupVars()

			cfg, err := Load()
			if err != nil {
				t.Fatalf("неожиданная ошибка для режима %s: %v", mode, err)
			}
			if cfg.Mode != mode {
				t.Errorf("Mode: ожидалось %q, получено %q", mode, cfg.Mode)
			}
		})
	}
}

func TestLoad_ValidLogLevels(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"DEBUG", slog.LevelDebug},
		{"INFO", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cleanup := clearAllSEEnvVars(t)
			defer cleanup()

			vars := requiredEnvVars()
			vars["SE_LOG_LEVEL"] = tt.input
			cleanupVars := setEnvVars(t, vars)
			defer cleanupVars()

			cfg, err := Load()
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if cfg.LogLevel != tt.expected {
				t.Errorf("LogLevel: ожидалось %v, получено %v", tt.expected, cfg.LogLevel)
			}
		})
	}
}

func TestSetupLogger(t *testing.T) {
	tests := []struct {
		name   string
		format string
	}{
		{"json", "json"},
		{"text", "text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				LogLevel:  slog.LevelInfo,
				LogFormat: tt.format,
			}
			logger := SetupLogger(cfg)
			if logger == nil {
				t.Fatal("SetupLogger вернул nil")
			}
		})
	}
}

// --- Тесты Phase 3: новые параметры соединений ---

// TestLoad_JWKSClientTimeoutFallback проверяет fallback per-client таймаута на глобальный.
func TestLoad_JWKSClientTimeoutFallback(t *testing.T) {
	cleanup := clearAllSEEnvVars(t)
	defer cleanup()

	vars := requiredEnvVars()
	// Задаём глобальный, не задаём per-client
	vars["SE_HTTP_CLIENT_TIMEOUT"] = "20s"
	cleanupVars := setEnvVars(t, vars)
	defer cleanupVars()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	// JWKSClientTimeout должен fallback на глобальный 20s
	if cfg.JWKSClientTimeout != 20*time.Second {
		t.Errorf("JWKSClientTimeout: ожидалось 20s (fallback на глобальный), получено %v", cfg.JWKSClientTimeout)
	}
}

// TestLoad_JWKSClientTimeoutOverride проверяет, что per-client таймаут перекрывает глобальный.
func TestLoad_JWKSClientTimeoutOverride(t *testing.T) {
	cleanup := clearAllSEEnvVars(t)
	defer cleanup()

	vars := requiredEnvVars()
	vars["SE_HTTP_CLIENT_TIMEOUT"] = "20s"
	vars["SE_JWKS_CLIENT_TIMEOUT"] = "5s"
	cleanupVars := setEnvVars(t, vars)
	defer cleanupVars()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	if cfg.HTTPClientTimeout != 20*time.Second {
		t.Errorf("HTTPClientTimeout: ожидалось 20s, получено %v", cfg.HTTPClientTimeout)
	}
	if cfg.JWKSClientTimeout != 5*time.Second {
		t.Errorf("JWKSClientTimeout: ожидалось 5s (override), получено %v", cfg.JWKSClientTimeout)
	}
}

// TestLoad_TLSSkipVerify проверяет парсинг булевого SE_TLS_SKIP_VERIFY.
func TestLoad_TLSSkipVerify(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		{"true", "true", true},
		{"false", "false", false},
		{"1", "1", true},
		{"0", "0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := clearAllSEEnvVars(t)
			defer cleanup()

			vars := requiredEnvVars()
			vars["SE_TLS_SKIP_VERIFY"] = tt.value
			cleanupVars := setEnvVars(t, vars)
			defer cleanupVars()

			cfg, err := Load()
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if cfg.TLSSkipVerify != tt.expected {
				t.Errorf("TLSSkipVerify: ожидалось %v, получено %v", tt.expected, cfg.TLSSkipVerify)
			}
		})
	}
}

// TestLoad_TLSSkipVerifyInvalid проверяет ошибку при невалидном значении.
func TestLoad_TLSSkipVerifyInvalid(t *testing.T) {
	cleanup := clearAllSEEnvVars(t)
	defer cleanup()

	vars := requiredEnvVars()
	vars["SE_TLS_SKIP_VERIFY"] = "maybe"
	cleanupVars := setEnvVars(t, vars)
	defer cleanupVars()

	_, err := Load()
	if err == nil {
		t.Error("ожидалась ошибка для невалидного SE_TLS_SKIP_VERIFY='maybe'")
	}
}

// TestLoad_CACertPathRenaming проверяет, что SE_CA_CERT_PATH парсится корректно (переименование из SE_JWKS_CA_CERT).
func TestLoad_CACertPathRenaming(t *testing.T) {
	cleanup := clearAllSEEnvVars(t)
	defer cleanup()

	vars := requiredEnvVars()
	vars["SE_CA_CERT_PATH"] = "/etc/ssl/custom-ca.crt"
	cleanupVars := setEnvVars(t, vars)
	defer cleanupVars()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}

	if cfg.CACertPath != "/etc/ssl/custom-ca.crt" {
		t.Errorf("CACertPath: ожидалось '/etc/ssl/custom-ca.crt', получено %q", cfg.CACertPath)
	}
}

// TestLoad_InvalidJWKSClientTimeout проверяет ошибку при невалидном per-client таймауте.
func TestLoad_InvalidJWKSClientTimeout(t *testing.T) {
	cleanup := clearAllSEEnvVars(t)
	defer cleanup()

	vars := requiredEnvVars()
	vars["SE_JWKS_CLIENT_TIMEOUT"] = "-5s"
	cleanupVars := setEnvVars(t, vars)
	defer cleanupVars()

	_, err := Load()
	if err == nil {
		t.Error("ожидалась ошибка для отрицательного SE_JWKS_CLIENT_TIMEOUT")
	}
}
