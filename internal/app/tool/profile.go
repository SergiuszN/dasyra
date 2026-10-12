package tool

import (
	"context"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

type UpdateProfile struct {
	Users dom.Users
}

func (UpdateProfile) Spec() llm.Tool {
	return Spec("update_profile", "Replace the user core profile (always in your context): who they are, north stars for this year, priority order, key stable facts, and language notes (the language they want, languages they mix in, words that mean something specific for them). Pass the complete updated text, not a diff. Also sets their time zone.",
		Str("name", "How to call the user"),
		Str("profile", "Complete profile text, a few short lines").Required(),
		Str("timezone", "IANA time zone where the user lives, e.g. Europe/Kyiv; set it once you know their city or they name a zone"),
	)
}

func (t UpdateProfile) Run(ctx context.Context, s *Session, a Args) (string, error) {
	profile, name, timezone := a.String("profile"), a.String("name"), a.String("timezone")

	// The time zone alone can be changed without retyping the profile
	if profile == "" && timezone == "" {
		return "Error: profile is required.", nil
	}
	if timezone != "" && !dom.ValidTimezone(timezone) {
		return "Error: unknown time zone " + timezone + ", use an IANA name like Europe/Kyiv. Nothing was saved.", nil
	}

	u := s.User
	if profile != "" {
		u.Profile = profile
	}
	if timezone != "" {
		u.Timezone = timezone
	}
	if name != "" {
		u.Name = name
	}
	if err := t.Users.Update(ctx, u); err != nil {
		return "", err
	}
	return "Profile updated successfully.", nil
}
