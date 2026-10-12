package sqlite

import (
	"context"
	"strings"
)

const (
	// A term found in more of the user's records than this share tells them nothing apart: the stop words of any
	// language drop out by themselves ("что", "the", "się"), and so does a name in every record
	commonShare = 0.2
	// Below this many records shares mean little, every term is kept
	commonFloor = 20
)

// ftsTable is a full-text index over a table of user records.
type ftsTable struct{ index, records string }

var (
	memoriesFTS      = ftsTable{"memories_fts", "memories"}
	conversationsFTS = ftsTable{"conversations_fts", "conversations"}
	notesFTS         = ftsTable{"notes_fts", "notes"}
)

// match builds an FTS5 query matching any of the terms as a substring, without the terms too common among the
// user's records; "" when none is left.
func (db *DB) match(ctx context.Context, t ftsTable, userID int64, terms []string) (string, error) {
	if len(terms) == 0 {
		return "", nil
	}
	var total int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+t.records+" WHERE user_id = ?", userID).Scan(&total); err != nil {
		return "", err
	}
	var kept []string
	for _, term := range terms {
		phrase := `"` + strings.ReplaceAll(term, `"`, `""`) + `"`
		if total >= commonFloor {
			var n int
			err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+t.index+" JOIN "+t.records+" r ON "+t.index+".rowid = r.id WHERE "+
				t.index+" MATCH ? AND r.user_id = ?", phrase, userID).Scan(&n)
			if err != nil {
				return "", err
			}
			if float64(n) > commonShare*float64(total) {
				continue
			}
		}
		kept = append(kept, phrase)
	}
	return strings.Join(kept, " OR "), nil
}
