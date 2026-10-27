-- Этап 0: пользователи, лоты, ставки (схема для ставок заложена заранее,
-- сам аукционный движок/эндпоинты ставок приходят в Этапе 1).

CREATE TABLE IF NOT EXISTS users (
    id                 UUID PRIMARY KEY,
    email              TEXT NOT NULL UNIQUE,
    password_hash      TEXT NOT NULL,
    name               TEXT NOT NULL DEFAULT '',
    phone              TEXT NOT NULL DEFAULT '',
    city               TEXT NOT NULL DEFAULT '',
    avatar_url         TEXT NOT NULL DEFAULT '',
    role               TEXT NOT NULL DEFAULT 'user',             -- user | admin
    verification_level TEXT NOT NULL DEFAULT 'email',            -- email | identity
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS lots (
    id                 UUID PRIMARY KEY,
    seller_id          UUID NOT NULL REFERENCES users(id),
    brand              TEXT NOT NULL,
    model              TEXT NOT NULL,
    year               INT NOT NULL,
    mileage_km         INT NOT NULL,
    vin                TEXT NOT NULL DEFAULT '',
    condition          TEXT NOT NULL DEFAULT '',
    description        TEXT NOT NULL DEFAULT '',
    city               TEXT NOT NULL DEFAULT '',
    photos             JSONB NOT NULL DEFAULT '[]',
    status             TEXT NOT NULL DEFAULT 'draft',            -- draft|pending|active|finished_sold|finished_unsold|rejected|cancelled
    start_price_kgs    BIGINT NOT NULL,
    min_step_kgs       BIGINT NOT NULL,
    reserve_price_kgs  BIGINT,
    current_price_kgs  BIGINT,
    start_at           TIMESTAMPTZ,
    end_at             TIMESTAMPTZ,
    moderation_note    TEXT NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_lots_status ON lots(status);
CREATE INDEX IF NOT EXISTS idx_lots_seller ON lots(seller_id);

CREATE TABLE IF NOT EXISTS bids (
    id          UUID PRIMARY KEY,
    lot_id      UUID NOT NULL REFERENCES lots(id),
    bidder_id   UUID NOT NULL REFERENCES users(id),
    amount_kgs  BIGINT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_bids_lot ON bids(lot_id, created_at DESC);
