package config

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

// setEnvs устанавливает переменные окружения и возвращает функцию для их очистки.
func setEnvs(t *testing.T, envs map[string]string) {
	t.Helper()
	for k, v := range envs {
		t.Setenv(k, v)
	}
}

// minimalEnvs возвращает минимальный набор обязательных переменных.
func minimalEnvs() map[string]string {
	return map[string]string{
		"AM_DB_HOST":               "localhost",
		"AM_DB_NAME":               "artsore",
		"AM_DB_USER":               "artsore",
		"AM_DB_PASSWORD":           "secret",
		"AM_KEYCLOAK_URL":          "https://keycloak.kryukov.lan",
		"AM_KEYCLOAK_CLIENT_ID":    "artsore-admin-module",
		"AM_KEYCLOAK_CLIENT_SECRET": "kc-secret",
	}
}

func TestLoad_MinimalConfig(t *testing.T) {
	setEnvs(t, minimalEnvs())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() вернул ошибку: %v", err)
	}

	// Проверяем значения по умолчанию
	if cfg.Port != 8000 {
		t.Errorf("Port = %d, ожидается 8000", cfg.Port)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, ожидается Info", cfg.LogLevel)
	}
	if cfg.LogFormat != "json" {
		t.Errorf("LogFormat = %q, ожидается json", cfg.LogFormat)
	}
	if cfg.DBHost != "localhost" {
		t.Errorf("DBHost = %q, ожидается localhost", cfg.DBHost)
	}
	if cfg.DBPort != 5432 {
		t.Errorf("DBPort = %d, ожидается 5432", cfg.DBPort)
	}
	if cfg.DBSSLMode != "disable" {
		t.Errorf("DBSSLMode = %q, ожидается disable", cfg.DBSSLMode)
	}
	if cfg.KeycloakRealm != "artsore" {
		t.Errorf("KeycloakRealm = %q, ожидается artsore", cfg.KeycloakRealm)
	}
	if cfg.KeycloakSAPrefix != "sa_" {
		t.Errorf("KeycloakSAPrefix = %q, ожидается sa_", cfg.KeycloakSAPrefix)
	}
	if cfg.SyncInterval != time.Hour {
		t.Errorf("SyncInterval = %v, ожидается 1h", cfg.SyncInterval)
	}
	if cfg.SyncPageSize != 1000 {
		t.Errorf("SyncPageSize = %d, ожидается 1000", cfg.SyncPageSize)
	}
	if cfg.SASyncInterval != 15*time.Minute {
		t.Errorf("SASyncInterval = %v, ожидается 15m", cfg.SASyncInterval)
	}
	if cfg.DephealthCheckInterval != 15*time.Second {
		t.Errorf("DephealthCheckInterval = %v, ожидается 15s", cfg.DephealthCheckInterval)
	}
	if cfg.ShutdownTimeout != 5*time.Second {
		t.Errorf("ShutdownTimeout = %v, ожидается 5s", cfg.ShutdownTimeout)
	}
}

