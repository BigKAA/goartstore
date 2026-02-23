-- Откат миграции 003: удаление таблицы ui_settings

DROP TRIGGER IF EXISTS ui_settings_updated_at ON ui_settings;
DROP TABLE IF EXISTS ui_settings;
