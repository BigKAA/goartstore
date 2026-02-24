package service

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestNewDephealthService_ValidURL(t *testing.T) {
	// Создаём mock HTTP-сервер для JWKS
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	defer mockServer.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Используем изолированный Prometheus registry для тестов
	reg := prometheus.NewRegistry()

	ds, err := NewDephealthServiceWithRegisterer(
		"test-se-01",
		"storage-element",
		"admin-jwks",
		mockServer.URL,
		5*time.Second,
		false,
		logger,
		reg,
	)

	if err != nil {
		t.Fatalf("Ошибка создания DephealthService: %v", err)
	}
	if ds == nil {
		t.Fatal("DephealthService nil")
	}
}

func TestDephealthService_StartStop(t *testing.T) {
	// Создаём mock HTTP-сервер для JWKS
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	defer mockServer.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	reg := prometheus.NewRegistry()

	ds, err := NewDephealthServiceWithRegisterer(
		"test-se-02",
		"storage-element",
		"admin-jwks",
		mockServer.URL,
		1*time.Second,
		false,
		logger,
		reg,
	)
	if err != nil {
		t.Fatalf("Ошибка создания DephealthService: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start не должен блокировать
	if err := ds.Start(ctx); err != nil {
		t.Fatalf("Ошибка запуска: %v", err)
	}

	// Даём время на первую проверку (интервал 1s + запас)
	time.Sleep(3 * time.Second)

	// Health возвращает map с ключами формата "dependency:host:port"
	health := ds.Health()
	if health == nil {
		t.Fatal("Health() вернул nil")
	}

	// Ищем запись, содержащую "admin-jwks"
	found := false
	for key, val := range health {
		if strings.HasPrefix(key, "admin-jwks:") {
			found = true
			if !val {
				t.Errorf("admin-jwks health = false для ключа %q, ожидалось true", key)
			}
			break
		}
	}
	if !found {
		t.Errorf("Нет записи для admin-jwks в Health(), keys=%v", healthKeys(health))
	}

	// Stop не должен паниковать
	ds.Stop()
}

func TestDephealthService_UnhealthyDependency(t *testing.T) {
	// Сервер, который возвращает 500
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockServer.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	reg := prometheus.NewRegistry()

	ds, err := NewDephealthServiceWithRegisterer(
		"test-se-03",
		"storage-element",
		"admin-jwks",
		mockServer.URL,
		1*time.Second,
		false,
		logger,
		reg,
	)
	if err != nil {
		t.Fatalf("Ошибка создания DephealthService: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := ds.Start(ctx); err != nil {
		t.Fatalf("Ошибка запуска: %v", err)
	}

	// Даём время на первую проверку
	time.Sleep(3 * time.Second)

	health := ds.Health()

	// Ищем запись admin-jwks
	found := false
	for key, val := range health {
		if strings.HasPrefix(key, "admin-jwks:") {
			found = true
			if val {
				t.Errorf("admin-jwks health = true для ключа %q, ожидалось false (сервер 500)", key)
			}
			break
		}
	}
	if !found {
		t.Errorf("Нет записи для admin-jwks в Health(), keys=%v", healthKeys(health))
	}

	ds.Stop()
}

// healthKeys возвращает ключи карты health для вывода в сообщениях об ошибках.
func healthKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
