// readiness.go — адаптеры readiness checkers для HealthHandler.
// Оборачивают adminclient.CheckHealth в интерфейс ReadinessChecker.
package handlers

import (
	"context"

	"github.com/bigkaa/goartstore/ingester-module/internal/adminclient"
)

// AdminModuleChecker — ReadinessChecker для Admin Module.
// Проверяет доступность AM через GET /health/ready.
type AdminModuleChecker struct {
	client *adminclient.Client
}

// NewAdminModuleChecker создаёт checker для Admin Module.
func NewAdminModuleChecker(client *adminclient.Client) *AdminModuleChecker {
	return &AdminModuleChecker{client: client}
}

// CheckReady проверяет готовность Admin Module.
func (c *AdminModuleChecker) CheckReady() (status, message string) {
	err := c.client.CheckHealth(context.Background())
	if err != nil {
		return "fail", "Admin Module недоступен: " + err.Error()
	}
	return "ok", "Admin Module доступен"
}
