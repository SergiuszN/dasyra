package dom

import (
	"testing"
	"time"
)

func TestParseLocalKeepsWallClockAndConvertsOffsets(t *testing.T) {
	u := &User{Timezone: "Europe/Kyiv"}
	got, err := u.ParseLocal("2026-10-10 09:00:00")
	if err != nil || got.Format(DateTimeLayout) != "2026-10-10 09:00:00" || got.Location().String() != "Europe/Kyiv" {
		t.Fatalf("got %v %v", got, err)
	}
	got, err = u.ParseLocal("2026-10-10T06:00:00Z")
	if err != nil || got.Format(DateTimeLayout) != "2026-10-10 09:00:00" {
		t.Fatalf("offset not converted: %v %v", got, err)
	}
	got, err = u.ParseLocal("2026-10-10 Sat 09:00")
	if err != nil || got.Format(DateTimeLayout) != "2026-10-10 09:00:00" {
		t.Fatalf("the shown format not parsed: %v %v", got, err)
	}
	if _, err := u.ParseLocal("tomorrow-ish"); err == nil {
		t.Fatal("garbage must fail")
	}
}

func TestLocalFallsBackToDefaultZone(t *testing.T) {
	u := &User{Timezone: "Mars/Olympus"}
	if got := u.Location().String(); got != DefaultTimezone {
		t.Fatalf("got %s", got)
	}
	instant := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)
	if got := u.Local(instant).Format("15:04"); got != "10:00" {
		t.Fatalf("got %s", got)
	}
}

func TestValidTimezone(t *testing.T) {
	for zone, want := range map[string]bool{"Europe/Kyiv": true, "UTC": true, "EST": false, "Local": false, "": false, "Europe/Nowhere": false} {
		if got := ValidTimezone(zone); got != want {
			t.Errorf("%q: got %v", zone, got)
		}
	}
}

func TestDisplayNameAndProfile(t *testing.T) {
	u := &User{Profile: "  \n"}
	if u.DisplayName() != "the user" || u.HasProfile() {
		t.Fatal("empty user")
	}
}

func TestHistoryHelpers(t *testing.T) {
	h := History{
		{ID: 5, Role: RoleUser}, {ID: 4, Role: RoleUser}, {ID: 3, Role: RoleAssistant}, {ID: 2, Role: RoleUser},
	}
	if len(h.Unanswered()) != 2 || h.LastFromUser().ID != 5 || h.Contains(3) == nil || h.Contains(9) != nil {
		t.Fatal("history helpers")
	}
	if HistoryLimit(0) != 12 || HistoryLimit(11) != 23 || HistoryLimit(12) != 12 {
		t.Fatal("history window")
	}
}

func TestTodoLine(t *testing.T) {
	u := &User{Timezone: "Europe/Warsaw"}
	done := time.Date(2026, 10, 12, 8, 49, 0, 0, time.UTC)
	cases := map[string]Todo{
		"#1 2026-10-12 Mon, today: Invoice":                        {ID: 1, Text: "Invoice", Day: "2026-10-12"},
		"#2 2026-10-11 Sun, overdue: Laundry":                      {ID: 2, Text: "Laundry", Day: "2026-10-11"},
		"#3 no day: Brushes":                                       {ID: 3, Text: "Brushes"},
		"#4 2026-10-12 Mon, today: Mail, done at 2026-10-12 10:49": {ID: 4, Text: "Mail", Day: "2026-10-12", DoneAt: &done},
	}
	for want, todo := range cases {
		if got := todo.Line(u, "2026-10-12"); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}
