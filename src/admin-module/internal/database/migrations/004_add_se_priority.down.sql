-- Откат миграции 004: Удаление поля priority из storage_elements

DROP INDEX IF EXISTS idx_se_priority;
ALTER TABLE storage_elements DROP COLUMN IF EXISTS priority;