func TestLoad_JWTAutoDerive(t *testing.T) {
	setEnvs(t, minimalEnvs())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() вернул ошибку: %v", err)
	}

	expectedIssuer := "https://keycloak.kryukov.lan/realms/artsore"
	if cfg.JWTIssuer != expectedIssuer {
		t.Errorf("JWTIssuer = %q, ожидается %q", cfg.JWTIssuer, expectedIssuer)
	}

	expectedJWKS := "https://keycloak.kryukov.lan/realms/artsore/protocol/openid-connect/certs"
	if cfg.JWTJWKSURL != expectedJWKS {
		t.Errorf("JWTJWKSURL = %q, ожидается %q", cfg.JWTJWKSURL, expectedJWKS)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	envs := minimalEnvs()
	envs["AM_PORT"] = "8005"
	envs["AM_LOG_LEVEL"] = "debug"
	envs["AM_LOG_FORMAT"] = "text"
	envs["AM_DB_PORT"] = "5433"
	envs["AM_DB_SSL_MODE"] = "require"
	envs["AM_SYNC_INTERVAL"] = "30m"
	envs["AM_SYNC_PAGE_SIZE"] = "500"
	envs["AM_SA_SYNC_INTERVAL"] = "5m"
	envs["AM_SE_CA_CERT_PATH"] = "/certs/ca.pem"
	envs["AM_ROLE_ADMIN_GROUPS"] = "admins, super-admins"
	envs["AM_ROLE_READONLY_GROUPS"] = "viewers, guests"
	envs["AM_SHUTDOWN_TIMEOUT"] = "10s"
	setEnvs(t, envs)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() вернул ошибку: %v", err)
	}

	if cfg.Port != 8005 {
		t.Errorf("Port = %d, ожидается 8005", cfg.Port)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v, ожидается Debug", cfg.LogLevel)
	}
	if cfg.LogFormat != "text" {
		t.Errorf("LogFormat = %q, ожидается text", cfg.LogFormat)
	}
	if cfg.DBPort != 5433 {
		t.Errorf("DBPort = %d, ожидается 5433", cfg.DBPort)
	}
	if cfg.DBSSLMode != "require" {
		t.Errorf("DBSSLMode = %q, ожидается require", cfg.DBSSLMode)
	}
	if cfg.SyncInterval != 30*time.Minute {
		t.Errorf("SyncInterval = %v, ожидается 30m", cfg.SyncInterval)
	}
	if cfg.SyncPageSize != 500 {
		t.Errorf("SyncPageSize = %d, ожидается 500", cfg.SyncPageSize)
	}
	if cfg.SASyncInterval != 5*time.Minute {
		t.Errorf("SASyncInterval = %v, ожидается 5m", cfg.SASyncInterval)
	}
	if cfg.SECACertPath != "/certs/ca.pem" {
		t.Errorf("SECACertPath = %q, ожидается /certs/ca.pem", cfg.SECACertPath)
	}
	if len(cfg.RoleAdminGroups) != 2 || cfg.RoleAdminGroups[0] != "admins" || cfg.RoleAdminGroups[1] != "super-admins" {
		t.Errorf("RoleAdminGroups = %v, ожидается [admins super-admins]", cfg.RoleAdminGroups)
	}
	if len(cfg.RoleReadonlyGroups) != 2 || cfg.RoleReadonlyGroups[0] != "viewers" || cfg.RoleReadonlyGroups[1] != "guests" {
		t.Errorf("RoleReadonlyGroups = %v, ожидается [viewers guests]", cfg.RoleReadonlyGroups)
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Errorf("ShutdownTimeout = %v, ожидается 10s", cfg.ShutdownTimeout)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	requiredVars := []string{
		"AM_DB_HOST", "AM_DB_NAME", "AM_DB_USER", "AM_DB_PASSWORD",
		"AM_KEYCLOAK_URL", "AM_KEYCLOAK_CLIENT_ID", "AM_KEYCLOAK_CLIENT_SECRET",
	}

	for _, missing := range requiredVars {
		t.Run(missing, func(t *testing.T) {
			envs := minimalEnvs()
			delete(envs, missing)
			// Очищаем все переменные окружения
			for k := range minimalEnvs() {
				os.Unsetenv(k)
			}
			setEnvs(t, envs)

			_, err := Load()
			if err == nil {
				t.Errorf("Load() не вернул ошибку при отсутствии %s", missing)
			}
		})
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"ниже диапазона", "7999"},
		{"выше диапазона", "8010"},
		{"не число", "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envs := minimalEnvs()
			envs["AM_PORT"] = tt.value
			for k := range minimalEnvs() {
				os.Unsetenv(k)
			}
			setEnvs(t, envs)

			_, err := Load()
			if err == nil {
				t.Errorf("Load() не вернул ошибку при AM_PORT=%q", tt.value)
			}
		})
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	envs := minimalEnvs()
	envs["AM_LOG_LEVEL"] = "verbose"
	for k := range minimalEnvs() {
		os.Unsetenv(k)
	}
	setEnvs(t, envs)

	_, err := Load()
	if err == nil {
		t.Error("Load() не вернул ошибку при AM_LOG_LEVEL=verbose")
	}
}

