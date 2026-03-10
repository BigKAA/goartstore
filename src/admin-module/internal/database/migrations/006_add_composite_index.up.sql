-- Миграция: composite index для ускорения DeleteExcept при синхронизации.
-- Запрос DELETE WHERE storage_element_id = $1 AND file_id != ALL($2)
-- использует оба поля — composite index ускоряет scan.

CREATE INDEX IF NOT EXISTS idx_file_registry_se_id_file_id
    ON file_registry(storage_element_id, file_id);
