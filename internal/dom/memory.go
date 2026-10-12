package dom

import "time"

const (
	CategorySkill   = "skill"
	CategoryJournal = "journal"
)

// Memory is one focused topic Dasyra remembers. Every update or delete is archived by the repository.
type Memory struct {
	ID        int64
	Name      string
	Category  string
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Seen holds the ids shown with their content in this run: only those may be rewritten,
// so a memory or event is never changed blind.
type Seen map[int64]bool

func (s Seen) Mark(ids ...int64) {
	for _, id := range ids {
		s[id] = true
	}
}

func (s Seen) Has(id int64) bool { return s[id] }
