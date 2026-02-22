// Точка входа Admin Module — управляющий модуль системы Artsore v2.0.0.
// Загружает конфигурацию, подключается к PostgreSQL, применяет миграции,
// инициализирует Keycloak и SE клиенты, создаёт сервисный слой и API handlers,
// запускает HTTP-сервер с JWT middleware и graceful shutdown.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/arturkryukov/artsore/admin-module/internal/api/handlers"
	"github.com/arturkryukov/artsore/admin-module/internal/api/middleware"
	"github.com/arturkryukov/artsore/admin-module/internal/config"
	"github.com/arturkryukov/artsore/admin-module/internal/database"
	"github.com/arturkryukov/artsore/admin-module/internal/keycloak"
	"github.com/arturkryukov/artsore/admin-module/internal/repository"
	"github.com/arturkryukov/artsore/admin-module/internal/seclient"
	"github.com/arturkryukov/artsore/admin-module/internal/server"
	"github.com/arturkryukov/artsore/admin-module/internal/service"
)

func main() {
	// 1. Загрузка конфигурации из переменных окружения
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Ошибка загрузки конфигурации", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// 2. Настройка логирования
	logger := config.SetupLogger(cfg)
	logger.Info("Admin Module запускается",
		slog.String("version", config.Version),
		slog.Int("port", cfg.Port),
	)

	// 3. Применение миграций БД
	logger.Info("Применение миграций БД...")
	if err := database.Migrate(cfg, logger); err != nil {
		logger.Error("Ошибка миграций БД", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// 4. Подключение к PostgreSQL (pgxpool)
	ctx := context.Background()
	pool, err := database.Connect(ctx, cfg, logger)
	if err != nil {
		logger.Error("Ошибка подключения к PostgreSQL", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()

	// 5. HTTP-клиент с кастомным CA (для Keycloak и SE)
	var httpClientCA *http.Client
	if cfg.SECACertPath != "" {
		httpClientCA, err = buildHTTPClientWithCA(cfg.SECACertPath)
		if err != nil {
			logger.Error("Ошибка загрузки CA-сертификата", slog.String("path", cfg.SECACertPath), slog.String("error", err.Error()))
			os.Exit(1)
		}
		logger.Info("CA-сертификат загружен", slog.String("path", cfg.SECACertPath))
	}

	// 6. Keycloak Admin API клиент
	kcClient := keycloak.New(
		cfg.KeycloakURL,
		cfg.KeycloakRealm,
		cfg.KeycloakClientID,
		cfg.KeycloakClientSecret,
		httpClientCA, // nil — стандартный пул CA
		logger,
	)
	logger.Info("Keycloak клиент создан",
		slog.String("url", cfg.KeycloakURL),
		slog.String("realm", cfg.KeycloakRealm),
	)

	// 7. SE HTTP-клиент
	seClient, err := seclient.New(cfg.SECACertPath, kcClient.TokenProvider(), logger)
	if err != nil {
		logger.Error("Ошибка создания SE-клиента", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// 8. Repositories
	roleRepo := repository.NewRoleOverrideRepository(pool)
	saRepo := repository.NewServiceAccountRepository(pool)
	seRepo := repository.NewStorageElementRepository(pool)
	fileRepo := repository.NewFileRegistryRepository(pool)
	syncStateRepo := repository.NewSyncStateRepository(pool)

	// 9. Services
	adminUsersSvc := service.NewAdminUserService(
		kcClient, roleRepo,
		cfg.RoleAdminGroups, cfg.RoleReadonlyGroups,
		logger,
	)
	serviceAcctsSvc := service.NewServiceAccountService(
		kcClient, saRepo,
		cfg.KeycloakSAPrefix,
		logger,
	)
	storageElemsSvc := service.NewStorageElementService(
		seClient, seRepo, fileRepo,
		logger,
	)
	filesSvc := service.NewFileRegistryService(
		fileRepo, seRepo,
		logger,
	)
	idpSvc := service.NewIDPService(
		kcClient, saRepo, syncStateRepo,
		cfg.KeycloakURL, cfg.KeycloakRealm, cfg.KeycloakSAPrefix,
		logger,
	)

	// 10. Readiness checkers (PostgreSQL + Keycloak)
	pgChecker := database.NewReadinessChecker(pool)
	kcChecker, err := middleware.NewKeycloakReadinessChecker(cfg.JWTJWKSURL, cfg.SECACertPath)
	if err != nil {
		logger.Error("Ошибка создания Keycloak readiness checker", slog.String("error", err.Error()))
		os.Exit(1)
	}
	healthHandler := handlers.NewHealthHandler(pgChecker, kcChecker)

	// 11. API handler (реализует generated.ServerInterface)
	apiHandler := handlers.NewAPIHandler(
		healthHandler,
		adminUsersSvc,
		serviceAcctsSvc,
		storageElemsSvc,
		filesSvc,
		idpSvc,
		logger,
	)

	// 12. JWT middleware
	// Адаптер RoleOverrideRepository → middleware.RoleOverrideProvider
	roleProvider := &roleOverrideAdapter{repo: roleRepo}

	jwtAuth, err := middleware.NewJWTAuth(
		cfg.JWTJWKSURL,
		cfg.SECACertPath,
		cfg.JWTIssuer,
		roleProvider,
		cfg.RoleAdminGroups,
		cfg.RoleReadonlyGroups,
		logger,
	)
	if err != nil {
		logger.Error("Ошибка создания JWT middleware", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer jwtAuth.Close()
	logger.Info("JWT middleware инициализирован",
		slog.String("jwks_url", cfg.JWTJWKSURL),
		slog.String("issuer", cfg.JWTIssuer),
	)

	// 13. Создание и запуск HTTP-сервера
	srv := server.New(cfg, logger, apiHandler, jwtAuth)
	if err := srv.Run(); err != nil {
		logger.Error("Ошибка сервера", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("Admin Module остановлен")
}

// --- Вспомогательные типы ---

// roleOverrideAdapter — адаптер RoleOverrideRepository → middleware.RoleOverrideProvider.
// Преобразует *model.RoleOverride в *string (additional_role).
type roleOverrideAdapter struct {
	repo repository.RoleOverrideRepository
}

// GetRoleOverride возвращает дополнительную роль для пользователя.
// Если override не найден — возвращает nil, nil.
func (a *roleOverrideAdapter) GetRoleOverride(ctx context.Context, keycloakUserID string) (*string, error) {
	ro, err := a.repo.GetByKeycloakUserID(ctx, keycloakUserID)
	if err != nil {
		// Если override не найден — возвращаем nil (нет дополнительной роли)
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if ro == nil {
		return nil, nil
	}
	return &ro.AdditionalRole, nil
}

// buildHTTPClientWithCA создаёт HTTP-клиент с кастомным CA-сертификатом.
func buildHTTPClientWithCA(caCertPath string) (*http.Client, error) {
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, err
	}

	caCertPool, err := x509.SystemCertPool()
	if err != nil {
		caCertPool = x509.NewCertPool()
	}
	caCertPool.AppendCertsFromPEM(caCert)

	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
			},
		},
	}, nil
}
