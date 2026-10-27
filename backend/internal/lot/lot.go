package lot

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// execer — общая часть API *pgxpool.Pool и pgx.Tx, позволяющая писать
// ставку и обновление лота в одной транзакции (см. RecordBidTx).
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Статусы лота — см. ТЗ, раздел 4.2.
const (
	StatusDraft          = "draft"
	StatusPending        = "pending"
	StatusActive         = "active"
	StatusFinishedSold   = "finished_sold"
	StatusFinishedUnsold = "finished_unsold"
	StatusRejected       = "rejected"
	StatusCancelled      = "cancelled"
)

// ListingType — аукцион (по умолчанию, вся логика Этапов 1-4) или витрина
// с фиксированной ценой (по запросу пользователя): без торгов и без Room —
// продавец публикует цену, покупатель сразу видит его контакты и
// договаривается напрямую, как в обычных объявлениях.
const (
	ListingTypeAuction    = "auction"
	ListingTypeFixedPrice = "fixed_price"
)

var (
	ErrNotFound  = errors.New("lot not found")
	ErrForbidden = errors.New("not allowed for this lot in its current status")
)

type Lot struct {
	ID              uuid.UUID  `json:"id"`
	SellerID        uuid.UUID  `json:"seller_id"`
	ListingType     string     `json:"listing_type"`
	Brand           string     `json:"brand"`
	Model           string     `json:"model"`
	Year            int        `json:"year"`
	MileageKm       int        `json:"mileage_km"`
	VIN             string     `json:"vin"`
	Condition       string     `json:"condition"`
	Description     string     `json:"description"`
	City            string     `json:"city"`
	Photos          []string   `json:"photos"`
	Videos          []string   `json:"videos"`
	Status          string     `json:"status"`
	StartPriceKGS   int64      `json:"start_price_kgs"`
	MinStepKGS      int64      `json:"min_step_kgs"`
	ReservePriceKGS *int64     `json:"reserve_price_kgs,omitempty"`
	CurrentPriceKGS *int64     `json:"current_price_kgs,omitempty"`
	StartAt         *time.Time `json:"start_at,omitempty"`
	EndAt           *time.Time `json:"end_at,omitempty"`
	ModerationNote  string     `json:"moderation_note,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Input — поля, редактируемые продавцом (без status/модерации).
type Input struct {
	ListingType     string // "" трактуется как ListingTypeAuction (обратная совместимость)
	Brand           string
	Model           string
	Year            int
	MileageKm       int
	VIN             string
	Condition       string
	Description     string
	City            string
	Photos          []string
	Videos          []string
	StartPriceKGS   int64
	MinStepKGS      int64
	ReservePriceKGS *int64
	StartAt         *time.Time
	EndAt           *time.Time
}

// Patch — частичное обновление лота: нетронутые (nil) поля сохраняют текущее
// значение в БД (см. Repo.Update). В отличие от Input, который требует полный
// набор полей при создании лота. ListingType сюда намеренно не входит —
// тип объявления не меняется после создания.
type Patch struct {
	Brand           *string
	Model           *string
	Year            *int
	MileageKm       *int
	VIN             *string
	Condition       *string
	Description     *string
	City            *string
	Photos          *[]string
	Videos          *[]string
	StartPriceKGS   *int64
	MinStepKGS      *int64
	ReservePriceKGS *int64
	StartAt         *time.Time
	EndAt           *time.Time
}

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) Create(ctx context.Context, sellerID uuid.UUID, in Input) (*Lot, error) {
	listingType := in.ListingType
	if listingType == "" {
		listingType = ListingTypeAuction
	}
	l := &Lot{
		ID:              uuid.New(),
		SellerID:        sellerID,
		ListingType:     listingType,
		Status:          StatusDraft,
		Photos:          in.Photos,
		Videos:          in.Videos,
		Brand:           in.Brand,
		Model:           in.Model,
		Year:            in.Year,
		MileageKm:       in.MileageKm,
		VIN:             in.VIN,
		Condition:       in.Condition,
		Description:     in.Description,
		City:            in.City,
		StartPriceKGS:   in.StartPriceKGS,
		MinStepKGS:      in.MinStepKGS,
		ReservePriceKGS: in.ReservePriceKGS,
		StartAt:         in.StartAt,
		EndAt:           in.EndAt,
	}
	photosJSON, err := json.Marshal(l.Photos)
	if err != nil {
		return nil, err
	}
	videosJSON, err := json.Marshal(l.Videos)
	if err != nil {
		return nil, err
	}
	err = r.pool.QueryRow(ctx, `
		INSERT INTO lots (
			id, seller_id, listing_type, brand, model, year, mileage_km, vin, condition, description, city,
			photos, videos, status, start_price_kgs, min_step_kgs, reserve_price_kgs, start_at, end_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		RETURNING created_at, updated_at
	`, l.ID, l.SellerID, l.ListingType, l.Brand, l.Model, l.Year, l.MileageKm, l.VIN, l.Condition, l.Description, l.City,
		photosJSON, videosJSON, l.Status, l.StartPriceKGS, l.MinStepKGS, l.ReservePriceKGS, l.StartAt, l.EndAt,
	).Scan(&l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return l, nil
}

func (r *Repo) GetByID(ctx context.Context, id uuid.UUID) (*Lot, error) {
	row := r.pool.QueryRow(ctx, selectColumns+` WHERE id = $1`, id)
	return scanLot(row)
}

func (r *Repo) ListBySeller(ctx context.Context, sellerID uuid.UUID) ([]Lot, error) {
	rows, err := r.pool.Query(ctx, selectColumns+` WHERE seller_id = $1 ORDER BY created_at DESC`, sellerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLots(rows)
}

// Filter — параметры поиска по каталогу активных лотов (ТЗ, раздел 4.4).
// Пустая строка/nil означает "без этого фильтра".
type Filter struct {
	ListingType string // "" означает "без фильтра по типу" (нужно Manager.Resume, которому важны все)
	Brand       string
	Model       string
	City        string
	YearMin     *int
	YearMax     *int
	PriceMinKGS *int64
	PriceMaxKGS *int64
	Limit       int
	Offset      int
}

// ListActive возвращает каталог активных лотов с фильтрами и пагинацией.
// Filter{} (без лимита) возвращает ВСЕ активные лоты без пагинации — так
// им пользуется Manager.Resume при старте сервера; разумный лимит страницы
// для HTTP-каталога выставляет сам httpapi-хендлер, а не репозиторий.
func (r *Repo) ListActive(ctx context.Context, f Filter) ([]Lot, error) {
	query := selectColumns + ` WHERE status = $1`
	args := []any{StatusActive}

	addFilter := func(clause string, value any) {
		args = append(args, value)
		query += clause + "$" + strconv.Itoa(len(args))
	}
	if f.ListingType != "" {
		addFilter(" AND listing_type = ", f.ListingType)
	}
	if f.Brand != "" {
		addFilter(" AND brand ILIKE ", "%"+escapeLike(f.Brand)+"%")
	}
	if f.Model != "" {
		addFilter(" AND model ILIKE ", "%"+escapeLike(f.Model)+"%")
	}
	if f.City != "" {
		addFilter(" AND city ILIKE ", "%"+escapeLike(f.City)+"%")
	}
	if f.YearMin != nil {
		addFilter(" AND year >= ", *f.YearMin)
	}
	if f.YearMax != nil {
		addFilter(" AND year <= ", *f.YearMax)
	}
	if f.PriceMinKGS != nil {
		addFilter(" AND current_price_kgs >= ", *f.PriceMinKGS)
	}
	if f.PriceMaxKGS != nil {
		addFilter(" AND current_price_kgs <= ", *f.PriceMaxKGS)
	}

	if f.ListingType == ListingTypeAuction {
		query += " ORDER BY end_at ASC" // аукцион: сначала те, что скорее завершатся
	} else {
		// Витрина (end_at всегда NULL) или смешанный список без фильтра по типу:
		// end_at ASC в последнем случае увёл бы всю витрину в конец (NULLS LAST
		// по умолчанию в Postgres) независимо от свежести объявления (см. code review).
		query += " ORDER BY created_at DESC"
	}
	if f.Limit > 0 {
		args = append(args, f.Limit)
		query += " LIMIT $" + strconv.Itoa(len(args))
	}
	if f.Offset > 0 {
		args = append(args, f.Offset)
		query += " OFFSET $" + strconv.Itoa(len(args))
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLots(rows)
}

// ListFavoritedBy возвращает лоты, добавленные пользователем в избранное,
// в порядке добавления (сначала новые). Видны только сами ставшие публичными
// лоты (active/finished_*) либо свои собственные — так же, как canViewLot
// решает это при самом добавлении в избранное (см. code review про
// расхождение чтения и записи). limit<=0 означает "без пагинации".
func (r *Repo) ListFavoritedBy(ctx context.Context, userID uuid.UUID, limit, offset int) ([]Lot, error) {
	query := `
		SELECT l.id, l.seller_id, l.listing_type, l.brand, l.model, l.year, l.mileage_km, l.vin, l.condition,
		       l.description, l.city, l.photos, l.videos, l.status, l.start_price_kgs, l.min_step_kgs,
		       l.reserve_price_kgs, l.current_price_kgs, l.start_at, l.end_at, l.moderation_note,
		       l.created_at, l.updated_at
		FROM favorites f
		JOIN lots l ON l.id = f.lot_id
		WHERE f.user_id = $1
		  AND (l.status IN ($2, $3, $4) OR l.seller_id = $1)
		ORDER BY f.created_at DESC
	`
	args := []any{userID, StatusActive, StatusFinishedSold, StatusFinishedUnsold}
	if limit > 0 {
		args = append(args, limit)
		query += " LIMIT $" + strconv.Itoa(len(args))
	}
	if offset > 0 {
		args = append(args, offset)
		query += " OFFSET $" + strconv.Itoa(len(args))
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLots(rows)
}

// escapeLike экранирует спецсимволы ILIKE (% _ \), чтобы они совпадали
// буквально, а не трактовались как шаблон (см. code review).
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)
	return s
}

func (r *Repo) ListPending(ctx context.Context) ([]Lot, error) {
	rows, err := r.pool.Query(ctx, selectColumns+` WHERE status = $1 ORDER BY created_at ASC`, StatusPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLots(rows)
}

// Update редактирует лот продавцом — разрешено только в статусах draft/rejected.
// Поля, оставленные в Patch как nil, не передаются в запрос вовсе и благодаря
// COALESCE сохраняют текущее значение в БД — PATCH не стирает то, что не прислали.
func (r *Repo) Update(ctx context.Context, id, sellerID uuid.UUID, p Patch) (*Lot, error) {
	var photosJSON, videosJSON []byte
	if p.Photos != nil {
		b, err := json.Marshal(*p.Photos)
		if err != nil {
			return nil, err
		}
		photosJSON = b
	}
	if p.Videos != nil {
		b, err := json.Marshal(*p.Videos)
		if err != nil {
			return nil, err
		}
		videosJSON = b
	}
	row := r.pool.QueryRow(ctx, `
		UPDATE lots SET
			brand = COALESCE($3, brand),
			model = COALESCE($4, model),
			year = COALESCE($5, year),
			mileage_km = COALESCE($6, mileage_km),
			vin = COALESCE($7, vin),
			condition = COALESCE($8, condition),
			description = COALESCE($9, description),
			city = COALESCE($10, city),
			photos = COALESCE($11, photos),
			start_price_kgs = COALESCE($12, start_price_kgs),
			min_step_kgs = COALESCE($13, min_step_kgs),
			reserve_price_kgs = COALESCE($14, reserve_price_kgs),
			start_at = COALESCE($15, start_at),
			end_at = COALESCE($16, end_at),
			videos = COALESCE($17, videos),
			updated_at = now()
		WHERE id = $1 AND seller_id = $2 AND status IN ('draft', 'rejected')
		RETURNING id, seller_id, listing_type, brand, model, year, mileage_km, vin, condition, description, city,
		          photos, videos, status, start_price_kgs, min_step_kgs, reserve_price_kgs, current_price_kgs,
		          start_at, end_at, moderation_note, created_at, updated_at
	`, id, sellerID, p.Brand, p.Model, p.Year, p.MileageKm, p.VIN, p.Condition,
		p.Description, p.City, photosJSON, p.StartPriceKGS, p.MinStepKGS,
		p.ReservePriceKGS, p.StartAt, p.EndAt, videosJSON)
	l, err := scanLot(row)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrForbidden
		}
		return nil, err
	}
	return l, nil
}

// Submit отправляет лот на модерацию: draft/rejected -> pending.
func (r *Repo) Submit(ctx context.Context, id, sellerID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE lots SET status = $3, moderation_note = '', updated_at = now()
		WHERE id = $1 AND seller_id = $2 AND status IN ('draft', 'rejected')
	`, id, sellerID, StatusPending)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrForbidden
	}
	return nil
}