func TestLoad_InvalidLogFormat(t *testing.T) {
	envs := minimalEnvs()
	envs["AM_LOG_FORMAT"] = "xml"
	for k := range minimalEnvs() {
		os.Unsetenv(k)
	}
	setEnvs(t, envs)

	_, err := Load()
	if err == nil {
		t.Error("Load() не вернул ошибку при AM_LOG_FORMAT=xml")
	}
}

func TestLoad_InvalidSSLMode(t *testing.T) {
	envs := minimalEnvs()
	envs["AM_DB_SSL_MODE"] = "prefer"
	for k := range minimalEnvs() {
		os.Unsetenv(k)
	}
	setEnvs(t, envs)

	_, err := Load()
	if err == nil {
		t.Error("Load() не вернул ошибку при AM_DB_SSL_MODE=prefer")
	}
}

func TestLoad_InvalidDuration(t *testing.T) {
	envs := minimalEnvs()
	envs["AM_SYNC_INTERVAL"] = "abc"
	for k := range minimalEnvs() {
		os.Unsetenv(k)
	}
	setEnvs(t, envs)

	_, err := Load()
	if err == nil {
		t.Error("Load() не вернул ошибку при AM_SYNC_INTERVAL=abc")
	}
}

func TestLoad_InvalidSyncPageSize(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"слишком маленький", "0"},
		{"слишком большой", "10001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envs := minimalEnvs()
			envs["AM_SYNC_PAGE_SIZE"] = tt.value
			for k := range minimalEnvs() {
				os.Unsetenv(k)
			}
			setEnvs(t, envs)

			_, err := Load()
			if err == nil {
				t.Errorf("Load() не вернул ошибку при AM_SYNC_PAGE_SIZE=%q", tt.value)
			}
		})
	}
}

func TestLoad_KeycloakURLTrailingSlash(t *testing.T) {
	envs := minimalEnvs()
	envs["AM_KEYCLOAK_URL"] = "https://keycloak.kryukov.lan/"
	for k := range minimalEnvs() {
		os.Unsetenv(k)
	}
	setEnvs(t, envs)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() вернул ошибку: %v", err)
	}
	if cfg.KeycloakURL != "https://keycloak.kryukov.lan" {
		t.Errorf("KeycloakURL = %q, ожидается без trailing slash", cfg.KeycloakURL)
	}
}

func TestDatabaseDSN(t *testing.T) {
	cfg := &Config{
		DBHost:     "db.example.com",
		DBPort:     5432,
		DBName:     "artsore",
		DBUser:     "user",
		DBPassword: "pass",
		DBSSLMode:  "disable",
	}
	expected := "host=db.example.com port=5432 dbname=artsore user=user password=pass sslmode=disable"
	if dsn := cfg.DatabaseDSN(); dsn != expected {
		t.Errorf("DatabaseDSN() = %q, ожидается %q", dsn, expected)
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
				t.Error("SetupLogger() вернул nil")
			}
		})
	}
}

func TestParseCSV(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"", nil},
		{"admins", []string{"admins"}},
		{"admins, viewers", []string{"admins", "viewers"}},
		{"admins,,viewers,", []string{"admins", "viewers"}},
		{" admins , viewers , guests ", []string{"admins", "viewers", "guests"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseCSV(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("parseCSV(%q) = %v (len %d), ожидается %v (len %d)",
					tt.input, result, len(result), tt.expected, len(tt.expected))
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("parseCSV(%q)[%d] = %q, ожидается %q", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}
