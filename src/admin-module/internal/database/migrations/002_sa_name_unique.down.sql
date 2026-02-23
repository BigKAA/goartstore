-- 002_sa_name_unique.down.sql

ALTER TABLE service_accounts DROP CONSTRAINT IF EXISTS service_accounts_name_unique;
