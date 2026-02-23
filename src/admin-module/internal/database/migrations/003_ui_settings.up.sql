-- Миграция 003: таблица ui_settings
-- Хранение настроек Admin UI (Prometheus URL, конфигурация графиков и т.д.)

CREATE TABLE IF NOT EXISTS ui_settings (
    -- Уникальный ключ настройки (например, "prometheus.url", "prometheus.enabled")
    key TEXT PRIMARY KEY,

    -- Значение настройки (строковое представление, типизация на уровне сервиса)
    value TEXT NOT NULL DEFAULT '',

    -- Дата последнего обновления
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Кто обновил настройку (username из JWT)
    updated_by TEXT NOT NULL DEFAULT ''
);

-- Триггер автоматического обновления updated_at
-- Функция update_updated_at() уже создана в миграции 001
CREATE TRIGGER ui_settings_updated_at
    BEFORE UPDATE ON ui_settings
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at();

-- Комментарии к таблице и столбцам
COMMENT ON TABLE ui_settings IS 'Настройки Admin UI, хранимые в БД (Prometheus конфигурация и др.)';
COMMENT ON COLUMN ui_settings.key IS 'Уникальный ключ настройки (dot-notation, например prometheus.url)';
COMMENT ON COLUMN ui_settings.value IS 'Значение настройки (строковое представление)';
COMMENT ON COLUMN ui_settings.updated_at IS 'Время последнего обновления';
COMMENT ON COLUMN ui_settings.updated_by IS 'Пользователь, обновивший настройку';
