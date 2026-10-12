package dom

import (
	"slices"
	"testing"
)

func TestSearchTermsCutEndingsAndSkipShortWords(t *testing.T) {
	got := SearchTerms("Мотоцикл, и ОК 42 мотоцикл, мотоциклы на даче")
	if want := []string{"мотоц", "мотоци", "даче"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := SearchTerms("Rechnungen für die Firma"); !slices.Equal(got, []string{"rechnu", "für", "die", "firma"}) {
		t.Fatalf("got %v", got)
	}
	if got := SearchTerms("да, ок"); len(got) != 0 {
		t.Fatalf("short words only: %v", got)
	}
}
