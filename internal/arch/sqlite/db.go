// Package sqlite stores everything in one SQLite file, with FTS5 for searching memories and messages.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// DB is the connection with the server time zone: message and memory times are stored in it,
// calendar times in the user's own zone.
type DB struct {
	*sql.DB
	Server *time.Location
}

func Open(path string, server *time.Location) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return &DB{DB: conn, Server: server}, nil
}

const layout = "2006-01-02 15:04:05"

// stamp formats an instant as the server's wall clock, the form message times are stored in.
func (db *DB) stamp(t time.Time) string {
	return t.In(db.Server).Format(layout)
}

func wallClock(t time.Time, loc *time.Location) string {
	return t.In(loc).Format(layout)
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableTime(t *time.Time, loc *time.Location) any {
	if t == nil {
		return nil
	}
	return wallClock(*t, loc)
}

// wall reads a DATETIME column as a zone-less wall clock, whether the driver hands over text or a time.
type wall struct {
	t     time.Time
	valid bool
}

func (w *wall) Scan(v any) error {
	switch v := v.(type) {
	case nil:
		w.valid = false
	case time.Time:
		w.t, w.valid = v, true
	case string:
		return w.parse(v)
	case []byte:
		return w.parse(string(v))
	default:
		return fmt.Errorf("unexpected time value %T", v)
	}
	return nil
}

func (w *wall) parse(s string) error {
	for _, l := range []string{layout, "2006-01-02T15:04:05Z07:00", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.Parse(l, s); err == nil {
			w.t, w.valid = t, true
			return nil
		}
	}
	return fmt.Errorf("unexpected time %q", s)
}

// in reads the wall clock in the zone it was written in.
func (w wall) in(loc *time.Location) time.Time {
	t := w.t
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, loc)
}

func (w wall) ptr(loc *time.Location) *time.Time {
	if !w.valid {
		return nil
	}
	t := w.in(loc)
	return &t
}

// IDs hands out time ordered row ids (microseconds), never the same one twice.
type IDs struct {
	mu   sync.Mutex
	last int64
}

func (g *IDs) Next() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.last = max(time.Now().UnixMicro(), g.last+1)
	return g.last
}

// tx runs f in a transaction.
func (db *DB) tx(ctx context.Context, f func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := f(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
