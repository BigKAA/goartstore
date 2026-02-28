-- 001_search_indexes.down.sql
-- Откат индексов поиска. Расширение pg_trgm не удаляется — может использоваться другими модулями.

DROP INDEX IF EXISTS idx_file_registry_retention_expires;
DROP INDEX IF EXISTS idx_file_registry_size;
DROP INDEX IF EXISTS idx_file_registry_status_uploaded_at;
DROP INDEX IF EXISTS idx_file_registry_content_type;
DROP INDEX IF EXISTS idx_file_registry_filename_trgm;
DROP INDEX IF EXISTS idx_file_registry_tags_gin;
