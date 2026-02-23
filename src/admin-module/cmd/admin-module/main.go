// Точка входа Admin Module — управляющий модуль системы Artstore.
// Загружает конфигурацию, подключается к PostgreSQL, применяет миграции,
// инициализирует Keycloak и SE клиенты, создаёт сервисный слой и API handlers,
// запускает фоновые задачи (sync SE, sync SA, topologymetrics),
// HTTP-сервер с JWT middleware и graceful shutdown.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/bigkaa/goartstore/admin-module/internal/api/handlers"
	"github.com/bigkaa/goartstore/admin-module/internal/api/middleware"
	"github.com/bigkaa/goartstore/admin-module/internal/config"
	"github.com/bigkaa/goartstore/admin-module/internal/database"
	"github.com/bigkaa/goartstore/admin-module/internal/keycloak"
	"github.com/bigkaa/goartstore/admin-module/internal/repository"
	"github.com/bigkaa/goartstore/admin-module/internal/seclient"
	"github.com/bigkaa/goartstore/admin-module/internal/server"
	"github.com/bigkaa/goartstore/admin-module/internal/service"
	"github.com/bigkaa/goartstore/admin-module/internal/ui/auth"
	uihandlers "github.com/bigkaa/goartstore/admin-module/internal/ui/handlers"
	uimiddleware "github.com/bigkaa/goartstore/admin-module/internal/ui/middleware"
	uiprometheus "github.com/bigkaa/goartstore/admin-module/internal/ui/prometheus"
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

	// Предупреждения о дефолтных значениях topologymetrics
	if os.Getenv("AM_DEPHEALTH_GROUP") == "" {
		logger.Warn("AM_DEPHEALTH_GROUP не задана, используется значение по умолчанию",
			slog.String("default", cfg.DephealthGroup),
		)
	}

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

	// 4.1 Адаптер pgxpool → *sql.DB для topologymetrics (connection pool mode).
	// Проверка здоровья PostgreSQL будет идти через существующий пул соединений,
	// что позволяет обнаружить его исчерпание.
	pgDB := stdlib.OpenDBFromPool(pool)
	defer pgDB.Close()

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

	// 10. Фоновые сервисы синхронизации
	storageSyncSvc := service.NewStorageSyncService(
		seClient, seRepo, fileRepo, syncStateRepo,
		cfg.SyncPageSize, cfg.SyncInterval,
		logger,
	)
	saSyncSvc := service.NewSASyncService(
		kcClient, saRepo, syncStateRepo,
		cfg.KeycloakSAPrefix, cfg.SASyncInterval,
		logger,
	)

	// Подключаем sync-сервисы к основным сервисам
	storageElemsSvc.SetSyncService(storageSyncSvc)
	idpSvc.SetSASyncService(saSyncSvc)

	// 11. Начальная синхронизация SA при старте
	logger.Info("Начальная синхронизация SA с Keycloak...")
	if result, syncErr := saSyncSvc.SyncNow(ctx); syncErr != nil {
		logger.Warn("Ошибка начальной синхронизации SA",
			slog.String("error", syncErr.Error()),
		)
	} else {
		logger.Info("Начальная синхронизация SA завершена",
			slog.Int("total_local", result.TotalLocal),
			slog.Int("total_keycloak", result.TotalKeycloak),
			slog.Int("created_local", result.CreatedLocal),
			slog.Int("created_keycloak", result.CreatedKeycloak),
		)
	}

	// 12. Readiness checkers (PostgreSQL + Keycloak)
	pgChecker := database.NewReadinessChecker(pool)
	kcChecker, err := middleware.NewKeycloakReadinessChecker(cfg.JWTJWKSURL, cfg.SECACertPath)
	if err != nil {
		logger.Error("Ошибка создания Keycloak readiness checker", slog.String("error", err.Error()))
		os.Exit(1)
	}
	healthHandler := handlers.NewHealthHandler(pgChecker, kcChecker)

	// 13. API handler (реализует generated.ServerInterface)
	apiHandler := handlers.NewAPIHandler(
		healthHandler,
		adminUsersSvc,
		serviceAcctsSvc,
		storageElemsSvc,
		filesSvc,
		idpSvc,
		logger,
	)

	// 14. JWT middleware
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

	// 15. Запуск фоновых задач
	storageSyncSvc.Start(ctx)
	saSyncSvc.Start(ctx)

	// 15.1 topologymetrics — мониторинг зависимостей (PostgreSQL + Keycloak)
	var dephealthSvc *service.DephealthService
	dephealthSvc, dephealthErr := service.NewDephealthService(
		"admin-module",
		cfg.DephealthGroup,
		pgDB,
		cfg.DatabaseURL(),
		cfg.JWTJWKSURL,
		cfg.DephealthCheckInterval,
		logger,
	)
	if dephealthErr != nil {
		logger.Warn("topologymetrics недоступен, запуск без мониторинга зависимостей",
			slog.String("error", dephealthErr.Error()),
		)
	} else {
		if startErr := dephealthSvc.Start(ctx); startErr != nil {
			logger.Warn("Ошибка запуска topologymetrics",
				slog.String("error", startErr.Error()),
			)
		} else {
			logger.Info("topologymetrics запущен",
				slog.String("group", cfg.DephealthGroup),
				slog.String("check_interval", cfg.DephealthCheckInterval.String()),
			)
		}
	}

	// 16. Admin UI (опционально, если AM_UI_ENABLED=true)
	var uiComponents *server.UIComponents
	if cfg.UIEnabled {
		// Определяем secure cookie: true если Keycloak URL начинается с https
		secureCookie := strings.HasPrefix(cfg.KeycloakURL, "https")

		// Session Manager — шифрование/дешифрование UI-сессий (AES-256-GCM)
		sessionMgr, sessionErr := auth.NewSessionManager(cfg.UISessionSecret, secureCookie)
		if sessionErr != nil {
			logger.Error("Ошибка создания Session Manager", slog.String("error", sessionErr.Error()))
			os.Exit(1)
		}

		if cfg.UISessionSecret == "" {
			logger.Warn("AM_UI_SESSION_SECRET не задан, UI-сессии не сохраняются между рестартами")
		}

		// OIDC-клиент для авторизации через Keycloak (PKCE)
		oidcClient := auth.NewOIDCClient(auth.OIDCConfig{
			KeycloakURL: cfg.KeycloakURL,
			Realm:       cfg.KeycloakRealm,
			ClientID:    cfg.UIOIDCClientID,
			HTTPClient:  httpClientCA,
		})

		// Auth handler — login/callback/logout
		authHandler := uihandlers.NewAuthHandler(
			oidcClient, sessionMgr,
			cfg.RoleAdminGroups, cfg.RoleReadonlyGroups,
			secureCookie,
			logger,
		)

		// UI auth middleware — проверка сессии, авто-refresh токенов
		uiAuthMiddleware := uimiddleware.NewUIAuth(sessionMgr, oidcClient, logger)

		// Dashboard handler — страница Dashboard с реальными данными
		dashboardHandler := uihandlers.NewDashboardHandler(
			storageElemsSvc,
			filesSvc,
			serviceAcctsSvc,
			dephealthSvc, // может быть nil — Dashboard обработает gracefully
			logger,
		)

		// Storage Elements handler — управление SE
		storageElementsHandler := uihandlers.NewStorageElementsHandler(
			storageElemsSvc,
			filesSvc,
			logger,
		)

		// Files handler — файловый реестр
		filesHandler := uihandlers.NewFilesHandler(
			filesSvc,
			storageElemsSvc,
			logger,
		)

		// Access handler — управление доступом (пользователи + SA)
		accessHandler := uihandlers.NewAccessHandler(
			adminUsersSvc,
			serviceAcctsSvc,
			idpSvc,
			logger,
		)

		// UI Settings Service (настройки Admin UI, Prometheus конфигурация)
		uiSettingsRepo := repository.NewUISettingsRepository(pool)
		uiSettingsSvc := service.NewUISettingsService(uiSettingsRepo, logger)

		// Prometheus-клиент для UI (опциональные графики latency)
		promClient := uiprometheus.New(uiSettingsSvc, logger)

		// Monitoring handler — страница мониторинга
		monitoringHandler := uihandlers.NewMonitoringHandler(
			storageElemsSvc,
			filesSvc,
			dephealthSvc,
			syncStateRepo,
			promClient,
			cfg,
			logger,
		)

		// Settings handler — страница настроек (admin only)
		settingsHandler := uihandlers.NewSettingsHandler(
			uiSettingsSvc,
			promClient,
			logger,
		)

		// Events handler — SSE endpoints для real-time обновлений
		eventsHandler := uihandlers.NewEventsHandler(
			storageElemsSvc,
			dephealthSvc,
			logger,
		)

		uiComponents = &server.UIComponents{
			AuthHandler:            authHandler,
			AuthMiddleware:         uiAuthMiddleware,
			DashboardHandler:       dashboardHandler,
			StorageElementsHandler: storageElementsHandler,
			FilesHandler:           filesHandler,
			AccessHandler:          accessHandler,
			MonitoringHandler:      monitoringHandler,
			SettingsHandler:        settingsHandler,
			EventsHandler:          eventsHandler,
		}

		logger.Info("Admin UI инициализирован",
			slog.String("oidc_client_id", cfg.UIOIDCClientID),
			slog.Bool("secure_cookie", secureCookie),
		)
	} else {
		logger.Info("Admin UI отключён (AM_UI_ENABLED=false)")
	}

	// 17. Создание и запуск HTTP-сервера
	srv := server.New(cfg, logger, apiHandler, jwtAuth, uiComponents)
	if err := srv.Run(); err != nil {
		logger.Error("Ошибка сервера", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// 18. Graceful shutdown фоновых задач
	logger.Info("Останавливаем фоновые задачи...")

	if dephealthSvc != nil {
		dephealthSvc.Stop()
	}
	storageSyncSvc.Stop()
	saSyncSvc.Stop()

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
