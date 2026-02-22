-- 001_initial_schema.up.sql
-- Начальная схема базы данных Admin Module v2.0.0.
-- 5 таблиц: storage_elements, file_registry, service_accounts, role_overrides, sync_state.

-- Включаем расширение для генерации UUID
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Функция обновления updated_at — используется триггерами
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ==========================================
-- Таблица: storage_elements
-- Реестр зарегистрированных Storage Elements.
-- ==========================================
CREATE TABLE storage_elements (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name            TEXT NOT NULL,
    url             TEXT NOT NULL UNIQUE,
    storage_id      TEXT NOT NULL UNIQUE,
    mode            TEXT NOT NULL CHECK (mode IN ('edit', 'rw', 'ro', 'ar')),
    status          TEXT NOT NULL DEFAULT 'online'
                    CHECK (status IN ('online', 'offline', 'degraded', 'maintenance')),
    capacity_bytes  BIGINT NOT NULL DEFAULT 0,
    used_bytes      BIGINT NOT NULL DEFAULT 0,
    available_bytes BIGINT,
    last_sync_at      TIMESTAMPTZ,
    last_file_sync_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER trg_storage_elements_updated_at
    BEFORE UPDATE ON storage_elements
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE INDEX idx_storage_elements_mode ON storage_elements(mode);
CREATE INDEX idx_storage_elements_status ON storage_elements(status);

-- ==========================================
-- Таблица: file_registry
-- Реестр файлов (вторичный индекс метаданных).
-- ==========================================
CREATE TABLE file_registry (
    file_id            UUID PRIMARY KEY,
    original_filename  TEXT NOT NULL,
    content_type       TEXT NOT NULL,
    size               BIGINT NOT NULL,
    checksum           TEXT NOT NULL,
    storage_element_id UUID NOT NULL REFERENCES storage_elements(id) ON DELETE CASCADE,
    uploaded_by        TEXT NOT NULL,
    uploaded_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    description        TEXT,
    tags               TEXT[],
    status             TEXT NOT NULL DEFAULT 'active'
                       CHECK (status IN ('active', 'deleted', 'expired')),
    retention_policy   TEXT NOT NULL DEFAULT 'permanent'
                       CHECK (retention_policy IN ('permanent', 'temporary')),
    ttl_days           INTEGER,
    expires_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER trg_file_registry_updated_at
    BEFORE UPDATE ON file_registry
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE INDEX idx_file_registry_storage_element_id ON file_registry(storage_element_id);
CREATE INDEX idx_file_registry_status ON file_registry(status);
CREATE INDEX idx_file_registry_retention_policy ON file_registry(retention_policy);
CREATE INDEX idx_file_registry_uploaded_by ON file_registry(uploaded_by);
CREATE INDEX idx_file_registry_uploaded_at ON file_registry(uploaded_at);

-- ==========================================
-- Таблица: service_accounts
-- Сервисные аккаунты (локальная копия + Keycloak sync).
-- ==========================================
CREATE TABLE service_accounts (
    id                 UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    keycloak_client_id TEXT UNIQUE,
    client_id          TEXT NOT NULL UNIQUE,
    name               TEXT NOT NULL,
    description        TEXT,
    scopes             TEXT[] NOT NULL DEFAULT '{}',
    status             TEXT NOT NULL DEFAULT 'active'
                       CHECK (status IN ('active', 'suspended')),
    source             TEXT NOT NULL DEFAULT 'local'
                       CHECK (source IN ('local', 'keycloak')),
    last_synced_at     TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER trg_service_accounts_updated_at
    BEFORE UPDATE ON service_accounts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE INDEX idx_service_accounts_status ON service_accounts(status);
CREATE INDEX idx_service_accounts_client_id ON service_accounts(client_id);

-- ==========================================
-- Таблица: role_overrides
-- Локальные дополнения ролей пользователей.
-- ==========================================
CREATE TABLE role_overrides (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    keycloak_user_id TEXT NOT NULL UNIQUE,
    username         TEXT NOT NULL,
    additional_role  TEXT NOT NULL CHECK (additional_role IN ('admin', 'readonly')),
    created_by       TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER trg_role_overrides_updated_at
    BEFORE UPDATE ON role_overrides
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE INDEX idx_role_overrides_keycloak_user_id ON role_overrides(keycloak_user_id);

-- ==========================================
-- Таблица: sync_state
-- Состояние синхронизации (одна строка).
-- ==========================================
CREATE TABLE sync_state (
    id                INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    last_sa_sync_at   TIMESTAMPTZ,
    last_file_sync_at TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER trg_sync_state_updated_at
    BEFORE UPDATE ON sync_state
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Начальная запись sync_state (единственная строка)
INSERT INTO sync_state (id) VALUES (1);
