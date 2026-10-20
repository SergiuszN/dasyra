package dom

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// Invite is a single-use code a new user sends to the bot with /start.
type Invite struct {
	Code      string
	Note      string // whom it was made for
	UserID    int64  // who used it, 0 while unused
	CreatedAt time.Time
	UsedAt    time.Time
}

func (i Invite) Used() bool {
	return !i.UsedAt.IsZero()
}

func NewInviteCode() string {
	buf := make([]byte, 6)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
