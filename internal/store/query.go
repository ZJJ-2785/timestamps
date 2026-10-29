package store

import (
	"database/sql"
	"sort"
	"strings"
	"time"
)

// Tag is one segment on the tag timeline.
type Tag struct {
	ID       int64  `json:"id"`
	S        int64  `json:"s"`
	E        int64  `json:"e"`
	Tags     string `json:"tags"`
	Notes    string `json:"notes"`
	Billable bool   `json:"billable"`
}

// Day is everything the day view needs, in a compact form: segments are
// [start, end, ref...] tuples and referenced names are sent once.
type Day struct {
	From   int64              `json:"from"`
	To     int64              `json:"to"`
	Usage  [][3]int64         `json:"usage"`
	Apps   [][4]int64         `json:"apps"`
	Docs   [][4]int64         `json:"docs"`
	Tags   []Tag              `json:"tags"`
	Shots  []int64            `json:"shots"`
	AppMap map[int64]AppInfo  `json:"appMap"`
	TitMap map[int64]string   `json:"titleMap"`
	DocMap map[int64]DocInfo  `json:"docMap"`
	TagMap map[string]TagMeta `json:"tagMap"`
	// Colors picked from website icons, by site (host and registrable domain).
	SiteColors map[string]string `json:"siteColors"`
}

// TagMeta holds per-tag display settings.
type TagMeta struct {
	Color    string `json:"color"`
	Billable bool   `json:"billable"`
}

// DayRange returns [start, end) unix seconds of a local calendar day.
func DayRange(date string) (int64, int64, error) {
	t, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return 0, 0, err
	}
	return t.Unix(), t.AddDate(0, 0, 1).Unix(), nil
}

// Range returns [start, end) unix seconds for an inclusive local date range.
func Range(from, to string) (int64, int64, error) {
	a, _, err := DayRange(from)
	if err != nil {
		return 0, 0, err
	}
	_, b, err := DayRange(to)
	if err != nil {
		return 0, 0, err
	}
	return a, b, nil
}

func clip(s, e, a, b int64) int64 {
	if s < a {
		s = a
	}
	if e > b {
		e = b
	}
	if e < s {
		return 0
	}
	return e - s
}

// Earliest returns the local midnight of the first recorded day (0 if none).
func (db *DB) Earliest() int64 {
	var m sql.NullInt64
	db.sql.QueryRow("SELECT MIN(s) FROM usage_seg").Scan(&m)
	if !m.Valid {
		return 0
	}
	t := time.Unix(m.Int64, 0).In(time.Local)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local).Unix()
}

// Day loads all timelines overlapping [a, b).
func (db *DB) Day(a, b int64) (*Day, error) {
	d := &Day{From: a, To: b, Usage: [][3]int64{}, Apps: [][4]int64{}, Docs: [][4]int64{}, Tags: []Tag{}, Shots: []int64{},
		AppMap: map[int64]AppInfo{}, TitMap: map[int64]string{}, DocMap: map[int64]DocInfo{}, TagMap: map[string]TagMeta{}}

	usage, err := db.usageRows(a, b)
	if err != nil {
		return nil, err
	}
	d.Usage = usage

	appIDs, titleIDs, docIDs := map[int64]bool{}, map[int64]bool{}, map[int64]bool{}
	rows, err := db.sql.Query("SELECT s, e, app, title FROM app_seg WHERE s >= ? AND s < ? AND e > ? ORDER BY s", db.lowS(TApp, a), b, a)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r [4]int64
		rows.Scan(&r[0], &r[1], &r[2], &r[3])
		d.Apps = append(d.Apps, r)
		appIDs[r[2]] = true
		titleIDs[r[3]] = true
	}
	rows.Close()

	rows, err = db.sql.Query("SELECT s, e, doc, app FROM doc_seg WHERE s >= ? AND s < ? AND e > ? ORDER BY s", db.lowS(TDoc, a), b, a)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r [4]int64
		rows.Scan(&r[0], &r[1], &r[2], &r[3])
		d.Docs = append(d.Docs, r)
		docIDs[r[2]] = true
	}
	rows.Close()

	d.Tags = db.TagsIn(a, b)
	rows, err = db.sql.Query("SELECT ts FROM shot WHERE ts >= ? AND ts < ? ORDER BY ts", a, b)
	if err == nil {
		for rows.Next() {
			var t int64
			rows.Scan(&t)
			d.Shots = append(d.Shots, t)
		}
		rows.Close()
	}

	db.fillMaps(d, appIDs, titleIDs, docIDs)
	return d, nil
}