func (r *Repo) Delete(ctx context.Context, id, sellerID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM lots WHERE id = $1 AND seller_id = $2 AND status = $3
	`, id, sellerID, StatusDraft)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrForbidden
	}
	return nil
}

// Approve — модератор одобряет лот: pending -> active.
// Для аукциона торги стартуют в момент одобрения (а не в изначально указанные
// продавцом start_at/end_at) — продолжительность, заданная продавцом как
// (end_at - start_at), сохраняется и отсчитывается от now(). Для витрины с
// фиксированной ценой таймера нет вовсе (end_at остаётся NULL) — лот просто
// становится видимым всем, без auction.Room.
func (r *Repo) Approve(ctx context.Context, id uuid.UUID) (*Lot, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE lots SET
			status = 'active',
			current_price_kgs = start_price_kgs,
			start_at = now(),
			end_at = CASE
				WHEN listing_type = 'auction' THEN now() + GREATEST(end_at - start_at, interval '1 minute')
				ELSE NULL
			END,
			updated_at = now()
		WHERE id = $1 AND status = 'pending'
		RETURNING id, seller_id, listing_type, brand, model, year, mileage_km, vin, condition, description, city,
		          photos, videos, status, start_price_kgs, min_step_kgs, reserve_price_kgs, current_price_kgs,
		          start_at, end_at, moderation_note, created_at, updated_at
	`, id)
	l, err := scanLot(row)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrForbidden
		}
		return nil, err
	}
	return l, nil
}

