package dom

import "testing"

func TestNoteTextSkipsTheTitleTheTextOpensWith(t *testing.T) {
	if got := (Note{Title: "Шарлотка", Content: "шарлотка с яблоками\n4 яйца"}).Text(); got != "шарлотка с яблоками\n4 яйца" {
		t.Fatalf("got %q", got)
	}
	if got := (Note{Title: "Борщ", Content: "свёкла", Source: "https://x.pl"}).Text(); got != "Борщ\n\nсвёкла\n\nhttps://x.pl" {
		t.Fatalf("got %q", got)
	}
}
