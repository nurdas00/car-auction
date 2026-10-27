package favorite

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// Add идемпотентен — повторное добавление уже избранного лота не ошибка.
func (r *Repo) Add(ctx context.Context, userID, lotID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO favorites (user_id, lot_id) VALUES ($1, $2)
		ON CONFLICT (user_id, lot_id) DO NOTHING
	`, userID, lotID)
	return err
}

// Remove идемпотентен — удаление отсутствующей записи не ошибка.
func (r *Repo) Remove(ctx context.Context, userID, lotID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM favorites WHERE user_id = $1 AND lot_id = $2`, userID, lotID)
	return err
}
