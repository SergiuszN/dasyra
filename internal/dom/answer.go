package dom

import (
	"regexp"
	"slices"
	"strings"
)

// Reactions are the emoji Dasyra may tap instead of a text reply: only Telegram's standard set works for bots.
var Reactions = []string{"👍", "❤", "🔥", "🙏", "😁"}

const MaxParts = 5

// Answer is the final text of a run, read for its special forms.
type Answer string

// Silent is NO_MESSAGE: a check-in found nothing worth sending.
func (a Answer) Silent() bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(string(a))), "no_message")
}

var reactionPattern = regexp.MustCompile(`^REACTION:\s*(\S+)\s*$`)

// Reaction is the emoji of a "REACTION: 👍" answer, an unknown one becomes 👍.
func (a Answer) Reaction() (string, bool) {
	m := reactionPattern.FindStringSubmatch(strings.TrimSpace(string(a)))
	if m == nil {
		return "", false
	}
	// Models write ❤️ with a variation selector, Telegram accepts only the bare ❤
	emoji := strings.ReplaceAll(m[1], "️", "")
	if slices.Contains(Reactions, emoji) {
		return emoji, true
	}
	return Reactions[0], true
}

var partSeparator = regexp.MustCompile(`\n\s*\n`)

// Parts splits a longer answer on empty lines into at most MaxParts messages, like a person texting.
func (a Answer) Parts() []string {
	var parts []string
	for _, p := range partSeparator.Split(strings.TrimSpace(string(a)), -1) {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) > MaxParts {
		parts = append(parts[:MaxParts-1], strings.Join(parts[MaxParts-1:], "\n"))
	}
	return parts
}
