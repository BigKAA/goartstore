package service

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/bigkaa/goartstore/demo-client/internal/activity"
	"github.com/bigkaa/goartstore/demo-client/internal/gateway"
	"github.com/bigkaa/goartstore/demo-client/internal/token"
)

// DashboardService — сервис для дашборда: health-checks, статистика, токен.
type DashboardService struct {
	gw       *gateway.Client
	tokenMgr *token.Manager
	log      *activity.Log
	logger   *slog.Logger
}

// NewDashboardService создаёт новый сервис дашборда.
func NewDashboardService(gw *gateway.Client, tokenMgr *token.Manager, log *activity.Log, logger *slog.Logger) *DashboardService {
	return &DashboardService{
		gw:       gw,
		tokenMgr: tokenMgr,
		log:      log,
		logger:   logger,
	}
}

// DashboardData — данные для отображения на дашборде.
type DashboardData struct {
	// Health — статусы backend-сервисов.
	Health map[string]gateway.HealthStatus
	// Token — информация о текущем SA токене.
	Token token.Status
	// Stats — статистика по файлам.
	Stats FileStats
	// RecentActivity — последние записи Activity Log.
	RecentActivity []activity.Entry
}

// FileStats — статистика по файлам в системе.
type FileStats struct {
	Total     int
	Temporary int
	Permanent int
}

// GetDashboardData собирает все данные для дашборда.
func (s *DashboardService) GetDashboardData() *DashboardData {
	data := &DashboardData{
		Token:          s.tokenMgr.TokenInfo(),
		RecentActivity: s.log.List(),
	}

	// Health-checks.
	data.Health = s.gw.HealthCheck()

	// Статистика по файлам — серия search-запросов с count.
	data.Stats = s.getFileStats()

	return data
}

// GetHealth возвращает актуальные health-статусы.
func (s *DashboardService) GetHealth() map[string]gateway.HealthStatus {
	start := time.Now()
	result := s.gw.HealthCheck()
	duration := time.Since(start)

	s.log.Append(activity.Entry{
		Timestamp:   time.Now(),
		Method:      "GET",
		Path:        "/health",
		StatusCode:  200,
		DurationMs:  duration.Milliseconds(),
		Description: "проверка здоровья сервисов",
	})

	return result
}

// GetTokenInfo возвращает информацию о текущем SA токене.
func (s *DashboardService) GetTokenInfo() token.Status {
	return s.tokenMgr.TokenInfo()
}

// getFileStats собирает статистику по файлам через search API.
// Выполняет запросы для подсчёта общего количества, temporary и permanent файлов.
func (s *DashboardService) getFileStats() FileStats {
	stats := FileStats{}

	// Общее количество файлов.
	if resp, err := s.gw.Search(gateway.SearchRequest{
		Limit: 1,
	}); err == nil {
		stats.Total = resp.Total
	}

	// Temporary файлы.
	if resp, err := s.gw.Search(gateway.SearchRequest{
		RetentionPolicy: "temporary",
		Limit:           1,
	}); err == nil {
		stats.Temporary = resp.Total
	}

	// Permanent файлы.
	if resp, err := s.gw.Search(gateway.SearchRequest{
		RetentionPolicy: "permanent",
		Limit:           1,
	}); err == nil {
		stats.Permanent = resp.Total
	}

	s.logger.Debug("статистика файлов",
		"total", stats.Total,
		"temporary", stats.Temporary,
		"permanent", stats.Permanent,
	)

	// Записываем в Activity Log.
	s.log.Append(activity.Entry{
		Timestamp:   time.Now(),
		Method:      "POST",
		Path:        "/query/api/v1/search",
		StatusCode:  200,
		Description: fmt.Sprintf("статистика файлов: всего %d", stats.Total),
	})

	return stats
}
