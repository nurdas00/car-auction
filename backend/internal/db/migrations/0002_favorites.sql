-- Этап 2: избранное (подписка пользователя на лот, ТЗ раздел 4.4).

-- ON DELETE CASCADE: без него удаление лота (или пользователя), на который
-- есть запись в favorites, упиралось бы в FK-violation (см. code review).
CREATE TABLE IF NOT EXISTS favorites (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lot_id     UUID NOT NULL REFERENCES lots(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, lot_id)
);

CREATE INDEX IF NOT EXISTS idx_favorites_lot ON favorites(lot_id);
