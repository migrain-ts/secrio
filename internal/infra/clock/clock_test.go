package clock

import (
	"testing"
	"time"

	"secrio/internal/app"
)

var (
	_ app.Clock = System{}
	_ app.Clock = (*Fake)(nil)
)

func TestSystemIsUTC(t *testing.T) {
	if (System{}).Now().Location() != time.UTC {
		t.Fatal("not UTC")
	}
}

func TestFake(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f := NewFake(start)
	f.Advance(time.Minute)
	if !f.Now().Equal(start.Add(time.Minute)) {
		t.Fatal("advance")
	}
	f.Set(start)
	if !f.Now().Equal(start) {
		t.Fatal("set")
	}
}
