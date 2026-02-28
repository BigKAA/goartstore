// Пакет seclient — HTTP-клиент для скачивания файлов из Storage Elements.
// Поддерживает TLS с кастомным CA (QM_SE_CA_CERT_PATH), streaming download,
// проброс Range header для частичных загрузок.
package seclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// TokenProvider — функция, возвращающая SA-токен для авторизации запросов к SE.
// Обычно это adminclient.Client.GetToken.
type TokenProvider func(ctx context.Context) (string, error)

// Client — HTTP-клиент для скачивания файлов из Storage Elements.
type Client struct {
	httpClient    *http.Client
	tokenProvider TokenProvider
	logger        *slog.Logger
}

// New создаёт SE-клиент для proxy download.
// caCertPath — путь к CA-сертификату для TLS (пустая строка — стандартный пул).
// timeout — таймаут HTTP-запросов для скачивания (из конфигурации QM_SE_DOWNLOAD_TIMEOUT).
// tokenProvider — функция для получения SA-токена (adminclient.Client.GetToken).
func New(caCertPath string, timeout time.Duration, tokenProvider TokenProvider, logger *slog.Logger) (*Client, error) {
	transport := &http.Transport{
		// Настройка пула idle-соединений для эффективного переиспользования
		MaxIdleConnsPerHost: 10,
	}

	if caCertPath != "" {
		tlsConfig, err := buildTLSConfig(caCertPath)
		if err != nil {
			return nil, fmt.Errorf("загрузка CA-сертификата SE: %w", err)
		}
		transport.TLSClientConfig = tlsConfig
		logger.Info("CA-сертификат SE добавлен в пул доверия",
			slog.String("ca_cert", caCertPath),
		)
	}

	httpClient := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}

	return &Client{
		httpClient:    httpClient,
		tokenProvider: tokenProvider,
		logger:        logger.With(slog.String("component", "se_client")),
	}, nil
}

// Download выполняет streaming-загрузку файла из Storage Element.
// Возвращает *http.Response — вызывающий код ОБЯЗАН закрыть resp.Body.
//
// seURL — базовый URL Storage Element (например, https://se-01:8010).
// fileID — UUID файла для скачивания.
// rangeHeader — значение заголовка Range от клиента (пустая строка — без Range).
//
// Формат запроса: GET {seURL}/api/v1/files/{fileID}/download
// Авторизация: Bearer SA-токен через tokenProvider.
func (c *Client) Download(ctx context.Context, seURL, fileID, rangeHeader string) (*http.Response, error) {
	reqURL := fmt.Sprintf("%s/api/v1/files/%s/download", normalizeURL(seURL), fileID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("создание запроса Download: %w", err)
	}

	// Добавляем SA-токен для авторизации
	if c.tokenProvider != nil {
		token, tokenErr := c.tokenProvider(ctx)
		if tokenErr != nil {
			return nil, fmt.Errorf("получение токена для SE: %w", tokenErr)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}

	// Пробрасываем Range header от клиента
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	resp, err := c.httpClient.Do(req) //nolint:gosec // G704: URL из конфигурации SE
	if err != nil {
		return nil, fmt.Errorf("запрос Download к %s: %w", seURL, err)
	}

	// Не закрываем resp.Body — вызывающий код отвечает за это (streaming)
	return resp, nil
}

// buildTLSConfig создаёт TLS-конфигурацию с кастомным CA-сертификатом.
func buildTLSConfig(caCertPath string) (*tls.Config, error) {
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("чтение CA-сертификата: %w", err)
	}

	caCertPool, err := x509.SystemCertPool()
	if err != nil {
		caCertPool = x509.NewCertPool()
	}
	caCertPool.AppendCertsFromPEM(caCert)

	return &tls.Config{
		RootCAs: caCertPool,
	}, nil
}

// normalizeURL убирает trailing slash из URL.
func normalizeURL(rawURL string) string {
	return strings.TrimRight(rawURL, "/")
}
