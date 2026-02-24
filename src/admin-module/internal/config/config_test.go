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
		"AM_DB_HOST":                "localhost",
		"AM_DB_NAME":                "artstore",
		"AM_DB_USER":                "artstore",
		"AM_DB_PASSWORD":            "secret",
		"AM_KEYCLOAK_URL":           "https://keycloak.kryukov.lan",
		"AM_KEYCLOAK_CLIENT_ID":     "artstore-admin-module",
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
	if cfg.KeycloakRealm != "artstore" {
		t.Errorf("KeycloakRealm = %q, ожидается artstore", cfg.KeycloakRealm)
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

	// --- Новые параметры: defaults ---

	// TLS
	if cfg.TLSSkipVerify != false {
		t.Errorf("TLSSkipVerify = %v, ожидается false", cfg.TLSSkipVerify)
	}
	if cfg.CACertPath != "" {
		t.Errorf("CACertPath = %q, ожидается пустая строка", cfg.CACertPath)
	}

	// HTTP Client Timeouts — все по умолчанию 30s (глобальный default)
	if cfg.HTTPClientTimeout != 30*time.Second {
		t.Errorf("HTTPClientTimeout = %v, ожидается 30s", cfg.HTTPClientTimeout)
	}
	if cfg.KeycloakClientTimeout != 30*time.Second {
		t.Errorf("KeycloakClientTimeout = %v, ожидается 30s (fallback на глобальный)", cfg.KeycloakClientTimeout)
	}
	if cfg.SEClientTimeout != 30*time.Second {
		t.Errorf("SEClientTimeout = %v, ожидается 30s (fallback на глобальный)", cfg.SEClientTimeout)
	}
	if cfg.JWKSClientTimeout != 30*time.Second {
		t.Errorf("JWKSClientTimeout = %v, ожидается 30s (fallback на глобальный)", cfg.JWKSClientTimeout)
	}
	if cfg.OIDCClientTimeout != 30*time.Second {
		t.Errorf("OIDCClientTimeout = %v, ожидается 30s (fallback на глобальный)", cfg.OIDCClientTimeout)
	}
	if cfg.PrometheusClientTimeout != 30*time.Second {
		t.Errorf("PrometheusClientTimeout = %v, ожидается 30s (fallback на глобальный)", cfg.PrometheusClientTimeout)
	}

	// HTTP Server Timeouts
	if cfg.HTTPReadTimeout != 30*time.Second {
		t.Errorf("HTTPReadTimeout = %v, ожидается 30s", cfg.HTTPReadTimeout)
	}
	if cfg.HTTPWriteTimeout != 60*time.Second {
		t.Errorf("HTTPWriteTimeout = %v, ожидается 60s", cfg.HTTPWriteTimeout)
	}
	if cfg.HTTPIdleTimeout != 120*time.Second {
		t.Errorf("HTTPIdleTimeout = %v, ожидается 120s", cfg.HTTPIdleTimeout)
	}

	// Keycloak
	if cfg.KeycloakReadinessTimeout != 5*time.Second {
		t.Errorf("KeycloakReadinessTimeout = %v, ожидается 5s", cfg.KeycloakReadinessTimeout)
	}
	if cfg.KeycloakTokenRefreshThreshold != 30*time.Second {
		t.Errorf("KeycloakTokenRefreshThreshold = %v, ожидается 30s", cfg.KeycloakTokenRefreshThreshold)
	}

	// JWT/JWKS
	if cfg.JWKSRefreshInterval != 15*time.Second {
		t.Errorf("JWKSRefreshInterval = %v, ожидается 15s", cfg.JWKSRefreshInterval)
	}
	if cfg.JWTLeeway != 5*time.Second {
		t.Errorf("JWTLeeway = %v, ожидается 5s", cfg.JWTLeeway)
	}

	// UI
	if cfg.SSEInterval != 15*time.Second {
		t.Errorf("SSEInterval = %v, ожидается 15s", cfg.SSEInterval)
	}
}

