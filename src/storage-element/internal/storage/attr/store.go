// store.go — адаптер AttrStore для локальной файловой системы.
//
// Store оборачивает package-level функции (Write, Read, Delete, ScanDir)
// в struct с методами, принимающими storagePath (относительный путь DATA-файла).
// Реализация сама вычисляет полный путь к .attr.json.
package attr

import (
	"context"
	"path/filepath"

	"github.com/bigkaa/goartstore/storage-element/internal/backend"
	"github.com/bigkaa/goartstore/storage-element/internal/domain/model"
)

// Compile-time check: Store реализует backend.AttrStore.
var _ backend.AttrStore = (*Store)(nil)

// Store — адаптер для работы с attr.json на локальной файловой системе.
// Принимает storagePath (относительный путь data-файла), сам вычисляет
// путь к .attr.json через AttrFilePath.
type Store struct {
	// dataDir — корневая директория хранения файлов (SE_DATA_DIR)
	dataDir string
}

// NewStore создаёт AttrStore для локальной файловой системы.
func NewStore(dataDir string) *Store {
	return &Store{dataDir: dataDir}
}

// Write атомарно записывает метаданные для data-файла.
// storagePath — относительный путь data-файла (напр. "2026/03/01/photo.jpg").
func (s *Store) Write(_ context.Context, storagePath string, meta *model.FileMetadata) error {
	attrPath := s.attrPath(storagePath)
	return Write(attrPath, meta)
}

// Read читает метаданные для data-файла.
// storagePath — относительный путь data-файла.
func (s *Store) Read(_ context.Context, storagePath string) (*model.FileMetadata, error) {
	attrPath := s.attrPath(storagePath)
	return Read(attrPath)
}

// Delete удаляет метаданные для data-файла. Идемпотентно.
// storagePath — относительный путь data-файла.
func (s *Store) Delete(_ context.Context, storagePath string) error {
	attrPath := s.attrPath(storagePath)
	return Delete(attrPath)
}

// ScanAll возвращает все метаданные из хранилища.
// Делегирует в ScanDir с dataDir.
func (s *Store) ScanAll(_ context.Context) ([]*model.FileMetadata, error) {
	return ScanDir(s.dataDir)
}

// attrPath вычисляет абсолютный путь к .attr.json для данного storagePath.
func (s *Store) attrPath(storagePath string) string {
	fullDataPath := filepath.Join(s.dataDir, storagePath)
	return AttrFilePath(fullDataPath)
}
