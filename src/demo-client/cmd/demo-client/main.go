// Точка входа Demo Client — BFF (Backend for Frontend) для artstore API.
//
// Инициализация:
//  1. Загрузка конфигурации из DC_* env-переменных
//  2. Настройка логирования (slog, JSON/text)
//  3. Создание HTTP-клиента с TLS (CA cert из конфигурации)
//  4. Запуск Token Manager (Client Credentials flow, фоновое обновление)
//  5. Запуск HTTP-сервера с graceful shutdown
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

	"github.com/bigkaa/goartstore/demo-client/internal/config"
	"github.com/bigkaa/goartstore/demo-client/internal/server"
	"github.com/bigkaa/goartstore/demo-client/internal/token"
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

	// 3. HTTP-клиент с TLS настройками (для Keycloak и Gateway)
	httpClient, err := buildHTTPClient(cfg.CACertPath, cfg.RequestTimeout)
	if err != nil {
		logger.Error("ошибка создания HTTP-клиента", "error", err)
		os.Exit(1)
	}

	// 4. Token Manager — фоновое обновление SA-токена
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

	// 5. HTTP-сервер
	srv := server.New(cfg, logger, tokenMgr)

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
