-- 001_initial_schema.down.sql
-- Откат начальной схемы Admin Module v2.0.0.

DROP TABLE IF EXISTS sync_state;
DROP TABLE IF EXISTS role_overrides;
DROP TABLE IF EXISTS file_registry;
DROP TABLE IF EXISTS service_accounts;
DROP TABLE IF EXISTS storage_elements;
DROP FUNCTION IF EXISTS update_updated_at();
DROP EXTENSION IF EXISTS "uuid-ossp";
