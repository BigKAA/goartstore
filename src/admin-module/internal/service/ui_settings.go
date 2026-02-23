// ui_settings.go — сервис управления настройками Admin UI.
// Предоставляет типизированные геттеры для Prometheus-конфигурации,
// валидацию ключей и CRUD-операции.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/bigkaa/goartstore/admin-module/internal/repository"
)

// Допустимые ключи настроек (dot-notation).
// Используется для валидации при Set.
var validSettingKeys = map[string]string{
	"prometheus.url":              "URL Prometheus сервера",
	"prometheus.enabled":          "Включён ли Prometheus (true/false)",
	"prometheus.timeout":          "Таймаут запросов к Prometheus (например, 10s)",
	"prometheus.retention_period": "Период хранения данных для графиков (например, 7d)",
}

// UISettingsService — сервис для работы с настройками UI.
type UISettingsService struct {
	repo   repository.UISettingsRepository
	logger *slog.Logger
}

// NewUISettingsService создаёт сервис настроек UI.
func NewUISettingsService(
	repo repository.UISettingsRepository,
	logger *slog.Logger,
) *UISettingsService {
	return &UISettingsService{
		repo:   repo,
		logger: logger.With(slog.String("service", "ui_settings")),
	}
}

// Get возвращает значение настройки по ключу.
// Возвращает ErrNotFound если настройка не существует.
func (s *UISettingsService) Get(ctx context.Context, key string) (*repository.UISetting, error) {
	setting, err := s.repo.Get(ctx, key)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("ошибка получения настройки %q: %w", key, err)
	}
	return setting, nil
}

// Set устанавливает значение настройки. Валидирует ключ и значение.
// updatedBy — имя пользователя, выполняющего изменение.
func (s *UISettingsService) Set(ctx context.Context, key, value, updatedBy string) error {
	// Валидация ключа
	if _, ok := validSettingKeys[key]; !ok {
		return fmt.Errorf("%w: недопустимый ключ настройки %q", ErrValidation, key)
	}

	// Валидация значения по типу ключа
	if err := s.validateValue(key, value); err != nil {
		return err
	}

	if err := s.repo.Set(ctx, key, value, updatedBy); err != nil {
		return fmt.Errorf("ошибка сохранения настройки %q: %w", key, err)
	}

	s.logger.Info("Настройка обновлена",
		slog.String("key", key),
		slog.String("updated_by", updatedBy),
	)
	return nil
}

// List возвращает все настройки.
func (s *UISettingsService) List(ctx context.Context) ([]repository.UISetting, error) {
	settings, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения списка настроек: %w", err)
	}
	return settings, nil
}

// Delete удаляет настройку по ключу.
func (s *UISettingsService) Delete(ctx context.Context, key string) error {
	if err := s.repo.Delete(ctx, key); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("ошибка удаления настройки %q: %w", key, err)
	}

	s.logger.Info("Настройка удалена", slog.String("key", key))
	return nil
}

// --- Типизированные геттеры для Prometheus --- //

// GetPrometheusURL возвращает URL Prometheus сервера.
// Возвращает пустую строку, если не настроен.
func (s *UISettingsService) GetPrometheusURL(ctx context.Context) string {
	setting, err := s.repo.Get(ctx, "prometheus.url")
	if err != nil {
		return ""
	}
	return setting.Value
}

// IsPrometheusEnabled возвращает true, если Prometheus включён.
func (s *UISettingsService) IsPrometheusEnabled(ctx context.Context) bool {
	setting, err := s.repo.Get(ctx, "prometheus.enabled")
	if err != nil {
		return false
	}
	return strings.EqualFold(setting.Value, "true")
}

// GetPrometheusTimeout возвращает таймаут для запросов к Prometheus.
// По умолчанию 10 секунд.
func (s *UISettingsService) GetPrometheusTimeout(ctx context.Context) time.Duration {
	setting, err := s.repo.Get(ctx, "prometheus.timeout")
	if err != nil {
		return 10 * time.Second
	}
	d, err := time.ParseDuration(setting.Value)
	if err != nil {
		return 10 * time.Second
	}
	return d
}

// --- Валидация значений --- //

// validateValue проверяет корректность значения для указанного ключа.
func (s *UISettingsService) validateValue(key, value string) error {
	switch key {
	case "prometheus.enabled":
		if value != "true" && value != "false" {
			return fmt.Errorf("%w: %s должен быть true или false", ErrValidation, key)
		}
	case "prometheus.url":
		if value != "" && !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
			return fmt.Errorf("%w: %s должен начинаться с http:// или https://", ErrValidation, key)
		}
	case "prometheus.timeout":
		if value != "" {
			if _, err := time.ParseDuration(value); err != nil {
				return fmt.Errorf("%w: %s — некорректная длительность: %s", ErrValidation, key, value)
			}
		}
	case "prometheus.retention_period":
		if value != "" {
			// Допускаем формат "7d", "24h", "30d" и т.д.
			if _, err := parseDurationExtended(value); err != nil {
				return fmt.Errorf("%w: %s — некорректный период: %s", ErrValidation, key, value)
			}
		}
	}
	return nil
}

// parseDurationExtended расширяет time.ParseDuration, добавляя поддержку суффикса "d" (дни).
func parseDurationExtended(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		numStr := strings.TrimSuffix(s, "d")
		days, err := strconv.Atoi(numStr)
		if err != nil {
			return 0, fmt.Errorf("некорректное число дней: %s", numStr)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
