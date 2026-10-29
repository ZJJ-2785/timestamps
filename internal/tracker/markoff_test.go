package tracker

import (
	"path/filepath"
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
	if last := tr.presence(t0, time.Hour, st); last.UnixMilli() != t0.UnixMilli() {
		t.Errorf("active: last = %v ago, want the sound's time", t0.Sub(last))
	}
	tr.usageState = store.Away
	if last := tr.presence(t0, time.Hour, st); !last.Equal(t0.Add(-time.Hour)) {
		t.Errorf("away: last = %v ago, want the last input's time", t0.Sub(last))
	}
}

// After MarkOff the time is off until the next input, once the click's own
// input is past; locking the screen doesn't end it.
func TestMarkOffUntilInput(t *testing.T) {
	tr := &Tracker{usageState: store.Active}
	t0 := time.Now()
	at := func(d time.Duration) time.Time { return t0.Add(d) }

	tr.offAt = t0
	for _, c := range []struct {
		name      string
		now, idle time.Duration
		locked    bool
		off       bool
	}{
		{"the click", time.Second, time.Second, false, true},
		{"the hand leaving the mouse", 10 * time.Second, 8 * time.Second, false, true},
		{"Win+L pressed", 20 * time.Second, 0, false, true},
		{"the lock screen up", 21 * time.Second, time.Second, true, true},
		{"locked, an hour later", time.Hour, time.Hour, true, true},
		{"the PIN typed, unlocked", 2 * time.Hour, 0, false, true},
		{"input since unlocking", 2*time.Hour + offLockWait, 0, false, false},
	} {
		if tr.markedOff(at(c.now), c.idle, c.locked) != c.off {
			t.Errorf("%s: off = %v, want %v", c.name, !c.off, c.off)
		}
	}
	if !tr.offAt.IsZero() || !tr.offBack.IsZero() {
		t.Error("input should end it")
	}

	// A single bump of the mouse ends it too.
	tr.offAt = t0
	tr.markedOff(at(time.Hour), time.Minute, false)
	if tr.markedOff(at(time.Hour+offLockWait), time.Minute+offLockWait, false) {
		t.Error("a bump: still off")
	}

	tr.offAt = t0
	tr.Pause(0)
	if !tr.offAt.IsZero() {
		t.Error("pausing should end it")
	}
}

// Marking off ends the open usage row at once; nothing more is recorded.
func TestMarkOffCloses(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	s := now.Add(-time.Hour).Unix()
	tr := &Tracker{db: db, usageState: store.Active, state: "active", lastTick: now.Add(-time.Second)}
	tr.usage = seg{id: db.BeginUsage(s, s, store.Active), s: s, e: s}
	tr.offAt = now // within offGrace of any input, so step needs no real idle time
	st := db.Settings()
	if ev := tr.step(now, &st); ev != "state" || tr.state != "off" || tr.usage.id != 0 {
		t.Fatalf("step: event %q, state %q, usage row %d", ev, tr.state, tr.usage.id)
	}
	if l := tr.Live(); l.State != "off" || l.Since != now.Unix() {
		t.Errorf("live: %s since %d, want off since %d", l.State, l.Since, now.Unix())
	}
	if ev := tr.step(now.Add(time.Second), &st); ev != "" {
		t.Errorf("second step: event %q", ev)
	}
	d, err := db.Day(s-60, now.Unix()+60)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Usage) != 1 || d.Usage[0][0] != s || d.Usage[0][1] != now.Unix() || d.Usage[0][2] != store.Active {
		t.Errorf("usage rows: %v, want one active row ending at the click", d.Usage)
	}
}
