-- 002_sa_name_unique.up.sql
-- Добавляет UNIQUE constraint на имя сервисного аккаунта.
-- По контракту API дубликат имени → HTTP 409 CONFLICT.

ALTER TABLE service_accounts ADD CONSTRAINT service_accounts_name_unique UNIQUE (name);
