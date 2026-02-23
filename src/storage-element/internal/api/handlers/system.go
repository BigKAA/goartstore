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
	diskFn       func() (total, used, available int64, err error)
	roleProvider RoleProvider
}

// NewSystemHandler создаёт обработчик системных endpoints.
// diskUsageFn — функция для получения дискового пространства (nil для заглушки).
// roleProvider — провайдер роли (nil для standalone).
func NewSystemHandler(
	cfg *config.Config,
	sm *mode.StateMachine,
	idx *index.Index,
	diskUsageFn func() (total, used, available int64, err error),
	roleProvider RoleProvider,
) *SystemHandler {
	return &SystemHandler{
		cfg:          cfg,
		sm:           sm,
		idx:          idx,
		diskFn:       diskUsageFn,
		roleProvider: roleProvider,
	}
}

// GetStorageInfo обрабатывает GET /api/v1/info.
// Без аутентификации. Возвращает информацию о SE для service discovery.
func (h *SystemHandler) GetStorageInfo(w http.ResponseWriter, r *http.Request) {
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

	// Получаем ёмкость диска
	var capacity generated.CapacityInfo
	if h.diskFn != nil {
		total, used, available, err := h.diskFn()
		if err == nil {
			capacity = generated.CapacityInfo{
				TotalBytes:     total,
				UsedBytes:      used,
				AvailableBytes: available,
			}
		}
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
