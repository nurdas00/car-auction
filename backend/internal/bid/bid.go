package bid

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Bid struct {
	ID        uuid.UUID `json:"id"`
	LotID     uuid.UUID `json:"lot_id"`
	BidderID  uuid.UUID `json:"bidder_id"`
	AmountKGS int64     `json:"amount_kgs"`
	CreatedAt time.Time `json:"created_at"`
}

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// execer — общая часть API *pgxpool.Pool и pgx.Tx (см. CreateTx).
type execer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Create — неизменная (append-only) запись ставки для аудита и разрешения споров (ТЗ, раздел 5).
func (r *Repo) Create(ctx context.Context, lotID, bidderID uuid.UUID, amountKGS int64) (*Bid, error) {
	return create(ctx, r.pool, lotID, bidderID, amountKGS)
}

// CreateTx — то же самое в рамках уже открытой транзакции: вызывающий код
// (auction.Room.handleBid) пишет ставку и обновление лота одной транзакцией,
// чтобы при сбое второго шага запись о ставке не осталась "висеть" одна.
func (r *Repo) CreateTx(ctx context.Context, tx pgx.Tx, lotID, bidderID uuid.UUID, amountKGS int64) (*Bid, error) {
	return create(ctx, tx, lotID, bidderID, amountKGS)
}

func create(ctx context.Context, db execer, lotID, bidderID uuid.UUID, amountKGS int64) (*Bid, error) {
	b := &Bid{ID: uuid.New(), LotID: lotID, BidderID: bidderID, AmountKGS: amountKGS}
	err := db.QueryRow(ctx, `
		INSERT INTO bids (id, lot_id, bidder_id, amount_kgs)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at
	`, b.ID, b.LotID, b.BidderID, b.AmountKGS).Scan(&b.CreatedAt)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (r *Repo) ListByLot(ctx context.Context, lotID uuid.UUID) ([]Bid, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, lot_id, bidder_id, amount_kgs, created_at
		FROM bids WHERE lot_id = $1 ORDER BY created_at DESC
	`, lotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bids []Bid
	for rows.Next() {
		var b Bid
		if err := rows.Scan(&b.ID, &b.LotID, &b.BidderID, &b.AmountKGS, &b.CreatedAt); err != nil {
			return nil, err
		}
		bids = append(bids, b)
	}
	return bids, rows.Err()
}

// HighestBidder возвращает id победителя по наивысшей ставке лота, если ставки были.
func (r *Repo) HighestBidder(ctx context.Context, lotID uuid.UUID) (uuid.UUID, bool, error) {
	var bidderID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT bidder_id FROM bids WHERE lot_id = $1 ORDER BY amount_kgs DESC, created_at ASC LIMIT 1
	`, lotID).Scan(&bidderID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return uuid.Nil, false, nil
		}
		return uuid.Nil, false, err
	}
	return bidderID, true, nil
}