func TestLoad_JWTAutoDerive(t *testing.T) {
	setEnvs(t, minimalEnvs())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() вернул ошибку: %v", err)
	}

	expectedIssuer := "https://keycloak.kryukov.lan/realms/artstore"
	if cfg.JWTIssuer != expectedIssuer {
		t.Errorf("JWTIssuer = %q, ожидается %q", cfg.JWTIssuer, expectedIssuer)
	}

	expectedJWKS := "https://keycloak.kryukov.lan/realms/artstore/protocol/openid-connect/certs"
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
	envs["AM_CA_CERT_PATH"] = "/certs/ca.pem"
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
	if cfg.CACertPath != "/certs/ca.pem" {
		t.Errorf("CACertPath = %q, ожидается /certs/ca.pem", cfg.CACertPath)
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

func TestLoad_TLSSkipVerify(t *testing.T) {
	// Тест: AM_TLS_SKIP_VERIFY=true парсится корректно
	envs := minimalEnvs()
	envs["AM_TLS_SKIP_VERIFY"] = "true"
	setEnvs(t, envs)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() вернул ошибку: %v", err)
	}
	if !cfg.TLSSkipVerify {
		t.Error("TLSSkipVerify = false, ожидается true")
	}
}

func TestLoad_TLSSkipVerify_InvalidValue(t *testing.T) {
	envs := minimalEnvs()
	envs["AM_TLS_SKIP_VERIFY"] = "maybe"
	for k := range minimalEnvs() {
		os.Unsetenv(k)
	}
	setEnvs(t, envs)

	_, err := Load()
	if err == nil {
		t.Error("Load() не вернул ошибку при AM_TLS_SKIP_VERIFY=maybe")
	}
}

// TestLoad_ClientTimeoutFallback проверяет иерархию таймаутов:
// per-client не задан → fallback на глобальный HTTPClientTimeout.
func TestLoad_ClientTimeoutFallback(t *testing.T) {
	envs := minimalEnvs()
	envs["AM_HTTP_CLIENT_TIMEOUT"] = "10s"
	setEnvs(t, envs)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() вернул ошибку: %v", err)
	}

	// Глобальный таймаут = 10s
	if cfg.HTTPClientTimeout != 10*time.Second {
		t.Errorf("HTTPClientTimeout = %v, ожидается 10s", cfg.HTTPClientTimeout)
	}

	// Все per-client таймауты должны наследовать глобальный (10s)
	checks := []struct {
		name  string
		value time.Duration
	}{
		{"KeycloakClientTimeout", cfg.KeycloakClientTimeout},
		{"SEClientTimeout", cfg.SEClientTimeout},
		{"JWKSClientTimeout", cfg.JWKSClientTimeout},
		{"OIDCClientTimeout", cfg.OIDCClientTimeout},
		{"PrometheusClientTimeout", cfg.PrometheusClientTimeout},
	}
	for _, c := range checks {
		if c.value != 10*time.Second {
			t.Errorf("%s = %v, ожидается 10s (fallback на глобальный)", c.name, c.value)
		}
	}
}

// TestLoad_ClientTimeoutOverride проверяет, что per-client таймаут переопределяет глобальный.
func TestLoad_ClientTimeoutOverride(t *testing.T) {
	envs := minimalEnvs()
	envs["AM_HTTP_CLIENT_TIMEOUT"] = "10s"
	envs["AM_KEYCLOAK_CLIENT_TIMEOUT"] = "5s"
	envs["AM_SE_CLIENT_TIMEOUT"] = "20s"
	envs["AM_JWKS_CLIENT_TIMEOUT"] = "8s"
	envs["AM_OIDC_CLIENT_TIMEOUT"] = "12s"
	envs["AM_PROMETHEUS_CLIENT_TIMEOUT"] = "15s"
	setEnvs(t, envs)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() вернул ошибку: %v", err)
	}

	checks := []struct {
		name     string
		value    time.Duration
		expected time.Duration
	}{
		{"HTTPClientTimeout", cfg.HTTPClientTimeout, 10 * time.Second},
		{"KeycloakClientTimeout", cfg.KeycloakClientTimeout, 5 * time.Second},
		{"SEClientTimeout", cfg.SEClientTimeout, 20 * time.Second},
		{"JWKSClientTimeout", cfg.JWKSClientTimeout, 8 * time.Second},
		{"OIDCClientTimeout", cfg.OIDCClientTimeout, 12 * time.Second},
		{"PrometheusClientTimeout", cfg.PrometheusClientTimeout, 15 * time.Second},
	}
	for _, c := range checks {
		if c.value != c.expected {
			t.Errorf("%s = %v, ожидается %v", c.name, c.value, c.expected)
		}
	}
}

