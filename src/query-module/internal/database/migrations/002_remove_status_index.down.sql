-- 002_remove_status_index.down.sql
-- Откат: восстанавливаем составной индекс по status + uploaded_at.

DROP INDEX IF EXISTS idx_file_registry_uploaded_at;
CREATE INDEX IF NOT EXISTS idx_file_registry_status_uploaded_at ON file_registry (status, uploaded_at DESC);
