// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package program

import "time"

// SimClock is the test-injectable clock seam. realClock wraps time.Now
// and time.AfterFunc; tests inject a fake clock to advance time
// deterministically (instead of time.Sleep) for the 800ms quit window
// and other timer-driven paths.
type SimClock interface {
	Now() time.Time
	AfterFunc(d time.Duration, f func()) Timer
}

// Timer is the minimal abstraction over *time.Timer used by Program.
// Stop returns true if the call stopped the timer from firing.
type Timer interface {
	Stop() bool
}

// realClock is the production SimClock backed by the stdlib time package.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }
func (realClock) AfterFunc(d time.Duration, f func()) Timer {
	return time.AfterFunc(d, f)
}
