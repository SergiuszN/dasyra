package httpd

import (
	"fmt"
	"html/template"
	"math"
	"strings"
	"time"

	"dasyra/internal/arch/sqlite"
	"dasyra/internal/dom"
)

type loginView struct {
	CSRF       string
	Configured bool
	Error      string
}

type panelView struct {
	Title, Path, Period, CSRF string
	Crumbs                    string // back link on the user page
	Totals                    sqlite.Totals
	Replies                   int
	ShowReplies               bool // hidden for calls without a user
	Chart                     chart
	Users                     []userRow
	Purposes, Models          breakdown
	TrackedSince              string
	User                      *dom.User
	Cookbook                  cookbookForm // on the user page
	Notice, Error             string
}

func (v panelView) Periods() any { return periods }

type userRow struct {
	sqlite.Group
	Label   string
	Replies int
	Total   float64 // all users' cost, for the share
	Bar     int     // px, relative to the most expensive user
}

func userRows(groups []sqlite.Group, replies map[int64]int) []userRow {
	var total, top float64
	for _, g := range groups {
		total, top = total+g.Cost, math.Max(top, g.Cost)
	}
	rows := make([]userRow, len(groups))
	for i, g := range groups {
		rows[i] = userRow{Group: g, Label: "No user", Replies: replies[g.UserID], Total: total}
		if g.UserID > 0 {
			rows[i].Label = nameOr(g.Name)
		}
		if top > 0 {
			rows[i].Bar = int(math.Round(g.Cost / top * 60))
		}
	}
	return rows
}

func sumReplies(replies map[int64]int) int {
	n := 0
	for _, r := range replies {
		n += r
	}
	return n
}

type breakdown struct {
	Heading string
	Rows    []labeled
	Total   float64
}

type labeled struct {
	sqlite.Group
	Label string
}

func breakdownOf(heading string, groups []sqlite.Group, label func(string) string) breakdown {
	b := breakdown{Heading: heading}
	for _, g := range groups {
		b.Rows, b.Total = append(b.Rows, labeled{g, label(g.Name)}), b.Total+g.Cost
	}
	return b
}

var purposes = map[string]string{
	"chat":          "Chat replies",
	"check_in":      "Check-ins",
	"check_in_gate": "Check-in gate",
	"reminder":      "Reminders and digests",
	"reflection":    "Nightly reflection",
	"web_search":    "Web search: answer synthesis",
	"text_reader":   "Reading pages and files",
	"transcription": "Voice transcription",
	"embedding":     "Embeddings for search by meaning",
}

func purposeLabel(p string) string {
	if label, ok := purposes[p]; ok {
		return label
	}
	return p
}

// modelLabel drops the provider prefix, it only widens the table.
func modelLabel(m string) string {
	_, name, found := strings.Cut(m, "/")
	if !found {
		return m
	}
	return name
}

type day struct {
	date time.Time
	cost float64
}

type chart struct {
	Cols        []col
	First, Last string
	Max         float64
	ShowMax     bool
}

type col struct {
	Day    string
	Cost   float64
	Height string // percent of the most expensive day
}

func chartOf(days []day) chart {
	var c chart
	for _, d := range days {
		c.Max = math.Max(c.Max, d.cost)
	}
	for _, d := range days {
		height := 0.5
		if c.Max > 0 {
			height = math.Max(0.5, d.cost/c.Max*100)
		}
		c.Cols = append(c.Cols, col{d.date.Format("2 Jan"), d.cost, fmt.Sprintf("%.2f", height)})
	}
	if len(c.Cols) > 0 {
		c.First, c.Last, c.ShowMax = c.Cols[0].Day, c.Cols[len(c.Cols)-1].Day, len(c.Cols) > 2
	}
	return c
}

func nameOr(name string) string {
	if name == "" {
		return "No name"
	}
	return name
}

var funcs = template.FuncMap{
	"money":   money,
	"tokens":  tokens,
	"percent": percent,
	"num":     func(n int) string { return fixed(float64(n), 0) },
	"minutes": minutes,
	"per":     func(cost float64, n int) string { return money(cost / float64(n)) },
	"stamp":   stamp,
	"float":   func(n int) float64 { return float64(n) },
}

// money keeps fractions of a cent visible: transcription and page reading must not look free.
func money(usd float64) string {
	switch a := math.Abs(usd); {
	case a >= 10:
		return "$" + fixed(usd, 2)
	case a >= 0.01 || usd == 0:
		return "$" + fixed(usd, 4)
	}
	return "$" + fixed(usd, 6)
}

func tokens(n int) string {
	switch c := float64(n); {
	case c >= 1_000_000:
		return fixed(c/1_000_000, 2) + "M"
	case c >= 10_000:
		return fixed(c/1_000, 1) + "k"
	}
	return fixed(float64(n), 0)
}

func percent(part, total float64) string {
	if total <= 0 {
		return "—"
	}
	return fmt.Sprintf("%.0f%%", math.Round(part/total*100))
}

func minutes(seconds float64) string {
	if seconds <= 0 {
		return ""
	}
	return fixed(seconds/60, 1) + " min"
}

// stamp shows a stored server time as "9 Oct 14:30".
func stamp(s string) string {
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		return ""
	}
	return t.Format("2 Jan 15:04")
}

// fixed is number_format with a space as the thousands separator.
func fixed(v float64, decimals int) string {
	s := fmt.Sprintf("%.*f", decimals, math.Abs(v))
	whole, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	if v < 0 && strings.Trim(s, "0.") != "" {
		b.WriteByte('-')
	}
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	if frac != "" {
		b.WriteByte('.')
		b.WriteString(frac)
	}
	return b.String()
}