// MarkSold — продавец отмечает лот-витрину как проданный (вне аукциона
// покупка/продажа идёт напрямую между сторонами, платформа лишь фиксирует
// факт). Разрешено только владельцу и только для активной витрины.
func (r *Repo) MarkSold(ctx context.Context, id, sellerID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE lots SET status = $3, updated_at = now()
		WHERE id = $1 AND seller_id = $2 AND status = $4 AND listing_type = $5
	`, id, sellerID, StatusFinishedSold, StatusActive, ListingTypeFixedPrice)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrForbidden
	}
	return nil
}

// RecordBid синхронизирует "горячую" цену/таймер лота в БД после принятой ставки.
// Источник истины во время торгов — состояние в памяти у auction.Room; эта запись
// нужна для REST-чтений лота и для восстановления комнаты после перезапуска сервера.
func (r *Repo) RecordBid(ctx context.Context, id uuid.UUID, newPriceKGS int64, newEndAt time.Time) error {
	return recordBid(ctx, r.pool, id, newPriceKGS, newEndAt)
}

// RecordBidTx — то же самое, но в рамках уже открытой транзакции: вызывающий
// код (auction.Room.handleBid) пишет ставку в bids и обновление лота в lots
// одной транзакцией, чтобы при сбое второго шага первый не остался "висеть".
func (r *Repo) RecordBidTx(ctx context.Context, tx pgx.Tx, id uuid.UUID, newPriceKGS int64, newEndAt time.Time) error {
	return recordBid(ctx, tx, id, newPriceKGS, newEndAt)
}

func recordBid(ctx context.Context, db execer, id uuid.UUID, newPriceKGS int64, newEndAt time.Time) error {
	tag, err := db.Exec(ctx, `
		UPDATE lots SET current_price_kgs = $2, end_at = $3, updated_at = now()
		WHERE id = $1 AND status = $4
	`, id, newPriceKGS, newEndAt, StatusActive)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrForbidden
	}
	return nil
}

// Finalize переводит лот из active в один из конечных статусов по истечении торгов.
func (r *Repo) Finalize(ctx context.Context, id uuid.UUID, status string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE lots SET status = $2, updated_at = now()
		WHERE id = $1 AND status = $3
	`, id, status, StatusActive)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrForbidden
	}
	return nil
}

