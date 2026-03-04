// Package token — Token Manager для автоматического получения и обновления
// SA JWT-токена через Client Credentials flow (Keycloak).
//
// Фоновая горутина обновляет токен до истечения срока действия.
// Потокобезопасное чтение текущего токена через GetToken().
package token

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// --- Prometheus метрики ---

var tokenRefreshesTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "dc_token_refreshes_total",
		Help: "Количество попыток обновления SA-токена",
	},
	[]string{"status"}, // success / error
)

// TokenStatus — информация о текущем состоянии токена (для UI).
type TokenStatus struct {
	Valid    bool      `json:"valid"`     // Токен валиден (не истёк)
	Expiry  time.Time `json:"expiry"`    // Время истечения
	ClientID string   `json:"client_id"` // ID клиента
}

// tokenData — внутреннее хранилище токена (atomic).
type tokenData struct {
	accessToken string
	expiry      time.Time
}

// Manager — Token Manager для Client Credentials flow.
type Manager struct {
	// Конфигурация
	tokenURL     string
	clientID     string
	clientSecret string
	scopes       []string
	refreshBefore time.Duration

	// HTTP-клиент для запросов к Keycloak
	httpClient *http.Client

	// Токен — atomic read/write
	current atomic.Pointer[tokenData]

	// Управление жизненным циклом
	cancel context.CancelFunc
	wg     sync.WaitGroup
	logger *slog.Logger
}

// tokenResponse — ответ Keycloak на запрос токена.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"` // секунды до истечения
	TokenType   string `json:"token_type"`
}

// NewManager — создание нового Token Manager.
// httpClient используется для запросов к Keycloak (с TLS настройками).
func NewManager(
	tokenURL, clientID, clientSecret string,
	scopes []string,
	refreshBefore time.Duration,
	httpClient *http.Client,
	logger *slog.Logger,
) *Manager {
	m := &Manager{
		tokenURL:      tokenURL,
		clientID:      clientID,
		clientSecret:  clientSecret,
		scopes:        scopes,
		refreshBefore: refreshBefore,
		httpClient:    httpClient,
		logger:        logger.With("component", "token-manager"),
	}
	// Начальное значение — пустой токен с истёкшим сроком
	m.current.Store(&tokenData{})
	return m
}

// Start — запуск фоновой горутины для обновления токена.
// Первое обновление выполняется немедленно.
func (m *Manager) Start(ctx context.Context) {
	ctx, m.cancel = context.WithCancel(ctx)
	m.wg.Add(1)
	go m.refreshLoop(ctx)
}

// Stop — остановка фоновой горутины.
func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
}

// GetToken — атомарное чтение текущего access token.
// Возвращает пустую строку если токен ещё не получен.
func (m *Manager) GetToken() string {
	data := m.current.Load()
	if data == nil {
		return ""
	}
	return data.accessToken
}

// TokenInfo — информация о токене для отображения в UI.
func (m *Manager) TokenInfo() TokenStatus {
	data := m.current.Load()
	if data == nil {
		return TokenStatus{ClientID: m.clientID}
	}
	return TokenStatus{
		Valid:    time.Now().Before(data.expiry),
		Expiry:   data.expiry,
		ClientID: m.clientID,
	}
}

// IsReady — проверка что токен валиден (для health-check).
func (m *Manager) IsReady() bool {
	data := m.current.Load()
	if data == nil || data.accessToken == "" {
		return false
	}
	return time.Now().Before(data.expiry)
}

// refreshLoop — фоновый цикл обновления токена с exponential backoff.
func (m *Manager) refreshLoop(ctx context.Context) {
	defer m.wg.Done()

	// Параметры exponential backoff
	const (
		initialBackoff = 5 * time.Second
		maxBackoff     = 5 * time.Minute
		backoffFactor  = 2
	)

	backoff := initialBackoff

	for {
		// Попытка обновления токена
		err := m.refreshToken(ctx)
		if err != nil {
			tokenRefreshesTotal.WithLabelValues("error").Inc()
			m.logger.Error("ошибка обновления токена",
				"error", err,
				"retry_in", backoff,
			)

			// Ждём backoff перед повторной попыткой
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}

			// Увеличиваем backoff (exponential)
			backoff *= time.Duration(backoffFactor)
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		// Успешное обновление — сброс backoff
		tokenRefreshesTotal.WithLabelValues("success").Inc()
		backoff = initialBackoff

		// Вычисляем время до следующего обновления
		data := m.current.Load()
		sleepDuration := time.Until(data.expiry) - m.refreshBefore
		if sleepDuration < initialBackoff {
			sleepDuration = initialBackoff
		}

		m.logger.Info("токен обновлён",
			"expires_at", data.expiry.Format(time.RFC3339),
			"next_refresh_in", sleepDuration,
		)

		// Ждём до следующего обновления
		select {
		case <-ctx.Done():
			return
		case <-time.After(sleepDuration):
		}
	}
}

// refreshToken — запрос нового токена через Client Credentials flow.
func (m *Manager) refreshToken(ctx context.Context) error {
	// Формируем запрос
	formData := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {m.clientID},
		"client_secret": {m.clientSecret},
	}
	if len(m.scopes) > 0 {
		formData.Set("scope", strings.Join(m.scopes, " "))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.tokenURL,
		strings.NewReader(formData.Encode()))
	if err != nil {
		return fmt.Errorf("создание запроса: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Выполняем запрос
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("запрос к token endpoint: %w", err)
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("чтение ответа: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token endpoint вернул %d: %s", resp.StatusCode, string(body))
	}

	// Парсим ответ
	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("парсинг ответа: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return fmt.Errorf("пустой access_token в ответе")
	}

	// Сохраняем новый токен
	m.current.Store(&tokenData{
		accessToken: tokenResp.AccessToken,
		expiry:      time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	})

	return nil
}
