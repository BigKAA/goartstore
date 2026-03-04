// delete.go — сервис удаления файлов: получение SE URL → удаление из SE → удаление из AM.
//
// Pipeline (3 шага):
//  1. Получить информацию о SE из Admin Module (URL, режим)
//  2. Удалить файл из Storage Element (DELETE /api/v1/files/{fileID})
//  3. Удалить запись файла из Admin Module (DELETE /api/v1/files/{fileID})
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/bigkaa/goartstore/ingester-module/internal/adminclient"
	"github.com/bigkaa/goartstore/ingester-module/internal/seclient"
)

// Ошибки delete service.
var (
	// ErrSENotFound — SE не найден в реестре Admin Module.
	ErrSENotFound = fmt.Errorf("storage element не найден в реестре")

	// ErrDeleteFileNotFound — файл не найден на SE.
	ErrDeleteFileNotFound = fmt.Errorf("файл не найден на Storage Element")

	// ErrDeleteModeNotAllowed — SE не в режиме edit (удаление запрещено).
	ErrDeleteModeNotAllowed = fmt.Errorf("удаление возможно только для SE в режиме edit")

	// ErrDeleteUploadInProgress — файл в процессе загрузки.
	ErrDeleteUploadInProgress = fmt.Errorf("файл находится в процессе загрузки")

	// ErrSEDeleteFailed — ошибка удаления файла на SE.
	ErrSEDeleteFailed = fmt.Errorf("ошибка удаления файла из Storage Element")
)

// Prometheus-метрики удаления.
var (
	deletesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "im_deletes_total",
		Help: "Общее количество операций удаления (по статусу).",
	}, []string{"status"})

	deleteDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "im_delete_duration_seconds",
		Help:    "Длительность полного delete pipeline.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10},
	})
)

// DeleteService — координатор delete pipeline.
type DeleteService struct {
	adminClient *adminclient.Client
	seClient    *seclient.Client
	logger      *slog.Logger
}

// NewDeleteService создаёт delete service.
func NewDeleteService(
	adminClient *adminclient.Client,
	seClient *seclient.Client,
	logger *slog.Logger,
) *DeleteService {
	return &DeleteService{
		adminClient: adminClient,
		seClient:    seClient,
		logger:      logger.With(slog.String("component", "delete_service")),
	}
}

// Delete выполняет полный delete pipeline.
//
// Pipeline:
//  1. Получить информацию о SE из Admin Module
//  2. Удалить файл из SE
//  3. Удалить запись файла из Admin Module
func (s *DeleteService) Delete(ctx context.Context, fileID, storageElementID string) error {
	start := time.Now()

	// 1. Получаем информацию о SE из Admin Module
	seInfo, err := s.adminClient.GetStorageElement(ctx, storageElementID)
	if err != nil {
		deletesTotal.WithLabelValues("error").Inc()
		s.logger.Error("Ошибка получения информации о SE",
			slog.String("file_id", fileID),
			slog.String("se_id", storageElementID),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("%w: %s", ErrSENotFound, err.Error())
	}

	// 2. Удаляем файл из SE
	if deleteErr := s.seClient.Delete(ctx, seInfo.URL, fileID); deleteErr != nil {
		deletesTotal.WithLabelValues("error").Inc()
		return s.mapSEDeleteError(deleteErr, fileID, seInfo)
	}

	// 3. Удаляем запись файла из Admin Module
	if amErr := s.adminClient.DeleteFile(ctx, fileID); amErr != nil {
		// Файл уже удалён на SE, но не в реестре AM — логируем как warning
		s.logger.Warn("Файл удалён из SE, но ошибка удаления из реестра AM",
			slog.String("file_id", fileID),
			slog.String("se_id", storageElementID),
			slog.String("error", amErr.Error()),
		)
		// Не возвращаем ошибку — файл физически удалён, реестр AM
		// будет очищен при следующей синхронизации или вручную.
	}

	// Метрики успеха
	duration := time.Since(start)
	deletesTotal.WithLabelValues("success").Inc()
	deleteDuration.Observe(duration.Seconds())

	s.logger.Info("Delete завершён успешно",
		slog.String("file_id", fileID),
		slog.String("se_id", storageElementID),
		slog.String("se_url", seInfo.URL),
		slog.Duration("duration", duration),
	)

	return nil
}

// mapSEDeleteError маппит ошибки SE client на ошибки delete service.
func (s *DeleteService) mapSEDeleteError(err error, fileID string, seInfo *adminclient.SEInfo) error {
	s.logger.Error("Ошибка удаления файла из SE",
		slog.String("file_id", fileID),
		slog.String("se_id", seInfo.ID),
		slog.String("se_url", seInfo.URL),
		slog.String("error", err.Error()),
	)

	switch {
	case errors.Is(err, seclient.ErrFileNotFound):
		return ErrDeleteFileNotFound
	case errors.Is(err, seclient.ErrModeNotAllowed):
		return ErrDeleteModeNotAllowed
	case errors.Is(err, seclient.ErrFileUploadInProgress):
		return ErrDeleteUploadInProgress
	default:
		return fmt.Errorf("%w: %s", ErrSEDeleteFailed, err.Error())
	}
}
