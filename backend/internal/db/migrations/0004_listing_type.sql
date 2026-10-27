-- Витрина с фиксированной ценой параллельно аукциону (по запросу пользователя):
-- listing_type='auction' — существующее поведение без изменений;
-- listing_type='fixed_price' — лот без торгов, сразу с контактами продавца,
-- не попадает в auction.Manager (ни Room, ни Resume).
ALTER TABLE lots ADD COLUMN IF NOT EXISTS listing_type TEXT NOT NULL DEFAULT 'auction';
ALTER TABLE lots ADD COLUMN IF NOT EXISTS videos JSONB NOT NULL DEFAULT '[]';

CREATE INDEX IF NOT EXISTS idx_lots_listing_type ON lots(listing_type);
