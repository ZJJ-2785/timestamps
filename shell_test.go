package main

import (
	"math"
	"testing"
	"time"
	_ "time/tzdata"
)

// The tray and window ring follows the clock, the same as the window's logo.
func TestRingFollowsTheClock(t *testing.T) {
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Skip(err)
	}
	for _, c := range []struct {
		at   time.Time
		frac float64
	}{
		{time.Date(2026, 10, 28, 0, 0, 0, 0, london), 0},
		{time.Date(2026, 10, 28, 12, 0, 0, 0, london), 0.5},
		{time.Date(2026, 10, 28, 18, 0, 0, 0, london), 0.75},
		{time.Date(2026, 10, 28, 19, 55, 30, 0, london), 239.0 / 288},
		{time.Date(2026, 10, 28, 23, 59, 59, 0, london), 287.0 / 288},
		// The 25-hour day when the clocks go back: 19:55 is still 19:55.
		{time.Date(2026, 10, 25, 19, 55, 0, 0, london), 239.0 / 288},
		// The 23-hour day when they go forward.
		{time.Date(2026, 3, 29, 18, 0, 0, 0, london), 0.75},
	} {
		if _, frac, _ := ringAt(c.at, false); math.Abs(frac-c.frac) > 1e-9 {
			t.Errorf("ringAt(%s) = %.4f, want %.4f", c.at.Format("Jan 2 15:04"), frac, c.frac)
		}
	}
	k1, _, _ := ringAt(time.Date(2026, 10, 28, 19, 55, 0, 0, london), false)
	k2, _, _ := ringAt(time.Date(2026, 10, 28, 19, 59, 0, 0, london), false)
	k3, _, _ := ringAt(time.Date(2026, 10, 28, 20, 0, 0, 0, london), false)
	if k1 != k2 || k2 == k3 {
		t.Errorf("the key should change every 5 minutes: %q %q %q", k1, k2, k3)
	}
	if kp, _, _ := ringAt(time.Date(2026, 10, 28, 19, 55, 0, 0, london), true); kp == k1 {
		t.Error("pausing should change the key")
	}
}