func idList(m map[int64]bool) string {
	var sb strings.Builder
	first := true
	for id := range m {
		if !first {
			sb.WriteByte(',')
		}
		first = false
		sb.WriteString(itoa(id))
	}
	return sb.String()
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func (db *DB) fillMaps(d *Day, apps, titles, docs map[int64]bool) {
	if len(apps) > 0 {
		rows, err := db.sql.Query("SELECT id, key, name, path, color, COALESCE(length(icon), 0) FROM app WHERE id IN (" + idList(apps) + ")")
		if err == nil {
			for rows.Next() {
				var a AppInfo
				rows.Scan(&a.ID, &a.Key, &a.Name, &a.Path, &a.Color, &a.IconV)
				a.Icon = a.IconV > 0
				d.AppMap[a.ID] = a
			}
			rows.Close()
		}
	}
	if len(titles) > 0 {
		rows, err := db.sql.Query("SELECT id, text FROM title WHERE id IN (" + idList(titles) + ")")
		if err == nil {
			for rows.Next() {
				var id int64
				var t string
				rows.Scan(&id, &t)
				d.TitMap[id] = t
			}
			rows.Close()
		}
	}
	if len(docs) > 0 {
		rows, err := db.sql.Query("SELECT id, kind, key, name, grp FROM doc WHERE id IN (" + idList(docs) + ")")
		if err == nil {
			for rows.Next() {
				var x DocInfo
				rows.Scan(&x.ID, &x.Kind, &x.Key, &x.Name, &x.Grp)
				d.DocMap[x.ID] = x
			}
			rows.Close()
		}
	}
	for k, v := range db.TagMetas() {
		d.TagMap[k] = v
	}
	d.SiteColors = map[string]string{}
	colors := db.SiteColors()
	for _, x := range d.DocMap {
		if x.Kind != KindWeb {
			continue
		}
		for _, k := range []string{x.Grp, BaseDomain(x.Grp)} {
			if c, ok := colors[strings.ToLower(k)]; ok {
				d.SiteColors[k] = c
			}
		}
	}
}

// TagsIn returns tag segments overlapping [a, b).
func (db *DB) TagsIn(a, b int64) []Tag {
	out := []Tag{}
	rows, err := db.sql.Query("SELECT id, s, e, tags, notes, billable FROM tag_seg WHERE s >= ? AND s < ? AND e > ? ORDER BY s", db.lowS(TTag, a), b, a)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var t Tag
		rows.Scan(&t.ID, &t.S, &t.E, &t.Tags, &t.Notes, &t.Billable)
		out = append(out, t)
	}
	return out
}

// TagMetas returns colour/billable settings for all tags that have them.
func (db *DB) TagMetas() map[string]TagMeta {
	out := map[string]TagMeta{}
	rows, err := db.sql.Query("SELECT tag, color, billable FROM tag_meta")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var m TagMeta
		rows.Scan(&k, &m.Color, &m.Billable)
		out[k] = m
	}
	return out
}

// dayStarts returns the local midnight boundaries covering [a, b).
func dayStarts(a, b int64) []int64 {
	t := time.Unix(a, 0).In(time.Local)
	t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	var out []int64
	for t.Unix() < b {
		out = append(out, t.Unix())
		t = t.AddDate(0, 0, 1)
	}
	out = append(out, t.Unix())
	return out
}

// splitDays calls fn for each local-day piece of [s, e) clipped to [a, b).
func splitDays(bounds []int64, s, e int64, fn func(day int, s, e int64)) {
	i := sort.Search(len(bounds), func(i int) bool { return bounds[i] > s }) - 1
	if i < 0 {
		i = 0
	}
	for ; i < len(bounds)-1 && s < e; i++ {
		ds, de := bounds[i], bounds[i+1]
		ps, pe := s, e
		if ps < ds {
			ps = ds
		}
		if pe > de {
			pe = de
		}
		if pe > ps {
			fn(i, ps, pe)
		}
		if e <= de {
			break
		}
	}
}
