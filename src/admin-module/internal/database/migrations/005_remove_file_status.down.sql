-- Откат: восстановление колонки status в таблице file_registry

-- Добавляем колонку status обратно
ALTER TABLE file_registry ADD COLUMN status TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'deleted', 'expired'));

-- Восстанавливаем индекс
CREATE INDEX idx_file_registry_status ON file_registry(status);
