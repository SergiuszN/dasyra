package sqlite

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"dasyra/internal/dom"
)

type Vectors struct{ DB *DB }

// embeddable is what gets embedded for each kind, with the time the record last changed.
var embeddable = map[string]string{
	dom.KindMemory:  "SELECT id, name || ': ' || content AS text, updated_at AS changed, user_id FROM memories",
	dom.KindMessage: "SELECT id, content AS text, created_at AS changed, user_id FROM conversations",
	dom.KindNote:    "SELECT id, title || ' (' || tags || '): ' || content AS text, updated_at AS changed, user_id FROM notes",
}

func (r Vectors) Stale(ctx context.Context, userID int64, kind, model string, limit int) ([]dom.Embeddable, error) {
	source, ok := embeddable[kind]
	if !ok {
		return nil, fmt.Errorf("unknown kind %q", kind)
	}
	rows, err := r.DB.QueryContext(ctx, `SELECT s.id, s.text FROM (`+source+`) s
		LEFT JOIN vectors v ON v.kind = ? AND v.record_id = s.id
		WHERE s.user_id = ? AND (v.record_id IS NULL OR v.model != ? OR v.embedded_at < s.changed)
		ORDER BY s.changed DESC LIMIT ?`, kind, userID, model, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stale []dom.Embeddable
	for rows.Next() {
		var e dom.Embeddable
		if err := rows.Scan(&e.ID, &e.Text); err != nil {
			return nil, err
		}
		stale = append(stale, e)
	}
	return stale, rows.Err()
}

func (r Vectors) Save(ctx context.Context, userID int64, kind, model string, at time.Time, vectors map[int64][]float32) error {
	return r.DB.tx(ctx, func(tx *sql.Tx) error {
		for id, v := range vectors {
			_, err := tx.ExecContext(ctx, `INSERT INTO vectors (kind, record_id, user_id, model, vector, embedded_at) VALUES (?, ?, ?, ?, ?, ?)
				ON CONFLICT (kind, record_id) DO UPDATE SET model = excluded.model, vector = excluded.vector, embedded_at = excluded.embedded_at`,
				kind, id, userID, model, encodeVector(v), r.DB.stamp(at))
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (r Vectors) All(ctx context.Context, userID int64, kind, model string) (map[int64][]float32, error) {
	rows, err := r.DB.QueryContext(ctx, "SELECT record_id, vector FROM vectors WHERE user_id = ? AND kind = ? AND model = ?", userID, kind, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	vectors := map[int64][]float32{}
	for rows.Next() {
		var id int64
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			return nil, err
		}
		vectors[id] = decodeVector(blob)
	}
	return vectors, rows.Err()
}

func encodeVector(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(f))
	}
	return b
}

func decodeVector(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return v
}
