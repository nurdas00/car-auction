package user

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID                uuid.UUID `json:"id"`
	Email             string    `json:"email"`
	PasswordHash      string    `json:"-"`
	Name              string    `json:"name"`
	Phone             string    `json:"phone"`
	City              string    `json:"city"`
	AvatarURL         string    `json:"avatar_url"`
	Role              string    `json:"role"`
	VerificationLevel string    `json:"verification_level"`
	IsBlocked         bool      `json:"is_blocked"`
	CreatedAt         time.Time `json:"created_at"`
}

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) Create(ctx context.Context, email, passwordHash, name string) (*User, error) {
	u := &User{
		ID:                uuid.New(),
		Email:             email,
		PasswordHash:      passwordHash,
		Name:              name,
		Role:              "user",
		VerificationLevel: "email",
	}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO users (id, email, password_hash, name, role, verification_level)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at
	`, u.ID, u.Email, u.PasswordHash, u.Name, u.Role, u.VerificationLevel).Scan(&u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *Repo) GetByEmail(ctx context.Context, email string) (*User, error) {
	return r.scanOne(ctx, `
		SELECT id, email, password_hash, name, phone, city, avatar_url, role, verification_level, is_blocked, created_at
		FROM users WHERE email = $1
	`, email)
}

func (r *Repo) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	return r.scanOne(ctx, `
		SELECT id, email, password_hash, name, phone, city, avatar_url, role, verification_level, is_blocked, created_at
		FROM users WHERE id = $1
	`, id)
}

// IsBlocked — узкий запрос для requireActiveUser: раньше там гонялась вся
// строка пользователя (включая password_hash) на каждый авторизованный
// запрос ради одного булева поля (см. code review).
func (r *Repo) IsBlocked(ctx context.Context, id uuid.UUID) (bool, error) {
	var blocked bool
	err := r.pool.QueryRow(ctx, `SELECT is_blocked FROM users WHERE id = $1`, id).Scan(&blocked)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, ErrNotFound
		}
		return false, err
	}
	return blocked, nil
}

// SetBlocked — блокировка/разблокировка пользователя админом (ТЗ раздел 4.6).
// Блокировка закрывает новые входы (см. httpapi.handleLogin) и действия в уже
// открытых сессиях (см. httpapi.requireActiveUser); уже выданный JWT остаётся
// технически валидным до истечения TTL — тот же принятый для MVP компромисс
// устаревания claim'а, что и у роли в auth.RequireAdmin (см. code review).
func (r *Repo) SetBlocked(ctx context.Context, id uuid.UUID, blocked bool) error {
	tag, err := r.pool.Exec(ctx, `UPDATE users SET is_blocked = $2, updated_at = now() WHERE id = $1`, id, blocked)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateProfile — частичное обновление: nil-поле оставляет текущее значение
// в БД нетронутым (через COALESCE), так что PATCH /api/me без, например,
// phone больше не стирает его (см. code review).
func (r *Repo) UpdateProfile(ctx context.Context, id uuid.UUID, name, phone, city, avatarURL *string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE users SET
			name = COALESCE($2, name),
			phone = COALESCE($3, phone),
			city = COALESCE($4, city),
			avatar_url = COALESCE($5, avatar_url),
			updated_at = now()
		WHERE id = $1
	`, id, name, phone, city, avatarURL)
	return err
}

func (r *Repo) scanOne(ctx context.Context, query string, args ...any) (*User, error) {
	row := r.pool.QueryRow(ctx, query, args...)
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Phone, &u.City, &u.AvatarURL, &u.Role, &u.VerificationLevel, &u.IsBlocked, &u.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}
