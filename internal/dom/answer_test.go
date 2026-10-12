package dom

import (
	"reflect"
	"testing"
)

func TestReaction(t *testing.T) {
	cases := map[string]string{
		"REACTION: 👍":   "👍",
		" REACTION:❤️ ": "❤",
		"REACTION: 🦄":   "👍",
		"REACTION: 🔥\n": "🔥",
	}
	for answer, want := range cases {
		got, ok := Answer(answer).Reaction()
		if !ok || got != want {
			t.Errorf("%q: got %q %v, want %q", answer, got, ok, want)
		}
	}
	for _, answer := range []string{"Отлично! REACTION: 👍", "REACTION: 👍 молодец", ""} {
		if _, ok := Answer(answer).Reaction(); ok {
			t.Errorf("%q must not be a reaction", answer)
		}
	}
}

func TestSilent(t *testing.T) {
	if !Answer(" NO_MESSAGE ").Silent() || !Answer("no_message").Silent() {
		t.Fatal("NO_MESSAGE must be silent")
	}
	if Answer("Привет").Silent() {
		t.Fatal("text must not be silent")
	}
}

func TestPartsSplitOnEmptyLines(t *testing.T) {
	got := Answer("Первое.\n\nВторое\nещё строка\n \n\nТретье").Parts()
	want := []string{"Первое.", "Второе\nещё строка", "Третье"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}

func TestPartsJoinTheTailBeyondFive(t *testing.T) {
	got := Answer("1\n\n2\n\n3\n\n4\n\n5\n\n6\n\n7").Parts()
	want := []string{"1", "2", "3", "4", "5\n6\n7"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}
