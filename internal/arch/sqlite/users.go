package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"dasyra/internal/dom"
)

type Users struct{ DB *DB }

const userColumns = "id, COALESCE(name, ''), COALESCE(telegram_chat_id, 0), COALESCE(profile, ''), timezone, created_at, last_reflected_at"

func (r Users) scan(row interface{ Scan(...any) error }) (*dom.User, error) {
	var u dom.User
	var created, reflected wall
	if err := row.Scan(&u.ID, &u.Name, &u.ChatID, &u.Profile, &u.Timezone, &created, &reflected); err != nil {
		return nil, err
	}
	u.CreatedAt = created.in(r.DB.Server)
	if reflected.valid {
		u.LastReflectedAt = reflected.in(r.DB.Server)
	}
	return &u, nil
}

func (r Users) one(ctx context.Context, where string, arg any) (*dom.User, error) {
	u, err := r.scan(r.DB.QueryRowContext(ctx, "SELECT "+userColumns+" FROM users WHERE "+where, arg))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return u, err
}

func (r Users) Get(ctx context.Context, id int64) (*dom.User, error) {
	return r.one(ctx, "id = ?", id)
}

func (r Users) ByChat(ctx context.Context, chatID int64) (*dom.User, error) {
	return r.one(ctx, "telegram_chat_id = ?", chatID)
}

func (r Users) AllWithChat(ctx context.Context) ([]*dom.User, error) {
	rows, err := r.DB.QueryContext(ctx, "SELECT "+userColumns+" FROM users WHERE telegram_chat_id IS NOT NULL ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*dom.User
	for rows.Next() {
		u, err := r.scan(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r Users) Insert(ctx context.Context, u *dom.User) error {
	_, err := r.DB.ExecContext(ctx,
		"INSERT INTO users (id, name, telegram_chat_id, profile, created_at, last_reflected_at, timezone) VALUES (?, ?, ?, ?, ?, ?, ?)",
		u.ID, nullable(u.Name), chat(u.ChatID), nullable(u.Profile), r.DB.stamp(u.CreatedAt), r.reflected(u), u.Timezone)
	return err
}

func (r Users) Update(ctx context.Context, u *dom.User) error {
	_, err := r.DB.ExecContext(ctx,
		"UPDATE users SET name = ?, telegram_chat_id = ?, profile = ?, last_reflected_at = ?, timezone = ? WHERE id = ?",
		nullable(u.Name), chat(u.ChatID), nullable(u.Profile), r.reflected(u), u.Timezone, u.ID)
	return err
}

func (r Users) BindOwnerChat(ctx context.Context, chatID int64) error {
	if chatID == 0 {
		return nil
	}
	_, err := r.DB.ExecContext(ctx, "UPDATE users SET telegram_chat_id = ? WHERE id = ? AND telegram_chat_id IS NULL", chatID, dom.OwnerID)
	return err
}

func (r Users) reflected(u *dom.User) any {
	if u.LastReflectedAt.IsZero() {
		return nil
	}
	return r.DB.stamp(u.LastReflectedAt)
}

func chat(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

type Invites struct{ DB *DB }

func (r Invites) Create(ctx context.Context, code string) error {
	_, err := r.DB.ExecContext(ctx, "INSERT INTO invites (code, created_at) VALUES (?, ?)", code, r.DB.stamp(time.Now()))
	return err
}

func (r Invites) Available(ctx context.Context, code string) (bool, error) {
	var n int
	err := r.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM invites WHERE code = ? AND used_at IS NULL", code).Scan(&n)
	return n > 0, err
}

func (r Invites) Use(ctx context.Context, code string, userID int64) error {
	_, err := r.DB.ExecContext(ctx, "UPDATE invites SET user_id = ?, used_at = ? WHERE code = ? AND used_at IS NULL", userID, r.DB.stamp(time.Now()), code)
	return err
}
