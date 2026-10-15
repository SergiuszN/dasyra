package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"dasyra/internal/dom"
)

// Cookbooks holds the recipe server each user is connected to.
type Cookbooks struct{ DB *DB }

func (r Cookbooks) Cookbook(ctx context.Context, userID int64) (*dom.Cookbook, error) {
	c := dom.Cookbook{UserID: userID}
	err := r.DB.QueryRowContext(ctx, "SELECT base_url, public_url, api_token FROM cookbooks WHERE user_id = ?", userID).
		Scan(&c.URL, &c.PublicURL, &c.Token)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r Cookbooks) SaveCookbook(ctx context.Context, c dom.Cookbook) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO cookbooks (user_id, base_url, public_url, api_token, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET base_url = excluded.base_url, public_url = excluded.public_url,
			api_token = excluded.api_token, updated_at = excluded.updated_at`,
		c.UserID, c.URL, c.PublicURL, c.Token, r.DB.stamp(time.Now()))
	return err
}

func (r Cookbooks) DeleteCookbook(ctx context.Context, userID int64) error {
	_, err := r.DB.ExecContext(ctx, "DELETE FROM cookbooks WHERE user_id = ?", userID)
	return err
}
