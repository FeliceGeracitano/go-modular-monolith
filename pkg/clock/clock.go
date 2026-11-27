// Package clock abstracts time so business code is deterministic under test.
package clock

import "time"

// Clock tells the current time.
type Clock interface {
	Now() time.Time
}

// System is the real clock. It always reports UTC.
type System struct{}

// Now implements Clock.
func (System) Now() time.Time { return time.Now().UTC() }

// Fixed is a clock frozen at a given instant, for tests.
type Fixed time.Time

// Now implements Clock.
func (f Fixed) Now() time.Time { return time.Time(f) }
