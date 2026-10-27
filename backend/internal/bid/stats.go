package bid

import "context"

// Count — общее число ставок за всё время (ТЗ раздел 4.6, базовая аналитика).
func (r *Repo) Count(ctx context.Context) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM bids`).Scan(&n)
	return n, err
}
