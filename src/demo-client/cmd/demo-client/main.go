// Точка входа Demo Client — BFF (Backend for Frontend) для artstore API.
//
// Инициализация:
//  1. Загрузка конфигурации из DC_* env-переменных
//  2. Настройка логирования (slog, JSON/text)
//  3. Создание HTTP-клиента с TLS (CA cert из конфигурации)
//  4. Запуск Token Manager (Client Credentials flow, фоновое обновление)
//  5. Инициализация Activity Log, Gateway Client, Service Layer
//  6. Запуск HTTP-сервера с graceful shutdown
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/bigkaa/goartstore/demo-client/internal/activity"
	"github.com/bigkaa/goartstore/demo-client/internal/config"
	"github.com/bigkaa/goartstore/demo-client/internal/gateway"
	"github.com/bigkaa/goartstore/demo-client/internal/server"
	"github.com/bigkaa/goartstore/demo-client/internal/service"
	"github.com/bigkaa/goartstore/demo-client/internal/token"
	"github.com/bigkaa/goartstore/demo-client/internal/ui/i18n"
)

func main() {
	// 1. Загрузка конфигурации
	cfg, err := config.Load()
	if err != nil {
		slog.Error("ошибка загрузки конфигурации", "error", err)
		os.Exit(1)
	}

	// 2. Настройка логирования
	logger := config.SetupLogger(cfg)
	logger.Info("Demo Client запускается",
		"version", config.Version,
		"port", cfg.Port,
	)

	// 3. Инициализация i18n
	bundle := i18n.Init(logger.With("component", "i18n"))
	if err := i18n.LoadFromEmbedFS(bundle, logger.With("component", "i18n")); err != nil {
		logger.Error("ошибка загрузки i18n", "error", err)
		os.Exit(1)
	}

	// 4. HTTP-клиент с TLS настройками (для Keycloak и Gateway)
	httpClient, err := buildHTTPClient(cfg.CACertPath, cfg.RequestTimeout)
	if err != nil {
		logger.Error("ошибка создания HTTP-клиента", "error", err)
		os.Exit(1)
	}

	// 5. Token Manager — фоновое обновление SA-токена
	tokenMgr := token.NewManager(
		cfg.TokenURL,
		cfg.ClientID,
		cfg.ClientSecret,
		cfg.Scopes,
		cfg.TokenRefreshBefore,
		httpClient,
		logger,
	)
	ctx := context.Background()
	tokenMgr.Start(ctx)
	defer tokenMgr.Stop()

	logger.Info("Token Manager запущен",
		"token_url", cfg.TokenURL,
		"client_id", cfg.ClientID,
		"scopes", cfg.Scopes,
	)

	// 6. Activity Log — in-memory ring buffer для записи API-вызовов
	activityLog := activity.NewLog(cfg.ActivityLogSize)
	logger.Info("Activity Log инициализирован",
		"size", cfg.ActivityLogSize,
	)

	// 7. Gateway Client — HTTP-клиент к API Gateway
	gwClient := gateway.NewClient(gateway.ClientConfig{
		BaseURL:        cfg.GatewayURL,
		RequestTimeout: cfg.RequestTimeout,
		UploadTimeout:  cfg.UploadTimeout,
		HTTPClient:     httpClient,
	}, tokenMgr.GetToken, logger.With("component", "gateway"))

	logger.Info("Gateway Client инициализирован",
		"gateway_url", cfg.GatewayURL,
		"request_timeout", cfg.RequestTimeout,
		"upload_timeout", cfg.UploadTimeout,
	)

	// 8. Service Layer — бизнес-логика поверх Gateway Client
	_ = service.NewUploadService(gwClient, activityLog, logger.With("component", "upload"))
	_ = service.NewSearchService(gwClient, activityLog, logger.With("component", "search"))
	_ = service.NewDownloadService(gwClient, activityLog, logger.With("component", "download"))
	dashboardSvc := service.NewDashboardService(gwClient, tokenMgr, activityLog, logger.With("component", "dashboard"))

	logger.Info("Service Layer инициализирован")

	// 9. HTTP-сервер с UI маршрутами
	srv := server.New(cfg, logger, tokenMgr, server.Deps{
		DashboardSvc: dashboardSvc,
		ActivityLog:  activityLog,
	})

	if err := srv.Run(); err != nil {
		logger.Error("ошибка HTTP-сервера", "error", err)
		os.Exit(1)
	}
}

// buildHTTPClient — создание HTTP-клиента с опциональным CA-сертификатом.
// Если caCertPath пуст, используются системные сертификаты.
func buildHTTPClient(caCertPath string, timeout time.Duration) (*http.Client, error) {
	transport := &http.Transport{}

	if caCertPath != "" {
		caCert, err := os.ReadFile(caCertPath)
		if err != nil {
			return nil, fmt.Errorf("чтение CA сертификата %s: %w", caCertPath, err)
		}

		caCertPool, err := x509.SystemCertPool()
		if err != nil {
			caCertPool = x509.NewCertPool()
		}

		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("не удалось добавить CA сертификат из %s", caCertPath)
		}

		transport.TLSClientConfig = &tls.Config{
			RootCAs: caCertPool,
		}
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}, nil
}
