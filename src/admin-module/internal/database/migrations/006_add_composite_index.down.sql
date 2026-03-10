-- Откат миграции: удаление composite index.

DROP INDEX IF EXISTS idx_file_registry_se_id_file_id;
