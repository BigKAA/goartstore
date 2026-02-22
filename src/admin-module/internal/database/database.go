// Пакет database — подключение к PostgreSQL через pgxpool,
// применение миграций (golang-migrate) и проверка готовности.
package database

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/arturkryukov/artsore/admin-module/internal/config"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Connect создаёт пул подключений к PostgreSQL.
// Выполняет ping для проверки доступности.
func Connect(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*pgxpool.Pool, error) {
	dsn := cfg.DatabaseDSN()

	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга DSN: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания пула подключений: %w", err)
	}

	// Проверяем подключение
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ошибка подключения к PostgreSQL: %w", err)
	}

	logger.Info("Подключение к PostgreSQL установлено",
		slog.String("host", cfg.DBHost),
		slog.Int("port", cfg.DBPort),
		slog.String("database", cfg.DBName),
	)

	return pool, nil
}

// Migrate применяет SQL-миграции из embedded FS к базе данных.
// Использует golang-migrate с драйвером pgx5.
func Migrate(cfg *config.Config, logger *slog.Logger) error {
	// Создаём источник миграций из embedded FS
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("ошибка создания источника миграций: %w", err)
	}

	// Формируем URL для golang-migrate (формат pgx5://user:pass@host:port/dbname)
	dbURL := fmt.Sprintf(
		"pgx5://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.DBSSLMode,
	)

	m, err := migrate.NewWithSourceInstance("iofs", source, dbURL)
	if err != nil {
		return fmt.Errorf("ошибка инициализации миграций: %w", err)
	}
	defer m.Close()

	// Применяем все миграции
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("ошибка применения миграций: %w", err)
	}

	version, dirty, _ := m.Version()
	logger.Info("Миграции применены",
		slog.Uint64("version", uint64(version)),
		slog.Bool("dirty", dirty),
	)

	return nil
}

// ReadinessChecker — проверка готовности PostgreSQL для health endpoint.
// Реализует интерфейс handlers.ReadinessChecker.
type ReadinessChecker struct {
	pool *pgxpool.Pool
}

// NewReadinessChecker создаёт проверку готовности PostgreSQL.
func NewReadinessChecker(pool *pgxpool.Pool) *ReadinessChecker {
	return &ReadinessChecker{pool: pool}
}

// CheckReady проверяет подключение к PostgreSQL через ping.
// Возвращает статус ("ok", "fail") и сообщение.
func (c *ReadinessChecker) CheckReady() (status string, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := c.pool.Ping(ctx); err != nil {
		return "fail", fmt.Sprintf("PostgreSQL недоступен: %v", err)
	}
	return "ok", "подключение активно"
}
