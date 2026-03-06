-- 002_remove_status_index.up.sql
-- Удаление составного индекса по status + uploaded_at.
-- Столбец status удалён миграцией Admin Module (005_remove_file_status).
-- QM заменяет составной индекс на простой по uploaded_at (сортировка по умолчанию).

-- Удаляем старый составной индекс (status, uploaded_at)
DROP INDEX IF EXISTS idx_file_registry_status_uploaded_at;

-- Создаём новый индекс только по uploaded_at (основной сортировочный столбец)
CREATE INDEX IF NOT EXISTS idx_file_registry_uploaded_at ON file_registry (uploaded_at DESC);
