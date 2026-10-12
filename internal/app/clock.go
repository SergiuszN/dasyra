package app

import "time"

// now is the clock of the use cases, replaced in tests.
var now = time.Now

// SetClock moves the clock to t and lets it run from there: dasyra chat replays a moment from a tester's chat.
func SetClock(t time.Time) {
	start := time.Now()
	now = func() time.Time { return t.Add(time.Since(start)) }
}
