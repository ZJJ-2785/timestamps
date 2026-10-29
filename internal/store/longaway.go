package store

// With a screen that never turns off, a night asleep looks like a long time
// away. With Settings.LongAwayOff a long stretch away counts as off instead,
// as if the computer had been shut down: it is left out of the timelines and
// totals as they are read. The stored rows stay as they are, so the option
// covers the past too and can be switched off again.

const (
	longAwayGap   = 30 * 60   // away rows this close make one stretch (the app restarted meanwhile)
	longAwayReach = 24 * 3600 // how far beyond a range rows are read to see whole stretches
)

// longAway reports whether an away stretch from s to e counts as off: it
// lasts min seconds or more (min > 0).
func longAway(s, e, min int64) bool { return min > 0 && e-s >= min }

// dropLongAway leaves the away stretches that count as off out of rows
// (ordered by start). A stretch is a run of away rows, each starting within
// longAwayGap of the end of the one before.
func dropLongAway(rows [][3]int64, min int64) [][3]int64 {
	out := make([][3]int64, 0, len(rows))
	for i := 0; i < len(rows); {
		j, e := i+1, rows[i][1]
		if rows[i][2] == Away {
			for j < len(rows) && rows[j][2] == Away && rows[j][0]-e <= longAwayGap {
				e = max64(e, rows[j][1])
				j++
			}
			if longAway(rows[i][0], e, min) {
				i = j
				continue
			}
		}
		out = append(out, rows[i:j]...)
		i = j
	}
	return out
}

// AwayStretchStart is where the away stretch that an away row starting at s
// belongs to begins, as dropLongAway joins them: at the earliest of the away
// rows before it that each end within longAwayGap of the next one's start
// (looking back as far as usageRows reads).
func (db *DB) AwayStretchStart(s int64) int64 {
	rows, err := db.sql.Query("SELECT s, e, state FROM usage_seg WHERE s >= ? AND s < ? ORDER BY s DESC", db.lowS(TUsage, s-longAwayReach), s)
	if err != nil {
		return s
	}
	defer rows.Close()
	for rows.Next() {
		var rs, re, state int64
		rows.Scan(&rs, &re, &state)
		if state != Away || s-re > longAwayGap {
			break
		}
		s = rs
	}
	return s
}

// usageRows reads the computer-usage rows overlapping [a, b), ordered by
// start, without the long away stretches when that option is on. Those can
// begin or end outside the range, so the rows are then read longAwayReach
// beyond it on both sides: a stretch reaching further lasts about a day at
// least, and counts anyway.
func (db *DB) usageRows(a, b int64) ([][3]int64, error) {
	min := db.longAway.Load()
	lo, hi := a, b
	if min > 0 {
		lo, hi = a-longAwayReach, b+longAwayReach
	}
	rows, err := db.sql.Query("SELECT s, e, state FROM usage_seg WHERE s >= ? AND s < ? AND e > ? ORDER BY s", db.lowS(TUsage, lo), hi, lo)
	if err != nil {
		return nil, err
	}
	out := [][3]int64{}
	for rows.Next() {
		var r [3]int64
		rows.Scan(&r[0], &r[1], &r[2])
		out = append(out, r)
	}
	rows.Close()
	if min == 0 {
		return out, nil
	}
	n := 0
	for _, r := range dropLongAway(out, min) {
		if r[0] < b && r[1] > a {
			out[n] = r
			n++
		}
	}
	return out[:n], nil
}
