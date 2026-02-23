package database

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/arturkryukov/artstore/admin-module/internal/config"
)

// setupTestDB запускает PostgreSQL в Docker-контейнере через testcontainers.
// Возвращает конфиг и функцию для очистки.
func setupTestDB(t *testing.T) *config.Config {
	t.Helper()

	if os.Getenv("TEST_INTEGRATION") == "" {
		t.Skip("Пропуск интеграционного теста: TEST_INTEGRATION не установлена")
	}

	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"docker.io/postgres:17-alpine",
		postgres.WithDatabase("artstore_test"),
		postgres.WithUsername("artstore"),
		postgres.WithPassword("test-password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("Не удалось запустить PostgreSQL контейнер: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Ошибка остановки контейнера: %v", err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("Не удалось получить host контейнера: %v", err)
	}
	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("Не удалось получить port контейнера: %v", err)
	}

	// Создаём конфиг с минимальными значениями
	os.Setenv("AM_DB_HOST", host)
	os.Setenv("AM_DB_PORT", port.Port())
	os.Setenv("AM_DB_NAME", "artstore_test")
	os.Setenv("AM_DB_USER", "artstore")
	os.Setenv("AM_DB_PASSWORD", "test-password")
	os.Setenv("AM_DB_SSL_MODE", "disable")
	os.Setenv("AM_KEYCLOAK_URL", "http://localhost:8080")
	os.Setenv("AM_KEYCLOAK_CLIENT_ID", "test")
	os.Setenv("AM_KEYCLOAK_CLIENT_SECRET", "test")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}

	return cfg
}

// TestConnect проверяет подключение к PostgreSQL через pgxpool.
func TestConnect(t *testing.T) {
	cfg := setupTestDB(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	pool, err := Connect(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("Connect() вернул ошибку: %v", err)
	}
	defer pool.Close()

	// Проверяем ping
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("pool.Ping() вернул ошибку: %v", err)
	}
}

// TestMigrate проверяет применение миграций.
func TestMigrate(t *testing.T) {
	cfg := setupTestDB(t)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Применяем миграции
	if err := Migrate(cfg, logger); err != nil {
		t.Fatalf("Migrate() вернул ошибку: %v", err)
	}

	// Повторное применение — должно быть без ошибки (ErrNoChange)
	if err := Migrate(cfg, logger); err != nil {
		t.Fatalf("Повторный Migrate() вернул ошибку: %v", err)
	}

	// Проверяем, что таблицы созданы
	ctx := context.Background()
	pool, err := Connect(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("Connect() вернул ошибку: %v", err)
	}
	defer pool.Close()

	tables := []string{
		"storage_elements",
		"file_registry",
		"service_accounts",
		"role_overrides",
		"sync_state",
	}

	for _, table := range tables {
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS (
				SELECT FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
			)`, table).Scan(&exists)
		if err != nil {
			t.Fatalf("Ошибка проверки таблицы %s: %v", table, err)
		}
		if !exists {
			t.Errorf("Таблица %s не создана", table)
		}
	}

	// Проверяем начальную запись sync_state
	var syncID int
	err = pool.QueryRow(ctx, `SELECT id FROM sync_state WHERE id = 1`).Scan(&syncID)
	if err != nil {
		t.Fatalf("Начальная запись sync_state не найдена: %v", err)
	}
	if syncID != 1 {
		t.Errorf("sync_state.id = %d, ожидали 1", syncID)
	}
}

// TestReadinessChecker проверяет ReadinessChecker.
func TestReadinessChecker(t *testing.T) {
	cfg := setupTestDB(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	pool, err := Connect(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("Connect() вернул ошибку: %v", err)
	}
	defer pool.Close()

	checker := NewReadinessChecker(pool)

	// Проверяем готовность — должен вернуть "ok"
	status, msg := checker.CheckReady()
	if status != "ok" {
		t.Errorf("CheckReady() status = %q, message = %q; ожидали status = %q",
			status, msg, "ok")
	}
}
