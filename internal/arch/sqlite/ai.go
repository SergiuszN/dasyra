package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"dasyra/internal/dom"
)

// AISettings holds the providers and per-role models of the admin panel, not scoped by user.
type AISettings struct{ DB *DB }

func (r AISettings) Providers(ctx context.Context) ([]dom.AIProvider, error) {
	rows, err := r.DB.QueryContext(ctx, "SELECT id, name, kind, base_url, api_key FROM ai_providers ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []dom.AIProvider
	for rows.Next() {
		var p dom.AIProvider
		if err := rows.Scan(&p.ID, &p.Name, &p.Kind, &p.BaseURL, &p.Key); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r AISettings) Provider(ctx context.Context, id int64) (*dom.AIProvider, error) {
	var p dom.AIProvider
	err := r.DB.QueryRowContext(ctx, "SELECT id, name, kind, base_url, api_key FROM ai_providers WHERE id = ?", id).
		Scan(&p.ID, &p.Name, &p.Kind, &p.BaseURL, &p.Key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r AISettings) SaveProvider(ctx context.Context, p *dom.AIProvider) error {
	if p.ID > 0 {
		_, err := r.DB.ExecContext(ctx, "UPDATE ai_providers SET name = ?, kind = ?, base_url = ?, api_key = ? WHERE id = ?",
			p.Name, p.Kind, p.BaseURL, p.Key, p.ID)
		return err
	}
	res, err := r.DB.ExecContext(ctx, "INSERT INTO ai_providers (name, kind, base_url, api_key, created_at) VALUES (?, ?, ?, ?, ?)",
		p.Name, p.Kind, p.BaseURL, p.Key, r.DB.stamp(time.Now()))
	if err != nil {
		return err
	}
	p.ID, err = res.LastInsertId()
	return err
}

func (r AISettings) DeleteProvider(ctx context.Context, id int64) error {
	return r.DB.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM ai_roles WHERE provider_id = ?", id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM ai_prices WHERE provider_id = ?", id); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM ai_providers WHERE id = ?", id)
		return err
	})
}

func (r AISettings) Roles(ctx context.Context) (map[string]dom.AIRole, error) {
	rows, err := r.DB.QueryContext(ctx, "SELECT role, coalesce(provider_id, 0), model, effort FROM ai_roles")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]dom.AIRole{}
	for rows.Next() {
		var role dom.AIRole
		if err := rows.Scan(&role.Role, &role.ProviderID, &role.Model, &role.Effort); err != nil {
			return nil, err
		}
		out[role.Role] = role
	}
	return out, rows.Err()
}

func (r AISettings) SaveRole(ctx context.Context, role dom.AIRole) error {
	if role.Default() {
		_, err := r.DB.ExecContext(ctx, "DELETE FROM ai_roles WHERE role = ?", role.Role)
		return err
	}
	var provider any
	if role.ProviderID > 0 {
		provider = role.ProviderID
	}
	_, err := r.DB.ExecContext(ctx, `INSERT INTO ai_roles (role, provider_id, model, effort) VALUES (?, ?, ?, ?)
		ON CONFLICT (role) DO UPDATE SET provider_id = excluded.provider_id, model = excluded.model, effort = excluded.effort`,
		role.Role, provider, strings.TrimSpace(role.Model), role.Effort)
	return err
}

const priceColumns = "provider_id, model, input, cache_read, cache_write, output, per_minute"

func scanPrice(row interface{ Scan(...any) error }) (dom.AIPrice, error) {
	var p dom.AIPrice
	err := row.Scan(&p.ProviderID, &p.Model, &p.Input, &p.CacheRead, &p.CacheWrite, &p.Output, &p.PerMinute)
	return p, err
}

func (r AISettings) Prices(ctx context.Context) ([]dom.AIPrice, error) {
	rows, err := r.DB.QueryContext(ctx, "SELECT "+priceColumns+" FROM ai_prices ORDER BY provider_id, model")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []dom.AIPrice
	for rows.Next() {
		p, err := scanPrice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r AISettings) Price(ctx context.Context, providerID int64, model string) (*dom.AIPrice, error) {
	p, err := scanPrice(r.DB.QueryRowContext(ctx, "SELECT "+priceColumns+" FROM ai_prices WHERE provider_id = ? AND model = ?", providerID, model))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r AISettings) SavePrice(ctx context.Context, p dom.AIPrice) error {
	_, err := r.DB.ExecContext(ctx, `INSERT INTO ai_prices (`+priceColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (provider_id, model) DO UPDATE SET input = excluded.input, cache_read = excluded.cache_read,
		cache_write = excluded.cache_write, output = excluded.output, per_minute = excluded.per_minute`,
		p.ProviderID, p.Model, p.Input, p.CacheRead, p.CacheWrite, p.Output, p.PerMinute)
	return err
}

func (r AISettings) DeletePrice(ctx context.Context, providerID int64, model string) error {
	_, err := r.DB.ExecContext(ctx, "DELETE FROM ai_prices WHERE provider_id = ? AND model = ?", providerID, model)
	return err
}
