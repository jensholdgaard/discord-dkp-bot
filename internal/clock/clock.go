package clock

import "time"

// Clock abstracts time operations for testability.
type Clock interface {
	Now() time.Time
}

// Real is a Clock backed by the system clock.
type Real struct{}

// Now returns the current time.
func (Real) Now() time.Time { return time.Now() }

// Mock is a Clock that always returns a fixed time.
type Mock struct {
	T time.Time
}

// Now returns the fixed time.
func (m Mock) Now() time.Time { return m.T }
