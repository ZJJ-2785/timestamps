package tracker

import (
	"testing"
	"time"
)

func TestSlept(t *testing.T) {
	last := time.Now() // carries a monotonic reading
	if slept(last, last.Add(time.Second)) || slept(time.Time{}, last) {
		t.Error("a second later is no sleep")
	}
	// After a night asleep on macOS only the clock has moved on: the
	// sample's wall time is hours later.
	woke := time.Unix(0, last.UnixNano()+int64(8*time.Hour))
	if !slept(last, woke) {
		t.Error("8 hours later by the clock is sleep")
	}
}

func TestSleeping(t *testing.T) {
	on := true
	defer func(f func() bool) { screenOn = f }(screenOn)
	screenOn = func() bool { return on }
	tr := &Tracker{subs: map[chan Event]bool{}, usageState: -1}
	if tr.sleeping(time.Now()) {
		t.Fatal("awake to begin with")
	}
	tr.Sleep()
	at := tr.asleep
	if !tr.sleeping(at.Add(5 * time.Second)) {
		t.Error("the samples right after going to sleep record nothing")
	}
	on = false
	if !tr.sleeping(at.Add(time.Hour)) {
		t.Error("with every screen asleep (Power Nap) nothing is recorded")
	}
	on = true
	if tr.sleeping(at.Add(8 * time.Hour)) {
		t.Error("awake once a screen is on")
	}
	on = false
	if tr.sleeping(at.Add(9 * time.Hour)) {
		t.Error("once awake, a screen turning off is no longer sleep")
	}
	if s := tr.Live().State; s != "off" {
		t.Errorf("state after sleep = %q", s)
	}
}