// TestLoad_ClientTimeoutMixed проверяет смешанный сценарий:
// часть per-client задана, часть наследует глобальный.
func TestLoad_ClientTimeoutMixed(t *testing.T) {
	envs := minimalEnvs()
	envs["AM_HTTP_CLIENT_TIMEOUT"] = "15s"
	envs["AM_KEYCLOAK_CLIENT_TIMEOUT"] = "5s"
	// Остальные не заданы — должны наследовать 15s
	setEnvs(t, envs)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() вернул ошибку: %v", err)
	}

	if cfg.KeycloakClientTimeout != 5*time.Second {
		t.Errorf("KeycloakClientTimeout = %v, ожидается 5s (явный override)", cfg.KeycloakClientTimeout)
	}
	if cfg.SEClientTimeout != 15*time.Second {
		t.Errorf("SEClientTimeout = %v, ожидается 15s (fallback на глобальный)", cfg.SEClientTimeout)
	}
	if cfg.JWKSClientTimeout != 15*time.Second {
		t.Errorf("JWKSClientTimeout = %v, ожидается 15s (fallback на глобальный)", cfg.JWKSClientTimeout)
	}
}

// TestLoad_ClientTimeoutDefaultFallback проверяет цепочку:
// per-client не задан → global не задан → hardcoded 30s.
func TestLoad_ClientTimeoutDefaultFallback(t *testing.T) {
	envs := minimalEnvs()
	// Не задаём ни глобальный, ни per-client
	setEnvs(t, envs)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() вернул ошибку: %v", err)
	}

	if cfg.HTTPClientTimeout != 30*time.Second {
		t.Errorf("HTTPClientTimeout = %v, ожидается 30s (hardcoded default)", cfg.HTTPClientTimeout)
	}
	if cfg.KeycloakClientTimeout != 30*time.Second {
		t.Errorf("KeycloakClientTimeout = %v, ожидается 30s (fallback → global → hardcoded)", cfg.KeycloakClientTimeout)
	}
}

func TestLoad_HTTPServerTimeouts(t *testing.T) {
	envs := minimalEnvs()
	envs["AM_HTTP_READ_TIMEOUT"] = "10s"
	envs["AM_HTTP_WRITE_TIMEOUT"] = "30s"
	envs["AM_HTTP_IDLE_TIMEOUT"] = "60s"
	setEnvs(t, envs)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() вернул ошибку: %v", err)
	}

	if cfg.HTTPReadTimeout != 10*time.Second {
		t.Errorf("HTTPReadTimeout = %v, ожидается 10s", cfg.HTTPReadTimeout)
	}
	if cfg.HTTPWriteTimeout != 30*time.Second {
		t.Errorf("HTTPWriteTimeout = %v, ожидается 30s", cfg.HTTPWriteTimeout)
	}
	if cfg.HTTPIdleTimeout != 60*time.Second {
		t.Errorf("HTTPIdleTimeout = %v, ожидается 60s", cfg.HTTPIdleTimeout)
	}
}

func TestLoad_KeycloakParams(t *testing.T) {
	envs := minimalEnvs()
	envs["AM_KEYCLOAK_READINESS_TIMEOUT"] = "10s"
	envs["AM_KEYCLOAK_TOKEN_REFRESH_THRESHOLD"] = "60s"
	setEnvs(t, envs)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() вернул ошибку: %v", err)
	}

	if cfg.KeycloakReadinessTimeout != 10*time.Second {
		t.Errorf("KeycloakReadinessTimeout = %v, ожидается 10s", cfg.KeycloakReadinessTimeout)
	}
	if cfg.KeycloakTokenRefreshThreshold != 60*time.Second {
		t.Errorf("KeycloakTokenRefreshThreshold = %v, ожидается 60s", cfg.KeycloakTokenRefreshThreshold)
	}
}

