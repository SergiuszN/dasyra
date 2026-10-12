package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"dasyra/internal/dom"
)

// Settings are the global settings of the admin panel, stored as JSON by name.
type Settings struct{ DB *DB }

const checkInPlanName = "check_in_plan"

func (r Settings) CheckInPlan(ctx context.Context) (dom.CheckInPlan, error) {
	plan := dom.DefaultCheckInPlan()
	return plan, r.load(ctx, checkInPlanName, &plan)
}

func (r Settings) SaveCheckInPlan(ctx context.Context, p dom.CheckInPlan) error {
	return r.save(ctx, checkInPlanName, p)
}

// load leaves v as it is when nothing is saved under the name.
func (r Settings) load(ctx context.Context, name string, v any) error {
	var value string
	err := r.DB.QueryRowContext(ctx, "SELECT value FROM settings WHERE name = ?", name).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(value), v)
}

func (r Settings) save(ctx context.Context, name string, v any) error {
	value, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = r.DB.ExecContext(ctx, "INSERT INTO settings (name, value) VALUES (?, ?) ON CONFLICT (name) DO UPDATE SET value = excluded.value", name, string(value))
	return err
}
