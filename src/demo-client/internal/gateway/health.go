package gateway

import (
	"fmt"
	"net/http"
)

// HealthCheck проверяет доступность backend-сервисов через их health endpoints.
// Возвращает map[serviceName]HealthStatus для Ingester Module и Query Module.
func (c *Client) HealthCheck() map[string]HealthStatus {
	results := make(map[string]HealthStatus)

	// Проверка Ingester Module (через Gateway).
	results["ingester"] = c.checkHealth("/upload/health/ready")

	// Проверка Query Module (через Gateway).
	results["query"] = c.checkHealth("/query/health/ready")

	return results
}

// checkHealth выполняет health-check по указанному пути.
func (c *Client) checkHealth(path string) HealthStatus {
	url := c.baseURL + path
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return HealthStatus{Status: "error", Ready: false}
	}

	// Health endpoints не требуют авторизации, но используем doRequest
	// для метрик и логирования.
	resp, err := c.doRequest(req, false)
	if err != nil {
		return HealthStatus{Status: "error", Ready: false}
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
		return HealthStatus{Status: "ok", Ready: true}
	case resp.StatusCode == http.StatusServiceUnavailable:
		return HealthStatus{
			Status: fmt.Sprintf("unavailable (HTTP %d)", resp.StatusCode),
			Ready:  false,
		}
	default:
		return HealthStatus{
			Status: fmt.Sprintf("unexpected (HTTP %d)", resp.StatusCode),
			Ready:  false,
		}
	}
}
