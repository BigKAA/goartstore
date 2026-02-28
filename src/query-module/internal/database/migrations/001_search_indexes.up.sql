-- 001_search_indexes.up.sql
-- Индексы для поиска в таблице file_registry (таблица owned by Admin Module).
-- QM использует отдельную таблицу миграций schema_migrations_qm.

-- GIN-индекс для поиска по тегам (оператор @> для "содержит все")
CREATE INDEX IF NOT EXISTS idx_file_registry_tags_gin ON file_registry USING GIN (tags);

-- B-tree индекс для поиска по имени файла (ILIKE partial match)
-- Используем pg_trgm для ускорения ILIKE-запросов
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX IF NOT EXISTS idx_file_registry_filename_trgm ON file_registry USING GIN (original_filename gin_trgm_ops);

-- B-tree индекс для фильтрации по content_type
CREATE INDEX IF NOT EXISTS idx_file_registry_content_type ON file_registry (content_type);

-- Составной индекс для типичного поиска: status + uploaded_at (сортировка по умолчанию)
CREATE INDEX IF NOT EXISTS idx_file_registry_status_uploaded_at ON file_registry (status, uploaded_at DESC);

-- Индекс для фильтрации по размеру файла
CREATE INDEX IF NOT EXISTS idx_file_registry_size ON file_registry (size);

-- Индекс для фильтрации по retention_policy + expires_at (для поиска истекающих файлов)
CREATE INDEX IF NOT EXISTS idx_file_registry_retention_expires ON file_registry (retention_policy, expires_at)
    WHERE retention_policy = 'temporary';
