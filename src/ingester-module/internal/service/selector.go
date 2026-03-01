// selector.go — алгоритм Sequential Fill для выбора Storage Element.
//
// Логика выбора:
//  1. Определить mode по retention_policy (temporary → edit, permanent → rw)
//  2. Запросить список online SE из Admin Module
//  3. Отсортировать по Priority ASC, при равном priority — по Name ASC
//  4. Выбрать первый SE с достаточным available_bytes, не в excludedIDs
//  5. Если подходящего SE нет → ErrNoStorageAvailable
package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/bigkaa/goartstore/ingester-module/internal/adminclient"
)

// ErrNoStorageAvailable — нет подходящих Storage Elements для загрузки.
var ErrNoStorageAvailable = fmt.Errorf("нет доступных Storage Elements с достаточным местом")

// Prometheus-метрика выбора SE.
var seSelectionTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "im_se_selection_total",
	Help: "Количество операций выбора SE (по результату).",
}, []string{"result"})

// SelectorService — сервис выбора Storage Element по алгоритму Sequential Fill.
type SelectorService struct {
	adminClient *adminclient.Client
}

// NewSelectorService создаёт сервис выбора SE.
func NewSelectorService(adminClient *adminclient.Client) *SelectorService {
	return &SelectorService{
		adminClient: adminClient,
	}
}

// SelectSE выбирает Storage Element для загрузки файла.
//
// Алгоритм Sequential Fill:
//   - Определяет mode по retentionPolicy: "temporary" → "edit", "permanent" → "rw"
//   - Запрашивает online SE из AM
//   - Сортирует по Priority ASC, Name ASC
//   - Выбирает первый SE с AvailableBytes >= fileSize, не в excludedSEIDs
//
// fileSize — размер загружаемого файла в байтах.
// retentionPolicy — "temporary" или "permanent".
// excludedSEIDs — ID SE, исключённых из выбора (после 507).
func (s *SelectorService) SelectSE(
	ctx context.Context,
	fileSize int64,
	retentionPolicy string,
	excludedSEIDs []string,
) (*adminclient.SEInfo, error) {
	// 1. Определяем mode по retention policy
	mode := retentionPolicyToMode(retentionPolicy)

	// 2. Запрашиваем online SE из Admin Module
	seList, err := s.adminClient.GetStorageElements(ctx, mode, "online")
	if err != nil {
		return nil, fmt.Errorf("получение списка SE: %w", err)
	}

	if len(seList) == 0 {
		seSelectionTotal.WithLabelValues("no_storage").Inc()
		return nil, ErrNoStorageAvailable
	}

	// 3. Сортируем по Priority ASC, при равном priority — по Name ASC
	sort.Slice(seList, func(i, j int) bool {
		if seList[i].Priority != seList[j].Priority {
			return seList[i].Priority < seList[j].Priority
		}
		return seList[i].Name < seList[j].Name
	})

	// Формируем map исключённых SE для O(1) lookup
	excludeMap := make(map[string]struct{}, len(excludedSEIDs))
	for _, id := range excludedSEIDs {
		excludeMap[id] = struct{}{}
	}

	// 4. Выбираем первый подходящий SE
	for i := range seList {
		se := &seList[i]

		// Пропускаем исключённые SE (после 507)
		if _, excluded := excludeMap[se.ID]; excluded {
			continue
		}

		// Пропускаем SE без данных о доступном месте
		if se.AvailableBytes == nil {
			continue
		}

		// Проверяем достаточность места
		if *se.AvailableBytes >= fileSize {
			seSelectionTotal.WithLabelValues("success").Inc()
			return se, nil
		}
	}

	// 5. Нет подходящего SE
	seSelectionTotal.WithLabelValues("no_storage").Inc()
	return nil, ErrNoStorageAvailable
}

// retentionPolicyToMode конвертирует retention policy в mode SE.
// temporary → edit (файлы с TTL загружаются в edit SE)
// permanent → rw (постоянные файлы — в read-write SE)
func retentionPolicyToMode(retentionPolicy string) string {
	switch retentionPolicy {
	case "temporary":
		return "edit"
	case "permanent":
		return "rw"
	default:
		return "rw"
	}
}
