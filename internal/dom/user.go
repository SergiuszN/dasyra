package dom

import (
	"strings"
	"time"
)

const (
	OwnerID         int64 = 1
	DefaultTimezone       = "UTC"
)

// User is one person Dasyra works for. Calendar times are their wall-clock time in Timezone.
type User struct {
	ID              int64
	Name            string
	ChatID          int64 // Telegram chat, 0 until bound
	Profile         string
	Timezone        string
	CreatedAt       time.Time
	LastReflectedAt time.Time // zero until the first nightly reflection
}

func (u *User) HasProfile() bool {
	return strings.TrimSpace(u.Profile) != ""
}

// DisplayName is how prompts call the user.
func (u *User) DisplayName() string {
	if u.Name == "" {
		return "the user"
	}
	return u.Name
}

func (u *User) Location() *time.Location {
	if loc, err := time.LoadLocation(u.Timezone); err == nil {
		return loc
	}
	loc, _ := time.LoadLocation(DefaultTimezone)
	return loc
}

// Local converts any instant to the user's wall clock.
func (u *User) Local(t time.Time) time.Time {
	return t.In(u.Location())
}

// ParseLocal reads a time the model wrote in the user's zone, an explicit offset (Z, +03:00) is converted to it.
func (u *User) ParseLocal(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return u.Local(t), nil
	}
	for _, layout := range localLayouts {
		if t, err := time.ParseInLocation(layout, s, u.Location()); err == nil {
			return t, nil
		}
	}
	return time.Time{}, &TimeFormatError{Value: s}
}

// ShortTime among them: the model copies times as it sees them
var localLayouts = []string{
	"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02", ShortTime,
}

type TimeFormatError struct{ Value string }

func (e *TimeFormatError) Error() string {
	return "invalid date format: " + e.Value + ", use Y-m-d H:i:s"
}

// ValidTimezone tells whether name is an IANA zone like Europe/Kyiv (or UTC), not an abbreviation like EST.
func ValidTimezone(name string) bool {
	if name != "UTC" && !strings.Contains(name, "/") {
		return false
	}
	_, err := time.LoadLocation(name)
	return err == nil
}
