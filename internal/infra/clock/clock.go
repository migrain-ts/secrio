// Package clock provides app.Clock implementations.
package clock

import "time"

// System is the real clock, in UTC.
type System struct{}

func (System) Now() time.Time { return time.Now().UTC() }
