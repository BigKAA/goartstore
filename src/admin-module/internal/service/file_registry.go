// file_registry.go — сервис файлового реестра.
// CRUD файлов: регистрация, получение, обновление, soft delete.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/arturkryukov/artstore/admin-module/internal/domain/model"
	"github.com/arturkryukov/artstore/admin-module/internal/repository"
)

// FileRegistryService — сервис файлового реестра.
type FileRegistryService struct {
	fileRepo repository.FileRegistryRepository
	seRepo   repository.StorageElementRepository
	logger   *slog.Logger
}

// NewFileRegistryService создаёт сервис файлового реестра.
func NewFileRegistryService(
	fileRepo repository.FileRegistryRepository,
	seRepo repository.StorageElementRepository,
	logger *slog.Logger,
) *FileRegistryService {
	return &FileRegistryService{
		fileRepo: fileRepo,
		seRepo:   seRepo,
		logger:   logger.With(slog.String("component", "file_registry_service")),
	}
}

// Register регистрирует файл в реестре.
// Проверяет существование SE перед регистрацией.
func (s *FileRegistryService) Register(ctx context.Context, f *model.FileRecord) error {
	// Проверяем существование Storage Element
	_, err := s.seRepo.GetByID(ctx, f.StorageElementID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return fmt.Errorf("%w: Storage Element '%s' не найден", ErrValidation, f.StorageElementID)
		}
		return fmt.Errorf("проверка SE: %w", err)
	}

	// Устанавливаем значения по умолчанию
	if f.Status == "" {
		f.Status = "active"
	}
	if f.UploadedAt.IsZero() {
		f.UploadedAt = time.Now().UTC()
	}

	// Вычисляем expires_at для temporary файлов
	if f.RetentionPolicy == "temporary" && f.TTLDays != nil && *f.TTLDays > 0 {
		expiresAt := f.UploadedAt.AddDate(0, 0, *f.TTLDays)
		f.ExpiresAt = &expiresAt
	}

	// Регистрируем файл
	if err := s.fileRepo.Register(ctx, f); err != nil {
		if errors.Is(err, repository.ErrConflict) {
			return fmt.Errorf("%w: файл с ID '%s' уже зарегистрирован", ErrConflict, f.FileID)
		}
		return fmt.Errorf("регистрация файла: %w", err)
	}

	s.logger.Info("Файл зарегистрирован",
		slog.String("file_id", f.FileID),
		slog.String("storage_element_id", f.StorageElementID),
		slog.String("filename", f.OriginalFilename),
	)

	return nil
}

// List возвращает список файлов с фильтрацией и пагинацией.
func (s *FileRegistryService) List(ctx context.Context, filters repository.FileListFilters, limit, offset int) ([]*model.FileRecord, int, error) {
	files, err := s.fileRepo.List(ctx, filters, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("получение списка файлов: %w", err)
	}

	total, err := s.fileRepo.Count(ctx, filters)
	if err != nil {
		return nil, 0, fmt.Errorf("подсчёт файлов: %w", err)
	}

	return files, total, nil
}

// Get возвращает файл по ID.
func (s *FileRegistryService) Get(ctx context.Context, fileID string) (*model.FileRecord, error) {
	f, err := s.fileRepo.GetByID(ctx, fileID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("получение файла: %w", err)
	}
	return f, nil
}

// Update обновляет метаданные файла (description, tags, status).
func (s *FileRegistryService) Update(ctx context.Context, fileID string, description *string, tags *[]string, status *string) (*model.FileRecord, error) {
	// Получаем текущий файл
	f, err := s.fileRepo.GetByID(ctx, fileID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("получение файла для обновления: %w", err)
	}

	// Применяем обновления
	if description != nil {
		f.Description = description
	}
	if tags != nil {
		f.Tags = *tags
	}
	if status != nil {
		f.Status = *status
	}

	// Обновляем в БД
	if err := s.fileRepo.Update(ctx, f); err != nil {
		return nil, fmt.Errorf("обновление файла: %w", err)
	}

	s.logger.Info("Файл обновлён",
		slog.String("file_id", fileID),
	)

	return f, nil
}

// Delete выполняет soft delete файла (status → deleted).
func (s *FileRegistryService) Delete(ctx context.Context, fileID string) error {
	if err := s.fileRepo.Delete(ctx, fileID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("soft delete файла: %w", err)
	}

	s.logger.Info("Файл помечен как удалённый",
		slog.String("file_id", fileID),
	)

	return nil
}
