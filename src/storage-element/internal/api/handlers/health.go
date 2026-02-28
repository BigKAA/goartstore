// health.go — обработчики health endpoints для Kubernetes probes.
package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/bigkaa/goartstore/storage-element/internal/config"
)

// statusFail — строковая константа для статуса "fail" в health checks.
const statusFail = "fail"

// IndexReadinessChecker — интерфейс для проверки готовности индекса.
type IndexReadinessChecker interface {
	IsReady() bool
}

// HealthHandler реализует health endpoints: /health/live, /health/ready.
type HealthHandler struct {
	version string
	// dataDir — путь к директории данных (для проверки FS)
	dataDir string
	// walDir — путь к директории WAL (для проверки WAL)
	walDir string
	// idx — ссылка на индекс для проверки готовности
	idx IndexReadinessChecker
	// roleProvider — провайдер роли для проверки leader connection (follower only)
	roleProvider RoleProvider
}

// NewHealthHandler создаёт обработчик health endpoints.
// Без параметров — базовая проверка (для обратной совместимости).
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{
		version: config.Version,
	}
}

// NewHealthHandlerFull создаёт обработчик health endpoints с реальными проверками.
// roleProvider — провайдер роли (nil для standalone).
func NewHealthHandlerFull(dataDir, walDir string, idx IndexReadinessChecker, roleProvider RoleProvider) *HealthHandler {
	return &HealthHandler{
		version:      config.Version,
		dataDir:      dataDir,
		walDir:       walDir,
		idx:          idx,
		roleProvider: roleProvider,
	}
}

// HealthLive обрабатывает GET /health/live.
// Возвращает 200, если процесс SE жив. Не проверяет зависимости.
func (h *HealthHandler) HealthLive(w http.ResponseWriter, _ *http.Request) {
	resp := map[string]any{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   h.version,
		"service":   "storage-element",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// HealthReady обрабатывает GET /health/ready.
// Проверяет: файловая система, WAL директория, готовность индекса.
func (h *HealthHandler) HealthReady(w http.ResponseWriter, _ *http.Request) {
	overallStatus := "ok"
	httpStatus := http.StatusOK

	// Проверка файловой системы
	fsCheck := h.checkFilesystem()
	if fsCheck["status"] != "ok" {
		overallStatus = statusFail
		httpStatus = http.StatusServiceUnavailable
	}

	// Проверка WAL
	walCheck := h.checkWAL()
	if walCheck["status"] != "ok" {
		if overallStatus != statusFail {
			overallStatus = "degraded"
		}
	}

	// Проверка индекса
	indexReady := true
	if h.idx != nil {
		indexReady = h.idx.IsReady()
	}
	if !indexReady {
		overallStatus = statusFail
		httpStatus = http.StatusServiceUnavailable
	}

	checks := map[string]any{
		"filesystem": fsCheck,
		"wal":        walCheck,
	}

	// Проверка leader connection (только для follower в replicated mode)
	if h.roleProvider != nil && !h.roleProvider.IsLeader() {
		leaderCheck := h.checkLeaderConnection()
		checks["leader_connection"] = leaderCheck
		if leaderCheck["status"] != "ok" {
			if overallStatus != statusFail {
				overallStatus = "degraded"
			}
		}
	}

	resp := map[string]any{
		"status":    overallStatus,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   h.version,
		"service":   "storage-element",
		"checks":    checks,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(resp)
}

// checkFilesystem проверяет доступность директории данных на запись.
func (h *HealthHandler) checkFilesystem() map[string]any {
	if h.dataDir == "" {
		return map[string]any{
			"status":  "ok",
			"message": "Проверка не настроена",
		}
	}

	testFile := filepath.Join(h.dataDir, ".health_check")
	if err := os.WriteFile(testFile, []byte("ok"), 0o600); err != nil {
		return map[string]any{
			"status":  statusFail,
			"message": "Директория данных недоступна для записи: " + err.Error(),
		}
	}
	_ = os.Remove(testFile)

	return map[string]any{
		"status": "ok",
	}
}

// checkLeaderConnection проверяет, известен ли адрес leader (для follower).
func (h *HealthHandler) checkLeaderConnection() map[string]any {
	if h.roleProvider == nil {
		return map[string]any{
			"status":  "ok",
			"message": "Проверка не применима",
		}
	}

	addr := h.roleProvider.LeaderAddr()
	if addr == "" {
		return map[string]any{
			"status":  statusFail,
			"message": "Адрес leader неизвестен",
		}
	}

	return map[string]any{
		"status":      "ok",
		"leader_addr": addr,
	}
}

// checkWAL проверяет доступность директории WAL на запись.
func (h *HealthHandler) checkWAL() map[string]any {
	if h.walDir == "" {
		return map[string]any{
			"status":  "ok",
			"message": "Проверка не настроена",
		}
	}

	testFile := filepath.Join(h.walDir, ".health_check")
	if err := os.WriteFile(testFile, []byte("ok"), 0o600); err != nil {
		return map[string]any{
			"status":  statusFail,
			"message": "Директория WAL недоступна для записи: " + err.Error(),
		}
	}
	_ = os.Remove(testFile)

	return map[string]any{
		"status": "ok",
	}
}
