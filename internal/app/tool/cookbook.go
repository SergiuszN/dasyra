package tool

import (
	"fmt"
	"net/netip"
	"net/url"
	"strings"

	"dasyra/internal/dom"
)

// The cookbook tools work with the user's recipe server (Mealie), connected per user in the admin panel.
// Users without one never see them.

const cookbookName = "the cookbook"

// withCookbook makes a tool conditional: it is there only for users with a cookbook connected.
type withCookbook struct{}

func (withCookbook) Enabled(s *Session) bool { return s.Cookbook != nil }

// cookbookError is a result, not a tool failure: the server may be down, the model tells the user.
func cookbookError(err error) string {
	return "Error: " + cookbookName + " did not do it: " + err.Error()
}

type recipeRow struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Servings    string   `json:"servings,omitempty"`
	TotalTime   string   `json:"total_time,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Ingredients []string `json:"ingredients,omitempty"`
	Steps       []string `json:"steps,omitempty"`
	Notes       []string `json:"notes,omitempty"`
	Source      string   `json:"source,omitempty"`
	Link        string   `json:"link,omitempty"`
}

func rowOf(r dom.Recipe) recipeRow {
	return recipeRow{r.Slug, r.Name, r.Description, r.Servings, r.TotalTime, r.Tags, r.Ingredients, r.Steps, r.Notes, r.Source, r.Link}
}

// recipeText is a recipe as the chat gets it: plain text, Telegram shows no markup from tools.
func recipeText(r dom.Recipe) string {
	var b strings.Builder
	b.WriteString(r.Name)
	var facts []string
	if r.Servings != "" {
		facts = append(facts, "👥 "+r.Servings)
	}
	if r.TotalTime != "" {
		facts = append(facts, "⏱ "+r.TotalTime)
	}
	if len(facts) > 0 {
		b.WriteString("\n" + strings.Join(facts, " · "))
	}
	if r.Description != "" {
		b.WriteString("\n\n" + r.Description)
	}
	if len(r.Ingredients) > 0 {
		b.WriteString("\n")
		for _, line := range r.Ingredients {
			if title, ok := strings.CutPrefix(line, "## "); ok {
				b.WriteString("\n" + title + ":")
				continue
			}
			b.WriteString("\n• " + line)
		}
	}
	if len(r.Steps) > 0 {
		b.WriteString("\n")
		for i, step := range r.Steps {
			fmt.Fprintf(&b, "\n%d. %s", i+1, step)
		}
	}
	for _, note := range r.Notes {
		b.WriteString("\n\n" + note)
	}
	if r.Link != "" {
		b.WriteString("\n\n" + r.Link)
	}
	return b.String()
}

// publicPage allows only pages on the internet: the cookbook server fetches the URL itself, from inside the network.
func publicPage(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return false
	}
	host := u.Hostname()
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.IsGlobalUnicast() && !ip.IsPrivate()
	}
	return strings.Contains(host, ".") && !strings.HasSuffix(host, ".local") && !strings.HasSuffix(host, ".internal")
}
