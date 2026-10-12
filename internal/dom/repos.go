package dom

import (
	"context"
	"time"
)

// Every repository call is scoped by the user: testers' chats must never mix.

type Users interface {
	Get(ctx context.Context, id int64) (*User, error)
	ByChat(ctx context.Context, chatID int64) (*User, error) // nil when unknown
	AllWithChat(ctx context.Context) ([]*User, error)
	Insert(ctx context.Context, u *User) error
	Update(ctx context.Context, u *User) error
	// BindOwnerChat gives the owner account its Telegram chat on first contact, so existing data stays with the owner
	BindOwnerChat(ctx context.Context, chatID int64) error
}

type Invites interface {
	Create(ctx context.Context, code string) error
	Available(ctx context.Context, code string) (bool, error)
	Use(ctx context.Context, code string, userID int64) error
}

type Messages interface {
	Insert(ctx context.Context, userID int64, m *Message) error
	Last(ctx context.Context, userID int64, n int) (History, error)
	Count(ctx context.Context, userID int64) (int, error)
	// After returns up to limit newest messages after the time, oldest first
	After(ctx context.Context, userID int64, after time.Time, limit int) ([]Message, error)
	Search(ctx context.Context, userID int64, terms []string, limit int, from, to *time.Time) ([]Message, error)
	// Some are the messages with these ids, in no particular order
	Some(ctx context.Context, userID int64, ids []int64) ([]Message, error)
	HasUserMessageAfter(ctx context.Context, userID, messageID int64) (bool, error)
	CheckInsSinceUserMessage(ctx context.Context, userID int64) (int, error)
	DocumentFileID(ctx context.Context, userID, telegramID int64) (string, error)
	// Seen guards against Telegram redelivering a webhook, which happens within a day
	Seen(ctx context.Context, userID, telegramID int64, since time.Time) (bool, error)
}

type Memories interface {
	Insert(ctx context.Context, userID int64, m *Memory) error
	Update(ctx context.Context, userID int64, m *Memory) error
	Delete(ctx context.Context, userID, id int64) error
	Get(ctx context.Context, userID, id int64) (*Memory, error) // nil when missing
	Search(ctx context.Context, userID int64, terms []string, category string, offset, limit int) ([]Memory, error)
	List(ctx context.Context, userID int64, category string, offset, limit int) ([]Memory, error)
}

// Kinds of records searched by meaning.
const (
	KindMemory  = "memory"
	KindMessage = "message"
	KindNote    = "note"
)

// Embeddable is the text of a record to embed.
type Embeddable struct {
	ID   int64
	Text string
}

// Vectors are the embeddings of the user's records, one per record, made by one model.
type Vectors interface {
	// Stale are records of kind without a vector of model or with one older than the record, newest first
	Stale(ctx context.Context, userID int64, kind, model string, limit int) ([]Embeddable, error)
	Save(ctx context.Context, userID int64, kind, model string, at time.Time, vectors map[int64][]float32) error
	// All are the vectors of kind made by model, by record id
	All(ctx context.Context, userID int64, kind, model string) (map[int64][]float32, error)
}

type Notes interface {
	Insert(ctx context.Context, userID int64, n *Note) error
	Update(ctx context.Context, userID int64, n *Note) error
	Delete(ctx context.Context, userID, id int64) error
	Get(ctx context.Context, userID, id int64) (*Note, error) // nil when missing
	// Find lists notes matching any of the terms (all newest first when there are none) and collection when given
	Find(ctx context.Context, userID int64, terms []string, collection string, offset, limit int) ([]Note, error)
	Collections(ctx context.Context, userID int64) ([]Collection, error)
}

type Todos interface {
	Insert(ctx context.Context, userID int64, t *Todo) error
	Update(ctx context.Context, userID int64, t *Todo) error
	Delete(ctx context.Context, userID, id int64) error
	Get(ctx context.Context, userID, id int64) (*Todo, error) // nil when missing
	// Current are the open items by day (no day last) and the ones done since doneSince
	Current(ctx context.Context, userID int64, doneSince time.Time, limit int) ([]Todo, error)
}

// EventFilter narrows list_events: Recurring nil means both kinds.
type EventFilter struct {
	Recurring     *bool
	Query         string
	From, To      *time.Time
	Offset, Limit int
}

// Events stores times as the user's wall clock, so every call takes the user for their zone.
type Events interface {
	Insert(ctx context.Context, u *User, e *Event) error
	Update(ctx context.Context, u *User, e *Event) error
	Delete(ctx context.Context, u *User, id int64) error
	Get(ctx context.Context, u *User, id int64) (*Event, error) // nil when missing
	Find(ctx context.Context, u *User, f EventFilter) ([]Event, error)
	// Upcoming are one-time events from a day ago to six days ahead
	Upcoming(ctx context.Context, u *User, now time.Time, limit int) ([]Event, error)
	// Recurring are the series that have not ended
	Recurring(ctx context.Context, u *User, now time.Time) ([]Event, error)
	// Due are events and occurrences of series that fell into (from, to] and were not reminded about yet
	Due(ctx context.Context, u *User, from, to time.Time) ([]Event, error)
	MarkReminded(ctx context.Context, u *User, id int64, at time.Time) error
}

// ToolCall is one tool run behind a stored reply, kept two days so follow-ups don't repeat searches.
type ToolCall struct {
	ReplyID   int64
	Tool      string
	Args      string // JSON
	Result    string
	CreatedAt time.Time
}

type ToolCalls interface {
	SaveRun(ctx context.Context, userID, replyID int64, calls []ToolCall) error
	// LatestRun returns the calls of the newest reply that used any tools
	LatestRun(ctx context.Context, userID int64) ([]ToolCall, error)
}

// Usage is the tokens and cost of one paid AI call.
type Usage struct {
	UserID           int64 // 0 when made on nobody's behalf
	Purpose          string
	Model            string
	PromptTokens     int
	CachedTokens     int
	CacheWriteTokens int
	CompletionTokens int
	ReasoningTokens  int
	AudioSeconds     float64
	Cost             float64
	Estimated        bool // Cost counted from a price set in the panel, the provider did not report it
}

// AISettings are the providers and the per-role models set in the admin panel, global like UsageLog.
type AISettings interface {
	Providers(ctx context.Context) ([]AIProvider, error)
	Provider(ctx context.Context, id int64) (*AIProvider, error) // nil when unknown
	SaveProvider(ctx context.Context, p *AIProvider) error       // ID 0 inserts
	// DeleteProvider also resets the roles that ran on it to their defaults and drops its prices
	DeleteProvider(ctx context.Context, id int64) error
	Roles(ctx context.Context) (map[string]AIRole, error)
	// SaveRole stores a role, a Default one is removed
	SaveRole(ctx context.Context, r AIRole) error
	Prices(ctx context.Context) ([]AIPrice, error)
	Price(ctx context.Context, providerID int64, model string) (*AIPrice, error) // nil when not set
	SavePrice(ctx context.Context, p AIPrice) error
	DeletePrice(ctx context.Context, providerID int64, model string) error
}

type UsageLog interface {
	Record(ctx context.Context, u Usage) error
}
