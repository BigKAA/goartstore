// Пакет index — потокобезопасный in-memory индекс метаданных файлов.
//
// Индекс строится при старте из attr.json файлов (BuildFromDir)
// и обновляется синхронно при операциях записи (Add, Update, Remove).
// Обеспечивает быструю фильтрацию, пагинацию и подсчёт
// без обращения к диску.
//
// Не персистентный: при рестарте пересобирается из attr.json.
// Потребление памяти: ~500 байт/файл, 100K файлов ≈ 50 МБ.
package index

import (
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/arturkryukov/artstore/storage-element/internal/domain/model"
	"github.com/arturkryukov/artstore/storage-element/internal/storage/attr"
)

// Index — потокобезопасный in-memory индекс метаданных.
// Использует sync.RWMutex для конкурентного чтения и
// эксклюзивной записи.
type Index struct {
	mu     sync.RWMutex
	files  map[string]*model.FileMetadata // file_id → metadata
	ready  bool                           // индекс построен и готов
	logger *slog.Logger
}

// New создаёт пустой индекс. Для заполнения вызовите BuildFromDir.
func New(logger *slog.Logger) *Index {
	return &Index{
		files:  make(map[string]*model.FileMetadata),
		logger: logger.With(slog.String("component", "index")),
	}
}

// BuildFromDir строит индекс из attr.json файлов в указанной директории.
// Вызывается при старте сервера. Заменяет текущее содержимое индекса.
// После успешного построения индекс помечается как ready.
func (idx *Index) BuildFromDir(dataDir string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Сканируем все attr.json файлы
	metadatas, err := attr.ScanDir(dataDir)
	if err != nil {
		return fmt.Errorf("ошибка сканирования директории %s: %w", dataDir, err)
	}

	// Очищаем текущий индекс и заполняем новыми данными
	idx.files = make(map[string]*model.FileMetadata, len(metadatas))
	for _, meta := range metadatas {
		idx.files[meta.FileID] = meta
	}

	idx.ready = true

	idx.logger.Info("Индекс метаданных построен",
		slog.Int("files", len(idx.files)),
		slog.String("data_dir", dataDir),
	)

	return nil
}

// RebuildFromDir полностью пересобирает индекс из attr.json.
// Аналогичен BuildFromDir, но используется при reconciliation.
func (idx *Index) RebuildFromDir(dataDir string) error {
	return idx.BuildFromDir(dataDir)
}

// IsReady возвращает true, если индекс построен и готов к использованию.
func (idx *Index) IsReady() bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.ready
}

// Add добавляет метаданные файла в индекс.
// Если файл с таким ID уже существует, он будет перезаписан.
func (idx *Index) Add(meta *model.FileMetadata) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Создаём копию, чтобы избежать data race при внешних изменениях
	copied := *meta
	idx.files[meta.FileID] = &copied
}

// Update обновляет метаданные файла в индексе.
// Возвращает ошибку, если файл не найден.
func (idx *Index) Update(meta *model.FileMetadata) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if _, ok := idx.files[meta.FileID]; !ok {
		return fmt.Errorf("файл %s не найден в индексе", meta.FileID)
	}

	copied := *meta
	idx.files[meta.FileID] = &copied
	return nil
}

// Remove удаляет файл из индекса по file_id.
// Возвращает true, если файл был найден и удалён.
func (idx *Index) Remove(fileID string) bool {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if _, ok := idx.files[fileID]; !ok {
		return false
	}
	delete(idx.files, fileID)
	return true
}

// Get возвращает метаданные файла по file_id.
// Возвращает nil, если файл не найден.
func (idx *Index) Get(fileID string) *model.FileMetadata {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	meta, ok := idx.files[fileID]
	if !ok {
		return nil
	}

	// Возвращаем копию для потокобезопасности
	copied := *meta
	return &copied
}

// List возвращает пагинированный список метаданных с опциональной фильтрацией по статусу.
// Параметры:
//   - limit: максимальное количество элементов (0 = все)
//   - offset: смещение от начала списка
//   - statusFilter: фильтр по статусу ("" = без фильтра)
//
// Возвращает срез метаданных и общее количество файлов (с учётом фильтра).
// Файлы отсортированы по дате загрузки (новые первые).
func (idx *Index) List(limit, offset int, statusFilter model.FileStatus) ([]*model.FileMetadata, int) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Собираем все файлы с учётом фильтра
	var filtered []*model.FileMetadata
	for _, meta := range idx.files {
		if statusFilter != "" && meta.Status != statusFilter {
			continue
		}
		copied := *meta
		filtered = append(filtered, &copied)
	}

	// Сортируем по дате загрузки (новые первые)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].UploadedAt.After(filtered[j].UploadedAt)
	})

	total := len(filtered)

	// Применяем пагинацию
	if offset >= total {
		return nil, total
	}

	end := total
	if limit > 0 && offset+limit < total {
		end = offset + limit
	}

	return filtered[offset:end], total
}

// Count возвращает общее количество файлов в индексе.
func (idx *Index) Count() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.files)
}

// CountByStatus возвращает количество файлов с указанным статусом.
func (idx *Index) CountByStatus(status model.FileStatus) int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	count := 0
	for _, meta := range idx.files {
		if meta.Status == status {
			count++
		}
	}
	return count
}
