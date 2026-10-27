package lot

import (
	"context"
	"encoding/json"
	"errors"
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

var (
	ErrNotFound   = errors.New("lot not found")
	ErrForbidden  = errors.New("not allowed for this lot in its current status")
)

type Lot struct {
	ID               uuid.UUID  `json:"id"`
	SellerID         uuid.UUID  `json:"seller_id"`
	Brand            string     `json:"brand"`
	Model            string     `json:"model"`
	Year             int        `json:"year"`
	MileageKm        int        `json:"mileage_km"`
	VIN              string     `json:"vin"`
	Condition        string     `json:"condition"`
	Description      string     `json:"description"`
	City             string     `json:"city"`
	Photos           []string   `json:"photos"`
	Status           string     `json:"status"`
	StartPriceKGS    int64      `json:"start_price_kgs"`
	MinStepKGS       int64      `json:"min_step_kgs"`
	ReservePriceKGS  *int64     `json:"reserve_price_kgs,omitempty"`
	CurrentPriceKGS  *int64     `json:"current_price_kgs,omitempty"`
	StartAt          *time.Time `json:"start_at,omitempty"`
	EndAt            *time.Time `json:"end_at,omitempty"`
	ModerationNote   string     `json:"moderation_note,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// Input — поля, редактируемые продавцом (без status/модерации).
type Input struct {
	Brand           string
	Model           string
	Year            int
	MileageKm       int
	VIN             string
	Condition       string
	Description     string
	City            string
	Photos          []string
	StartPriceKGS   int64
	MinStepKGS      int64
	ReservePriceKGS *int64
	StartAt         *time.Time
	EndAt           *time.Time
}

// Patch — частичное обновление лота: нетронутые (nil) поля сохраняют текущее
// значение в БД (см. Repo.Update). В отличие от Input, который требует полный
// набор полей при создании лота.
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
	l := &Lot{
		ID:              uuid.New(),
		SellerID:        sellerID,
		Status:          StatusDraft,
		Photos:          in.Photos,
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
	err = r.pool.QueryRow(ctx, `
		INSERT INTO lots (
			id, seller_id, brand, model, year, mileage_km, vin, condition, description, city,
			photos, status, start_price_kgs, min_step_kgs, reserve_price_kgs, start_at, end_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING created_at, updated_at
	`, l.ID, l.SellerID, l.Brand, l.Model, l.Year, l.MileageKm, l.VIN, l.Condition, l.Description, l.City,
		photosJSON, l.Status, l.StartPriceKGS, l.MinStepKGS, l.ReservePriceKGS, l.StartAt, l.EndAt,
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

func (r *Repo) ListActive(ctx context.Context) ([]Lot, error) {
	rows, err := r.pool.Query(ctx, selectColumns+` WHERE status = $1 ORDER BY end_at ASC`, StatusActive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLots(rows)
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
	var photosJSON []byte
	if p.Photos != nil {
		b, err := json.Marshal(*p.Photos)
		if err != nil {
			return nil, err
		}
		photosJSON = b
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
			updated_at = now()
		WHERE id = $1 AND seller_id = $2 AND status IN ('draft', 'rejected')
		RETURNING id, seller_id, brand, model, year, mileage_km, vin, condition, description, city,
		          photos, status, start_price_kgs, min_step_kgs, reserve_price_kgs, current_price_kgs,
		          start_at, end_at, moderation_note, created_at, updated_at
	`, id, sellerID, p.Brand, p.Model, p.Year, p.MileageKm, p.VIN, p.Condition,
		p.Description, p.City, photosJSON, p.StartPriceKGS, p.MinStepKGS,
		p.ReservePriceKGS, p.StartAt, p.EndAt)
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

// Approve — модератор одобряет лот: pending -> active. Торги стартуют в момент одобрения
// (а не в изначально указанные продавцом start_at/end_at) — продолжительность торгов,
// заданная продавцом как (end_at - start_at), сохраняется и отсчитывается от now().
func (r *Repo) Approve(ctx context.Context, id uuid.UUID) (*Lot, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE lots SET
			status = 'active',
			current_price_kgs = start_price_kgs,
			start_at = now(),
			end_at = now() + GREATEST(end_at - start_at, interval '1 minute'),
			updated_at = now()
		WHERE id = $1 AND status = 'pending'
		RETURNING id, seller_id, brand, model, year, mileage_km, vin, condition, description, city,
		          photos, status, start_price_kgs, min_step_kgs, reserve_price_kgs, current_price_kgs,
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

const selectColumns = `
	SELECT id, seller_id, brand, model, year, mileage_km, vin, condition, description, city,
	       photos, status, start_price_kgs, min_step_kgs, reserve_price_kgs, current_price_kgs,
	       start_at, end_at, moderation_note, created_at, updated_at
	FROM lots
`

func scanLot(row pgx.Row) (*Lot, error) {
	var l Lot
	var photosJSON []byte
	err := row.Scan(
		&l.ID, &l.SellerID, &l.Brand, &l.Model, &l.Year, &l.MileageKm, &l.VIN, &l.Condition,
		&l.Description, &l.City, &photosJSON, &l.Status, &l.StartPriceKGS, &l.MinStepKGS,
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
