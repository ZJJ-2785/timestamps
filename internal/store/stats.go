package store

import (
	"sort"
	"strings"
	"time"
)

// Total is a named duration in a ranking.
type Total struct {
	ID    int64  `json:"id,omitempty"`
	Key   string `json:"key"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
	Icon  bool   `json:"icon,omitempty"`
	IconV int64  `json:"iconV,omitempty"`
	Kind  int    `json:"kind,omitempty"`
	Secs  int64  `json:"secs"`
}

// DayStat summarises one calendar day.
type DayStat struct {
	Date   string `json:"date"`
	Active int64  `json:"active"`
	Away   int64  `json:"away"`
	First  int64  `json:"first"`
	Last   int64  `json:"last"`
	Tagged int64  `json:"tagged"`
}

// Stats is the statistics view payload.
type Stats struct {
	From   int64        `json:"from"`
	To     int64        `json:"to"`
	Active int64        `json:"active"`
	Away   int64        `json:"away"`
	Tagged int64        `json:"tagged"`
	Days   []DayStat    `json:"days"`
	Heat   [7][24]int64 `json:"heat"`
	Apps   []Total      `json:"apps"`
	Sites  []Total      `json:"sites"`
	Files  []Total      `json:"files"`
	Tags   []Total      `json:"tags"`
	Focus  Focus        `json:"focus"`
	Trend  []Series     `json:"trend"` // the top applications, per day
	Prev   *Period      `json:"prev,omitempty"`
}

// Focus breaks activity into sessions: stretches of activity with no break
// longer than sessionGap. Sessions under a minute are left out.
type Focus struct {
	Sessions int      `json:"sessions"`
	Longest  [2]int64 `json:"longest"`  // start and end of the longest session
	Lengths  [5]int   `json:"lengths"`  // under 15 min, 15-30 min, 30-60 min, 1-2 h, 2 h and more
	Switches int      `json:"switches"` // changes from one application to another
}

// Series is one application's time per day of the range.
type Series struct {
	ID   int64   `json:"id"`
	Days []int64 `json:"days"`
}

// Period sums a range, for comparing with another one.
type Period struct {
	From   int64 `json:"from"`
	To     int64 `json:"to"`
	Active int64 `json:"active"`
	Away   int64 `json:"away"`
	Tagged int64 `json:"tagged"`
	Days   int   `json:"days"` // days with activity
}

const (
	sessionGap = 120 // seconds between active stretches that still make one session
	trendApps  = 5
)

func (f *Focus) add(s, e int64) {
	n := e - s
	if n < 60 {
		return
	}
	f.Sessions++
	if n > f.Longest[1]-f.Longest[0] {
		f.Longest = [2]int64{s, e}
	}
	switch {
	case n < 15*60:
		f.Lengths[0]++
	case n < 30*60:
		f.Lengths[1]++
	case n < 3600:
		f.Lengths[2]++
	case n < 2*3600:
		f.Lengths[3]++
	default:
		f.Lengths[4]++
	}
}

// Stats aggregates every timeline over [a, b). With mergeSubdomains,
// websites are grouped by registrable domain (t.a.com + c.a.com -> a.com).
func (db *DB) Stats(a, b int64, mergeSubdomains bool) (*Stats, error) {
	st := &Stats{From: a, To: b, Apps: []Total{}, Sites: []Total{}, Files: []Total{}, Tags: []Total{}, Trend: []Series{}}
	bounds := dayStarts(a, b)
	st.Days = make([]DayStat, len(bounds)-1)
	for i := range st.Days {
		st.Days[i].Date = time.Unix(bounds[i], 0).In(time.Local).Format("2006-01-02")
	}

	usage, err := db.usageRows(a, b)
	if err != nil {
		return nil, err
	}
	spill := make([]bool, len(st.Days))
	var cs, ce int64 // the session being built
	for _, r := range usage {
		s, e, state := r[0], r[1], r[2]
		if s < a {
			s = a
		}
		if e > b {
			e = b
		}
		orig := s
		splitDays(bounds, s, e, func(i int, ps, pe int64) {
			ds := &st.Days[i]
			if state == Active {
				ds.Active += pe - ps
				// Work continuing from the previous evening doesn't count as
				// the start of the day unless nothing else started that day.
				startsHere := orig >= bounds[i]
				if startsHere && (ds.First == 0 || spill[i] || ps < ds.First) {
					ds.First, spill[i] = ps, false
				} else if !startsHere && ds.First == 0 {
					ds.First, spill[i] = ps, true
				}
				if pe > ds.Last {
					ds.Last = pe
				}
			} else {
				ds.Away += pe - ps
			}
		})
		if state == Active {
			st.Active += e - s
			heat(&st.Heat, s, e)
			if ce > 0 && s-ce <= sessionGap {
				if e > ce {
					ce = e
				}
			} else {
				st.Focus.add(cs, ce)
				cs, ce = s, e
			}
		} else {
			st.Away += e - s
		}
	}
	st.Focus.add(cs, ce)

	// Applications.
	// Aggregate first, then join: joining per segment would read every
	// app row (and its icon) once per segment.
	rows, err := db.sql.Query(`SELECT g.app, a.key, a.name, a.color, COALESCE(length(a.icon), 0), g.t FROM
		(SELECT app, SUM(MIN(e, ?) - MAX(s, ?)) t FROM app_seg WHERE s >= ? AND s < ? AND e > ? GROUP BY app) g
		JOIN app a ON a.id = g.app ORDER BY g.t DESC LIMIT 300`, b, a, db.lowS(TApp, a), b, a)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t Total
		rows.Scan(&t.ID, &t.Key, &t.Name, &t.Color, &t.IconV, &t.Secs)
		t.Icon = t.IconV > 0
		st.Apps = append(st.Apps, t)
	}
	rows.Close()
	// Switches and the top applications per day read every application
	// segment again: that runs alongside the websites, files and tags.
	pass := make(chan error, 1)
	go func() { pass <- db.appPass(st, bounds, a, b) }()

	// Websites by domain.
	rows, err = db.sql.Query(`SELECT d.grp, SUM(g.t) t FROM
		(SELECT doc, SUM(MIN(e, ?) - MAX(s, ?)) t FROM doc_seg WHERE s >= ? AND s < ? AND e > ? GROUP BY doc) g
		JOIN doc d ON d.id = g.doc WHERE d.kind = 2 GROUP BY d.grp ORDER BY t DESC LIMIT 1000`, b, a, db.lowS(TDoc, a), b, a)
	if err != nil {
		return nil, err
	}
	sites := map[string]int64{}
	for rows.Next() {
		var key string
		var secs int64
		rows.Scan(&key, &secs)
		if mergeSubdomains {
			key = BaseDomain(key)
		}
		sites[key] += secs
	}
	rows.Close()
	colors := db.SiteColors() // from the sites' icons; "" when unknown
	for k, v := range sites {
		st.Sites = append(st.Sites, Total{Key: k, Name: k, Kind: KindWeb, Color: colors[strings.ToLower(k)], Secs: v})
	}
	sortTotals(st.Sites)

	// Files by full path.
	rows, err = db.sql.Query(`SELECT d.id, d.key, d.name, g.t FROM
		(SELECT doc, SUM(MIN(e, ?) - MAX(s, ?)) t FROM doc_seg WHERE s >= ? AND s < ? AND e > ? GROUP BY doc) g
		JOIN doc d ON d.id = g.doc WHERE d.kind = 1 ORDER BY g.t DESC LIMIT 300`, b, a, db.lowS(TDoc, a), b, a)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t Total
		rows.Scan(&t.ID, &t.Key, &t.Name, &t.Secs)
		t.Kind, t.Color = KindFile, ColorFor(t.Name)
		st.Files = append(st.Files, t)
	}
	rows.Close()

	// Tags.
	metas := db.TagMetas()
	tagTotals := map[string]int64{}
	for _, tg := range db.TagsIn(a, b) {
		s, e := tg.S, tg.E
		if s < a {
			s = a
		}
		if e > b {
			e = b
		}
		tagTotals[tg.Tags] += e - s
		st.Tagged += e - s
		splitDays(bounds, s, e, func(i int, ps, pe int64) { st.Days[i].Tagged += pe - ps })
	}
	for k, v := range tagTotals {
		c := metas[k].Color
		if c == "" {
			c = ColorFor(k)
		}
		st.Tags = append(st.Tags, Total{Key: k, Name: k, Secs: v, Color: c})
	}
	sortTotals(st.Tags)
	if err := <-pass; err != nil {
		return nil, err
	}
	return st, nil
}

// appPass goes through the application segments in order, counting the
// switches from one application to another and adding up the top
// applications' time per day (st.Trend).
func (db *DB) appPass(st *Stats, bounds []int64, a, b int64) error {
	idx := map[int64]int{}
	if len(st.Days) > 1 {
		for _, app := range st.Apps {
			if len(st.Trend) == trendApps {
				break
			}
			idx[app.ID] = len(st.Trend)
			st.Trend = append(st.Trend, Series{ID: app.ID, Days: make([]int64, len(st.Days))})
		}
	}
	rows, err := db.sql.Query("SELECT app, s, e FROM app_seg WHERE s >= ? AND s < ? AND e > ? ORDER BY s", db.lowS(TApp, a), b, a)
	if err != nil {
		return err
	}
	defer rows.Close()
	var prev int64
	day := 0 // the day the segments have reached
	for rows.Next() {
		var app, s, e int64
		rows.Scan(&app, &s, &e)
		if prev != 0 && app != prev {
			st.Focus.Switches++
		}
		prev = app
		i, ok := idx[app]
		if !ok {
			continue
		}
		s, e = max64(s, a), min64(e, b)
		for day < len(bounds)-2 && bounds[day+1] <= s {
			day++
		}
		days := st.Trend[i].Days
		for d := day; d < len(bounds)-1 && bounds[d] < e; d++ {
			if pe, ps := min64(e, bounds[d+1]), max64(s, bounds[d]); pe > ps {
				days[d] += pe - ps
			}
		}
	}
	return nil
}

// Totals sums activity, away time and tags over [a, b).
func (db *DB) Totals(a, b int64) (Period, error) {
	p := Period{From: a, To: b}
	bounds := dayStarts(a, b)
	active := make([]bool, len(bounds)-1)
	usage, err := db.usageRows(a, b)
	if err != nil {
		return p, err
	}
	for _, r := range usage {
		s, e, state := max64(r[0], a), min64(r[1], b), r[2]
		if state != Active {
			p.Away += e - s
			continue
		}
		p.Active += e - s
		splitDays(bounds, s, e, func(i int, _, _ int64) { active[i] = true })
	}
	for _, ok := range active {
		if ok {
			p.Days++
		}
	}
	for _, tg := range db.TagsIn(a, b) {
		p.Tagged += min64(tg.E, b) - max64(tg.S, a)
	}
	return p, nil
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func heat(h *[7][24]int64, s, e int64) {
	for s < e {
		t := time.Unix(s, 0).In(time.Local)
		next := time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, time.Local).Unix()
		if next <= s {
			next = s + 3600
		}
		pe := e
		if next < pe {
			pe = next
		}
		wd := (int(t.Weekday()) + 6) % 7 // Monday = 0
		h[wd][t.Hour()] += pe - s
		s = pe
	}
}

func sortTotals(t []Total) {
	sort.Slice(t, func(i, j int) bool { return t[i].Secs > t[j].Secs })
}

// Detail is the drill-down for one app, site, file or tag.
type Detail struct {
	Days  []DayStat `json:"days"`
	Items []Total   `json:"items"`
	Total int64     `json:"total"`
}

// Detail breaks one entity down by day and by sub-item (window titles for
// apps, URLs for sites).
func (db *DB) Detail(kind string, key string, id int64, a, b int64, mergeSubdomains bool) (*Detail, error) {
	bounds := dayStarts(a, b)
	d := &Detail{Days: make([]DayStat, len(bounds)-1), Items: []Total{}}
	for i := range d.Days {
		d.Days[i].Date = time.Unix(bounds[i], 0).In(time.Local).Format("2006-01-02")
	}
	var q string
	var args []any
	switch kind {
	case "app":
		q = `SELECT x.s, x.e, t.text, x.title FROM app_seg x JOIN title t ON t.id = x.title
			WHERE x.app = ? AND x.s >= ? AND x.s < ? AND x.e > ?`
		args = []any{id, db.lowS(TApp, a), b, a}
	case "site":
		like := "\x00" // matches nothing unless subdomains are merged
		if mergeSubdomains {
			like = "%." + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(key)
		}
		q = `SELECT x.s, x.e, d.key, d.id FROM doc_seg x JOIN doc d ON d.id = x.doc
			WHERE d.kind = 2 AND (d.grp = ? OR d.grp LIKE ? ESCAPE '\') AND x.s >= ? AND x.s < ? AND x.e > ?`
		args = []any{key, like, db.lowS(TDoc, a), b, a}
	case "file":
		q = `SELECT x.s, x.e, a.name, x.app FROM doc_seg x LEFT JOIN app a ON a.id = x.app
			WHERE x.doc = ? AND x.s >= ? AND x.s < ? AND x.e > ?`
		args = []any{id, db.lowS(TDoc, a), b, a}
	case "tag":
		q = `SELECT s, e, notes, 0 FROM tag_seg WHERE tags = ? AND s >= ? AND s < ? AND e > ?`
		args = []any{key, db.lowS(TTag, a), b, a}
	default:
		return d, nil
	}
	rows, err := db.sql.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := map[string]*Total{}
	for rows.Next() {
		var s, e, ref int64
		var name *string
		rows.Scan(&s, &e, &name, &ref)
		if s < a {
			s = a
		}
		if e > b {
			e = b
		}
		if e <= s {
			continue
		}
		d.Total += e - s
		splitDays(bounds, s, e, func(i int, ps, pe int64) { d.Days[i].Active += pe - ps })
		n := ""
		if name != nil {
			n = *name
		}
		it := items[n]
		if it == nil {
			it = &Total{Key: n, Name: n, ID: ref}
			items[n] = it
		}
		it.Secs += e - s
	}
	for _, it := range items {
		d.Items = append(d.Items, *it)
	}
	sortTotals(d.Items)
	if len(d.Items) > 500 {
		d.Items = d.Items[:500]
	}
	return d, nil
}
