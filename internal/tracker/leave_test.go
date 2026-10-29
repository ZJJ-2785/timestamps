package tracker

import (
	"testing"
	"time"

	"timestamps/internal/store"
)

// Playing sound keeps you active, but doesn't bring you back once away.
func TestSoundKeepsActive(t *testing.T) {
	st := &store.Settings{TreatSoundAsActive: true}
	t0 := time.Now()
	tr := &Tracker{usageState: store.Active}
	tr.lastSound.Store(t0.UnixMilli())
	if last, _ := tr.presence(t0, time.Hour, st); last.UnixMilli() != t0.UnixMilli() {
		t.Errorf("active: last = %v ago, want the sound's time", t0.Sub(last))
	}
	tr.usageState = store.Away
	if last, _ := tr.presence(t0, time.Hour, st); !last.Equal(t0.Add(-time.Hour)) {
		t.Errorf("away: last = %v ago, want the last input's time", t0.Sub(last))
	}
}

// After Leave the user is away until the next input, once the click's own
// input is past; playing sound doesn't bring them back.
func TestLeaveUntilInput(t *testing.T) {
	tr := &Tracker{usageState: store.Active}
	st := &store.Settings{TreatSoundAsActive: true}
	t0 := time.Now()
	at := func(d time.Duration) time.Time { return t0.Add(d) }

	tr.leaveAt = t0
	for _, c := range []struct {
		name  string
		now   time.Duration
		idle  time.Duration
		sound bool
	}{
		{"the click", time.Second, time.Second, false},
		{"the hand leaving the mouse", 10 * time.Second, 8 * time.Second, false},
		{"sound playing", time.Minute, time.Hour, true},
	} {
		if c.sound {
			tr.lastSound.Store(at(c.now).UnixMilli())
		}
		if last, leaving := tr.presence(at(c.now), c.idle, st); !leaving || !last.Equal(t0) {
			t.Errorf("%s: last = %v, leaving = %v; want still away since Leave", c.name, last.Sub(t0), leaving)
		}
	}

	if last, leaving := tr.presence(at(2*time.Minute), time.Second, st); leaving || !last.Equal(at(2*time.Minute-time.Second)) {
		t.Errorf("input: last = %v, leaving = %v; want back at the input", last.Sub(t0), leaving)
	}
	if !tr.leaveAt.IsZero() {
		t.Error("input should end leaving")
	}

	tr.leaveAt = t0
	tr.Pause(0)
	if !tr.leaveAt.IsZero() {
		t.Error("pausing should end leaving")
	}
}