func TestLoad_JWKSParams(t *testing.T) {
	envs := minimalEnvs()
	envs["AM_JWKS_REFRESH_INTERVAL"] = "30s"
	envs["AM_JWT_LEEWAY"] = "10s"
	setEnvs(t, envs)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() вернул ошибку: %v", err)
	}

	if cfg.JWKSRefreshInterval != 30*time.Second {
		t.Errorf("JWKSRefreshInterval = %v, ожидается 30s", cfg.JWKSRefreshInterval)
	}
	if cfg.JWTLeeway != 10*time.Second {
		t.Errorf("JWTLeeway = %v, ожидается 10s", cfg.JWTLeeway)
	}
}

func TestLoad_SSEInterval(t *testing.T) {
	envs := minimalEnvs()
	envs["AM_SSE_INTERVAL"] = "30s"
	setEnvs(t, envs)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() вернул ошибку: %v", err)
	}

	if cfg.SSEInterval != 30*time.Second {
		t.Errorf("SSEInterval = %v, ожидается 30s", cfg.SSEInterval)
	}
}

// TestLoad_JWTLeewayZero проверяет, что JWTLeeway допускает значение 0.
func TestLoad_JWTLeewayZero(t *testing.T) {
	envs := minimalEnvs()
	envs["AM_JWT_LEEWAY"] = "0s"
	setEnvs(t, envs)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() вернул ошибку: %v", err)
	}

	if cfg.JWTLeeway != 0 {
		t.Errorf("JWTLeeway = %v, ожидается 0s", cfg.JWTLeeway)
	}
}

// --- Тесты валидации новых параметров ---

func TestLoad_InvalidTimeoutValues(t *testing.T) {
	tests := []struct {
		name string
		env  string
		val  string
	}{
		{"глобальный клиент таймаут отрицательный", "AM_HTTP_CLIENT_TIMEOUT", "-1s"},
		{"глобальный клиент таймаут нулевой", "AM_HTTP_CLIENT_TIMEOUT", "0s"},
		{"глобальный клиент таймаут невалидный", "AM_HTTP_CLIENT_TIMEOUT", "abc"},
		{"per-client таймаут отрицательный", "AM_KEYCLOAK_CLIENT_TIMEOUT", "-1s"},
		{"per-client таймаут нулевой", "AM_KEYCLOAK_CLIENT_TIMEOUT", "0s"},
		{"per-client таймаут невалидный", "AM_SE_CLIENT_TIMEOUT", "xyz"},
		{"HTTP read timeout отрицательный", "AM_HTTP_READ_TIMEOUT", "-1s"},
		{"HTTP write timeout нулевой", "AM_HTTP_WRITE_TIMEOUT", "0s"},
		{"HTTP idle timeout невалидный", "AM_HTTP_IDLE_TIMEOUT", "abc"},
		{"Keycloak readiness отрицательный", "AM_KEYCLOAK_READINESS_TIMEOUT", "-5s"},
		{"Keycloak token refresh нулевой", "AM_KEYCLOAK_TOKEN_REFRESH_THRESHOLD", "0s"},
		{"JWKS refresh interval отрицательный", "AM_JWKS_REFRESH_INTERVAL", "-1s"},
		{"JWT leeway отрицательный", "AM_JWT_LEEWAY", "-1s"},
		{"SSE interval нулевой", "AM_SSE_INTERVAL", "0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envs := minimalEnvs()
			envs[tt.env] = tt.val
			for k := range minimalEnvs() {
				os.Unsetenv(k)
			}
			setEnvs(t, envs)

			_, err := Load()
			if err == nil {
				t.Errorf("Load() не вернул ошибку при %s=%q", tt.env, tt.val)
			}
		})
	}
}

// --- Существующие тесты (обновлённые) ---

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
		DBName:     "artstore",
		DBUser:     "user",
		DBPassword: "pass",
		DBSSLMode:  "disable",
	}
	expected := "host=db.example.com port=5432 dbname=artstore user=user password=pass sslmode=disable"
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
