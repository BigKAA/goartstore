// Пакет attr — чтение и запись файлов метаданных (attr.json).
// Каждый файл в хранилище имеет сопутствующий *.attr.json,
// который является единственным источником истины для метаданных.
// Все операции записи выполняются атомарно: temp → fsync → rename.
package attr

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/arturkryukov/artsore/storage-element/internal/domain/model"
)

// AttrSuffix — суффикс файла метаданных.
const AttrSuffix = ".attr.json"

// maxAttrFileSize — максимальный допустимый размер attr.json (4 КБ).
// Ограничение гарантирует атомарность записи.
const maxAttrFileSize = 4096

// AttrFilePath возвращает путь к attr.json для данного файла данных.
// Пример: "/data/photo.jpg" → "/data/photo.jpg.attr.json"
func AttrFilePath(dataFilePath string) string {
	return dataFilePath + AttrSuffix
}

// DataFilePathFromAttr возвращает путь к файлу данных из пути attr.json.
// Пример: "/data/photo.jpg.attr.json" → "/data/photo.jpg"
func DataFilePathFromAttr(attrPath string) string {
	return strings.TrimSuffix(attrPath, AttrSuffix)
}

// IsAttrFile проверяет, является ли путь файлом метаданных.
func IsAttrFile(path string) bool {
	return strings.HasSuffix(path, AttrSuffix)
}

// Write атомарно записывает метаданные в attr.json файл.
// Паттерн: JSON → temp файл → fsync → atomic rename.
// Возвращает ошибку, если сериализованные данные превышают 4 КБ.
func Write(path string, meta *model.FileMetadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("ошибка сериализации метаданных: %w", err)
	}

	// Проверка размера для гарантии атомарности
	if len(data) > maxAttrFileSize {
		return fmt.Errorf("размер attr.json (%d байт) превышает максимум (%d байт)", len(data), maxAttrFileSize)
	}

	// Создаём директорию если не существует
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("не удалось создать директорию %s: %w", dir, err)
	}

	// Атомарная запись: temp → fsync → rename
	tmpPath := path + ".tmp"

	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("ошибка создания временного файла: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("ошибка записи: %w", err)
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("ошибка fsync: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("ошибка закрытия файла: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("ошибка атомарного переименования: %w", err)
	}

	return nil
}

// Read читает и десериализует метаданные из attr.json файла.
// Возвращает ошибку, если файл не найден или содержит невалидный JSON.
func Read(path string) (*model.FileMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения attr.json %s: %w", path, err)
	}

	var meta model.FileMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("ошибка десериализации attr.json %s: %w", path, err)
	}

	return &meta, nil
}

// Delete удаляет attr.json файл.
// Возвращает nil если файл уже не существует.
func Delete(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("ошибка удаления attr.json %s: %w", path, err)
	}
	return nil
}

// ScanDir сканирует директорию и возвращает все файлы метаданных.
// Не рекурсивный — сканирует только указанную директорию.
// Используется при построении in-memory индекса при старте.
func ScanDir(dir string) ([]*model.FileMetadata, error) {
	pattern := filepath.Join(dir, "*"+AttrSuffix)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("ошибка сканирования директории %s: %w", dir, err)
	}

	var result []*model.FileMetadata
	for _, path := range matches {
		meta, err := Read(path)
		if err != nil {
			// Пропускаем невалидные attr.json, логируем проблему
			continue
		}
		result = append(result, meta)
	}

	return result, nil
}
