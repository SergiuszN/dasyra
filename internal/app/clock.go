package app

import "time"

// now is the clock of the use cases, replaced in tests.
var now = time.Now

// SetClock moves the clock to t and lets it run from there: dasyra chat replays a moment from a tester's chat.
func SetClock(t time.Time) {
	start := time.Now()
	now = func() time.Time { return t.Add(time.Since(start)) }
}

// FreezeClock holds model-facing time fixed during a bench scenario and returns a function to restore it.
// Scenarios run sequentially: the use-case clock is shared by the process.
func FreezeClock(t time.Time) func() {
	previous := now
	now = func() time.Time { return t }
	return func() { now = previous }
}
