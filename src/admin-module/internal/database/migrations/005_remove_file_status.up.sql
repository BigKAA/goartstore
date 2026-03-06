-- Миграция: удаление колонки status из таблицы file_registry
-- Философия: файл либо есть, либо нет. Никаких промежуточных состояний.

-- Удаляем индекс по статусу
DROP INDEX IF EXISTS idx_file_registry_status;

-- Удаляем колонку status (CHECK constraint удаляется автоматически вместе с колонкой)
ALTER TABLE file_registry DROP COLUMN IF EXISTS status;
