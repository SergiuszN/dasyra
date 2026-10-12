package sqlite

import (
	"context"
	"strconv"
	"strings"
	"time"

	"dasyra/internal/dom"
)

// Events stores times, reminded_at included, as the user's wall clock: no conversion is needed for the model.
type Events struct{ DB *DB }

const eventColumns = "id, event, start_time, end_time, COALESCE(location, ''), COALESCE(repeat_interval, ''), recurrence_end_date, reminded_at"

func (r Events) list(ctx context.Context, u *dom.User, where string, args ...any) ([]dom.Event, error) {
	rows, err := r.DB.QueryContext(ctx, "SELECT "+eventColumns+" FROM calendar_events WHERE user_id = ? AND "+where, append([]any{u.ID}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	loc := u.Location()
	var events []dom.Event
	for rows.Next() {
		var e dom.Event
		var start, end, until, reminded wall
		if err := rows.Scan(&e.ID, &e.Title, &start, &end, &e.Location, &e.RepeatDays, &until, &reminded); err != nil {
			return nil, err
		}
		e.Start, e.End, e.RecurrenceEnd, e.RemindedAt = start.in(loc), end.ptr(loc), until.ptr(loc), reminded.ptr(loc)
		events = append(events, e)
	}
	return events, rows.Err()
}

func (r Events) Insert(ctx context.Context, u *dom.User, e *dom.Event) error {
	loc := u.Location()
	_, err := r.DB.ExecContext(ctx, `INSERT INTO calendar_events (id, user_id, event, start_time, end_time, location, repeat_interval, recurrence_end_date)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, u.ID, e.Title, wallClock(e.Start, loc), nullableTime(e.End, loc), nullable(e.Location), nullable(e.RepeatDays), nullableTime(e.RecurrenceEnd, loc))
	return err
}

// Update keeps reminded_at only while the start stays, a moved event is reminded about again.
func (r Events) Update(ctx context.Context, u *dom.User, e *dom.Event) error {
	loc := u.Location()
	start := wallClock(e.Start, loc)
	_, err := r.DB.ExecContext(ctx, `UPDATE calendar_events SET event = ?, start_time = ?, end_time = ?, location = ?, repeat_interval = ?, recurrence_end_date = ?,
		reminded_at = CASE WHEN start_time = ? THEN reminded_at ELSE NULL END WHERE id = ? AND user_id = ?`,
		e.Title, start, nullableTime(e.End, loc), nullable(e.Location), nullable(e.RepeatDays), nullableTime(e.RecurrenceEnd, loc), start, e.ID, u.ID)
	return err
}

func (r Events) Delete(ctx context.Context, u *dom.User, id int64) error {
	_, err := r.DB.ExecContext(ctx, "DELETE FROM calendar_events WHERE id = ? AND user_id = ?", id, u.ID)
	return err
}

func (r Events) Get(ctx context.Context, u *dom.User, id int64) (*dom.Event, error) {
	found, err := r.list(ctx, u, "id = ?", id)
	if err != nil || len(found) == 0 {
		return nil, err
	}
	return &found[0], nil
}

func (r Events) Upcoming(ctx context.Context, u *dom.User, now time.Time, limit int) ([]dom.Event, error) {
	from := now.Add(-24 * time.Hour)
	return r.list(ctx, u, "repeat_interval IS NULL AND start_time >= ? AND start_time <= ? ORDER BY start_time ASC LIMIT ?",
		wallClock(from, u.Location()), wallClock(from.AddDate(0, 0, 7), u.Location()), limit)
}

// Recurring leaves ended series out of the prompt: Luna ignored "until" and announced a March event in October.
func (r Events) Recurring(ctx context.Context, u *dom.User, now time.Time) ([]dom.Event, error) {
	return r.list(ctx, u, "repeat_interval IS NOT NULL AND (recurrence_end_date IS NULL OR recurrence_end_date >= ?)", wallClock(now, u.Location()))
}

func (r Events) Due(ctx context.Context, u *dom.User, from, to time.Time) ([]dom.Event, error) {
	f, t := wallClock(from, u.Location()), wallClock(to, u.Location())
	candidates, err := r.list(ctx, u, `(
			(repeat_interval IS NULL AND reminded_at IS NULL AND start_time > ? AND start_time <= ?)
			OR (repeat_interval IS NOT NULL AND start_time <= ? AND (recurrence_end_date IS NULL OR recurrence_end_date > ?))
		) ORDER BY repeat_interval IS NOT NULL, start_time ASC`, f, t, t, f)
	if err != nil {
		return nil, err
	}
	var due []dom.Event
	for _, e := range candidates {
		if e.Due(from, to) {
			due = append(due, e)
		}
	}
	return due, nil
}

func (r Events) MarkReminded(ctx context.Context, u *dom.User, id int64, at time.Time) error {
	_, err := r.DB.ExecContext(ctx, "UPDATE calendar_events SET reminded_at = ? WHERE id = ? AND user_id = ?", wallClock(at, u.Location()), id, u.ID)
	return err
}

func (r Events) Find(ctx context.Context, u *dom.User, f dom.EventFilter) ([]dom.Event, error) {
	query := strings.TrimSpace(f.Query)
	if id, err := strconv.ParseInt(query, 10, 64); err == nil {
		if e, err := r.Get(ctx, u, id); err != nil || e != nil {
			return eventList(e), err
		}
	}

	where, args := []string{"1 = 1"}, []any{}
	switch {
	case f.Recurring != nil && *f.Recurring:
		where = append(where, "repeat_interval IS NOT NULL")
	case f.Recurring != nil:
		where = append(where, "repeat_interval IS NULL")
		fallthrough
	default:
		if f.From != nil {
			where, args = append(where, "start_time >= ?"), append(args, wallClock(*f.From, u.Location()))
		}
		if f.To != nil {
			where, args = append(where, "start_time <= ?"), append(args, wallClock(*f.To, u.Location()))
		}
	}
	if query != "" {
		where, args = append(where, "(event LIKE ? OR location LIKE ?)"), append(args, "%"+query+"%", "%"+query+"%")
	}
	return r.list(ctx, u, strings.Join(where, " AND ")+" ORDER BY start_time ASC LIMIT ? OFFSET ?", append(args, f.Limit, f.Offset)...)
}

func eventList(e *dom.Event) []dom.Event {
	if e == nil {
		return nil
	}
	return []dom.Event{*e}
}
