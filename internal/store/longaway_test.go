package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLongAway(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	at := func(d, h, min int) int64 { return time.Date(2026, 10, d, h, min, 0, 0, time.Local).Unix() }
	db.BeginUsage(at(27, 21, 0), at(27, 23, 0), Active)
	// The night, in two rows: the app restarted at 03:00.
	db.BeginUsage(at(27, 23, 0), at(28, 3, 0), Away)
	db.BeginUsage(at(28, 3, 5), at(28, 10, 0), Away)
	db.BeginUsage(at(28, 10, 0), at(28, 12, 0), Active)
	db.BeginUsage(at(28, 12, 0), at(28, 12, 30), Away) // lunch
	db.BeginUsage(at(28, 12, 30), at(28, 14, 0), Active)
	db.BeginUsage(at(28, 14, 0), at(28, 19, 0), Away) // out for the afternoon
	db.BeginUsage(at(28, 19, 0), at(28, 20, 0), Active)
	day27, day28, day29 := at(27, 0, 0), at(28, 0, 0), at(29, 0, 0)

	const night, lunch, afternoon = (3*60 + 6*60 + 55) * 60, 30 * 60, 5 * 3600
	check := func(when string, away27, away28 int64, rows28 int) {
		t.Helper()
		for _, c := range []struct{ a, b, away int64 }{{day27, day28, away27}, {day28, day29, away28}} {
			st, err := db.Stats(c.a, c.b, true)
			if err != nil {
				t.Fatal(err)
			}
			p, err := db.Totals(c.a, c.b)
			if err != nil {
				t.Fatal(err)
			}
			if st.Away != c.away || p.Away != c.away || st.Days[0].Away != c.away {
				t.Errorf("%s: away on %s = %d, %d, %d; want %d", when, time.Unix(c.a, 0).Format("Jan 2"), st.Away, p.Away, st.Days[0].Away, c.away)
			}
			if c.a == day28 && (st.Active != 4.5*3600 || p.Active != 4.5*3600) {
				t.Errorf("%s: active = %d, %d; want 4.5 h", when, st.Active, p.Active)
			}
		}
		d, err := db.Day(day28, day29)
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Usage) != rows28 {
			t.Errorf("%s: %d usage rows on the 28th, want %d: %v", when, len(d.Usage), rows28, d.Usage)
		}
	}
	check("option off", 3600, night+lunch+afternoon, 7)

	s := db.Settings()
	if s.LongAwayOff || s.LongAwayHours != 6 {
		t.Fatalf("defaults: %v, %d hours", s.LongAwayOff, s.LongAwayHours)
	}
	s.LongAwayOff = true
	v := db.Version()
	if err := db.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	if db.Version() == v {
		t.Error("switching the option on should change the version (the day view's cache key)")
	}
	// The night (11 h over midnight, in two rows) is off, on both days.
	check("6 hours", 0, lunch+afternoon, 5)
	v = db.Version()
	db.SaveSettings(s)
	if db.Version() != v {
		t.Error("saving the same option should keep the version")
	}

	s.LongAwayHours = 3
	db.SaveSettings(s)
	check("3 hours", 0, lunch, 4)
	s.LongAwayHours = 12
	db.SaveSettings(s)
	check("12 hours", 3600, night+lunch+afternoon, 7)

	// Reopened, the option is read back from the settings.
	s.LongAwayHours = 6
	db.SaveSettings(s)
	db2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	if st, _ := db2.Stats(day28, day29, true); st.Away != lunch+afternoon {
		t.Errorf("reopened: away = %d", st.Away)
	}

	// The away prompt judges the same stretch: the night begins at 23:00.
	if got := db.AwayStretchStart(at(28, 3, 5)); got != at(27, 23, 0) {
		t.Errorf("stretch start = %s, want 23:00", time.Unix(got, 0).Format("Jan 2 15:04"))
	}
	if got := db.AwayStretchStart(at(28, 14, 0)); got != at(28, 14, 0) {
		t.Errorf("stretch after activity starts at %s, want 14:00", time.Unix(got, 0).Format("15:04"))
	}

	// Rows further apart than a restart make separate stretches.
	if got := dropLongAway([][3]int64{{0, 4 * 3600, Away}, {5 * 3600, 9 * 3600, Away}}, 6*3600); len(got) != 2 {
		t.Errorf("two 4-hour stretches an hour apart: %v", got)
	}
}

// A stretch longer than the rows read around a range still counts.
func TestLongAwayWeek(t *testing.T) {
	db := openTemp(t)
	at := func(d, h int) int64 { return time.Date(2026, 10, d, h, 0, 0, 0, time.Local).Unix() }
	db.BeginUsage(at(20, 9), at(28, 8), Away) // a week away, the computer on
	db.BeginUsage(at(28, 8), at(28, 9), Active)
	s := db.Settings()
	s.LongAwayOff = true
	db.SaveSettings(s)
	for d := 20; d <= 28; d++ {
		st, err := db.Stats(at(d, 0), at(d+1, 0), true)
		if err != nil {
			t.Fatal(err)
		}
		if st.Away != 0 || (d == 28 && st.Active != 3600) {
			t.Errorf("Oct %d: away = %d, active = %d", d, st.Away, st.Active)
		}
	}
}
