-- Этап 3: блокировка пользователей админом (ТЗ раздел 4.6).
-- Снятие лота с публикации использует уже существующий статус lots.status =
-- 'cancelled' (заложен в схеме Этапа 0, миграция не требуется).

ALTER TABLE users ADD COLUMN IF NOT EXISTS is_blocked BOOLEAN NOT NULL DEFAULT false;
