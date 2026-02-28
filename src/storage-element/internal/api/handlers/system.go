// system.go — обработчик GET /api/v1/info (информация о Storage Element).
// Публичный endpoint (без аутентификации) для service discovery и мониторинга.
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/bigkaa/goartstore/storage-element/internal/api/generated"
	"github.com/bigkaa/goartstore/storage-element/internal/config"
	"github.com/bigkaa/goartstore/storage-element/internal/domain/mode"
	"github.com/bigkaa/goartstore/storage-element/internal/storage/index"
)

// RoleProvider — интерфейс для получения текущей роли экземпляра SE.
// Используется в system и health handlers для динамического определения роли.
type RoleProvider interface {
	CurrentRole() string
	IsLeader() bool
	LeaderAddr() string
}

// SystemHandler — обработчик системных endpoints.
type SystemHandler struct {
	cfg          *config.Config
	sm           *mode.StateMachine
	idx          *index.Index
	roleProvider RoleProvider
}

// NewSystemHandler создаёт обработчик системных endpoints.
// roleProvider — провайдер роли (nil для standalone).
func NewSystemHandler(
	cfg *config.Config,
	sm *mode.StateMachine,
	idx *index.Index,
	roleProvider RoleProvider,
) *SystemHandler {
	return &SystemHandler{
		cfg:          cfg,
		sm:           sm,
		idx:          idx,
		roleProvider: roleProvider,
	}
}

// GetStorageInfo обрабатывает GET /api/v1/info.
// Без аутентификации. Возвращает информацию о SE для service discovery.
func (h *SystemHandler) GetStorageInfo(w http.ResponseWriter, _ *http.Request) {
	currentMode := h.sm.CurrentMode()

	// Формируем allowed_operations
	ops := h.sm.AllowedOperations()
	apiOps := make([]generated.StorageInfoAllowedOperations, 0, len(ops))
	for _, op := range ops {
		apiOps = append(apiOps, generated.StorageInfoAllowedOperations(op))
	}

	// Определяем статус
	status := generated.StorageInfoStatusOnline
	if !h.idx.IsReady() {
		status = generated.StorageInfoStatusMaintenance
	}

	// Вычисляем ёмкость из сконфигурированного лимита и индекса
	usedBytes := h.idx.TotalActiveSize()
	availableBytes := h.cfg.MaxCapacity - usedBytes
	if availableBytes < 0 {
		availableBytes = 0
	}
	capacity := generated.CapacityInfo{
		TotalBytes:     h.cfg.MaxCapacity,
		UsedBytes:      usedBytes,
		AvailableBytes: availableBytes,
	}

	// Режим развёртывания
	replicaMode := generated.StorageInfoReplicaModeStandalone
	if h.cfg.ReplicaMode == "replicated" {
		replicaMode = generated.StorageInfoReplicaModeReplicated
	}

	// Роль — определяется через RoleProvider (dynamic в replicated mode)
	role := generated.StorageInfoRoleStandalone
	if h.roleProvider != nil {
		role = generated.StorageInfoRole(h.roleProvider.CurrentRole())
	}

	resp := generated.StorageInfo{
		StorageId:         h.cfg.StorageID,
		Mode:              generated.StorageInfoMode(currentMode),
		Status:            status,
		Version:           config.Version,
		AllowedOperations: apiOps,
		Capacity:          capacity,
		ReplicaMode:       &replicaMode,
		Role:              &role,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
