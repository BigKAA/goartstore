// disk_usage.go — получение информации об ёмкости диска.
// Платформозависимый код для Unix-подобных систем.
package main

import (
	"fmt"
	"syscall"
)

// getDiskUsage возвращает информацию о дисковом пространстве в директории.
// Возвращает total, used, available в байтах.
func getDiskUsage(path string) (total, used, available int64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, 0, fmt.Errorf("ошибка statfs %s: %w", path, err)
	}

	total = int64(stat.Blocks) * int64(stat.Bsize)
	available = int64(stat.Bavail) * int64(stat.Bsize)
	used = total - available

	return total, used, available, nil
}