// Reject — модератор отклоняет лот с комментарием: pending -> rejected.
func (r *Repo) Reject(ctx context.Context, id uuid.UUID, note string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE lots SET status = $2, moderation_note = $3, updated_at = now()
		WHERE id = $1 AND status = $4
	`, id, StatusRejected, note, StatusPending)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrForbidden
	}
	return nil
}

// Stats — базовая статистика для админки (ТЗ раздел 4.6). Разбивка покрывает
// все статусы лота, чтобы TotalLots всегда равнялся их сумме — раньше
// draft/rejected/cancelled попадали в TotalLots без своего поля, и сумма
// остальных полей не сходилась с общим числом без видимой причины (см. code review).
type Stats struct {
	TotalLots       int64 `json:"total_lots"`
	DraftLots       int64 `json:"draft_lots"`
	PendingLots     int64 `json:"pending_lots"`
	ActiveLots      int64 `json:"active_lots"`
	FinishedSold    int64 `json:"finished_sold"`
	FinishedUnsold  int64 `json:"finished_unsold"`
	RejectedLots    int64 `json:"rejected_lots"`
	CancelledLots   int64 `json:"cancelled_lots"`
	// Разбиты по listing_type, а не слиты в одну сумму — аукцион (конкурентная
	// цена) и витрина (фиксированная цена) отвечают на разные вопросы, и общая
	// цифра не говорит ни о чём конкретном (см. code review).
	AuctionVolumeKGS       int64 `json:"auction_volume_kgs"`
	AuctionAvgSalePriceKGS int64 `json:"auction_avg_sale_price_kgs"`
	StoreVolumeKGS         int64 `json:"store_volume_kgs"`
	StoreAvgSalePriceKGS   int64 `json:"store_avg_sale_price_kgs"`
}

func (r *Repo) GetStats(ctx context.Context) (Stats, error) {
	var s Stats
	err := r.pool.QueryRow(ctx, `
		SELECT
			count(*),
			count(*) FILTER (WHERE status = $1),
			count(*) FILTER (WHERE status = $2),
			count(*) FILTER (WHERE status = $3),
			count(*) FILTER (WHERE status = $4),
			count(*) FILTER (WHERE status = $5),
			count(*) FILTER (WHERE status = $6),
			count(*) FILTER (WHERE status = $7),
			COALESCE(SUM(current_price_kgs) FILTER (WHERE status = $4 AND listing_type = $8), 0),
			COALESCE(AVG(current_price_kgs) FILTER (WHERE status = $4 AND listing_type = $8), 0)::bigint,
			COALESCE(SUM(current_price_kgs) FILTER (WHERE status = $4 AND listing_type = $9), 0),
			COALESCE(AVG(current_price_kgs) FILTER (WHERE status = $4 AND listing_type = $9), 0)::bigint
		FROM lots
	`, StatusDraft, StatusPending, StatusActive, StatusFinishedSold, StatusFinishedUnsold,
		StatusRejected, StatusCancelled, ListingTypeAuction, ListingTypeFixedPrice,
	).Scan(&s.TotalLots, &s.DraftLots, &s.PendingLots, &s.ActiveLots, &s.FinishedSold,
		&s.FinishedUnsold, &s.RejectedLots, &s.CancelledLots,
		&s.AuctionVolumeKGS, &s.AuctionAvgSalePriceKGS, &s.StoreVolumeKGS, &s.StoreAvgSalePriceKGS)
	return s, err
}

const selectColumns = `
	SELECT id, seller_id, listing_type, brand, model, year, mileage_km, vin, condition, description, city,
	       photos, videos, status, start_price_kgs, min_step_kgs, reserve_price_kgs, current_price_kgs,
	       start_at, end_at, moderation_note, created_at, updated_at
	FROM lots
`

func scanLot(row pgx.Row) (*Lot, error) {
	var l Lot
	var photosJSON, videosJSON []byte
	err := row.Scan(
		&l.ID, &l.SellerID, &l.ListingType, &l.Brand, &l.Model, &l.Year, &l.MileageKm, &l.VIN, &l.Condition,
		&l.Description, &l.City, &photosJSON, &videosJSON, &l.Status, &l.StartPriceKGS, &l.MinStepKGS,
		&l.ReservePriceKGS, &l.CurrentPriceKGS, &l.StartAt, &l.EndAt, &l.ModerationNote,
		&l.CreatedAt, &l.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal(photosJSON, &l.Photos); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(videosJSON, &l.Videos); err != nil {
		return nil, err
	}
	return &l, nil
}

func scanLots(rows pgx.Rows) ([]Lot, error) {
	var lots []Lot
	for rows.Next() {
		l, err := scanLot(rows)
		if err != nil {
			return nil, err
		}
		lots = append(lots, *l)
	}
	return lots, rows.Err()
}
