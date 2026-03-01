-- Миграция 004: Добавление поля priority в storage_elements
-- Priority определяет порядок заполнения SE в Sequential Fill Algorithm
-- Меньшее значение = более высокий приоритет (0 — наивысший)
-- SE с одинаковым priority сортируются по name ASC

ALTER TABLE storage_elements ADD COLUMN priority INTEGER NOT NULL DEFAULT 0;

-- Составной индекс для оптимизации выборки SE по приоритету:
-- используется в Ingester Module (Sequential Fill) и в LIST endpoint AM
CREATE INDEX idx_se_priority ON storage_elements (priority, mode, status);
