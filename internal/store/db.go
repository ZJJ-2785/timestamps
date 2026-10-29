// Package store keeps all tracked data in a single SQLite database.
package store

import (
	"database/sql"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"sync"
	"sync/atomic"

	_ "github.com/mattn/go-sqlite3"
)

// Timeline tables. Every segment table has the same leading columns
// (id, s, e, ..., src) where s/e are unix seconds and src marks imports.
const (
	TUsage = "usage_seg"
	TApp   = "app_seg"
	TDoc   = "doc_seg"
	TTag   = "tag_seg"
)

// Document kinds.
const (
	KindFile = 1
	KindWeb  = 2
)

// Usage states.
const (
	Away   = 0
	Active = 1
)

const schema = `
CREATE TABLE IF NOT EXISTS meta (k TEXT PRIMARY KEY, v TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS app (
	id INTEGER PRIMARY KEY, key TEXT NOT NULL UNIQUE, name TEXT NOT NULL,
	path TEXT NOT NULL DEFAULT '', color TEXT NOT NULL, icon BLOB);
CREATE TABLE IF NOT EXISTS title (id INTEGER PRIMARY KEY, text TEXT NOT NULL UNIQUE);
CREATE TABLE IF NOT EXISTS doc (
	id INTEGER PRIMARY KEY, kind INTEGER NOT NULL, key TEXT NOT NULL, name TEXT NOT NULL,
	grp TEXT NOT NULL, UNIQUE(kind, key));
CREATE TABLE IF NOT EXISTS usage_seg (id INTEGER PRIMARY KEY, s INTEGER NOT NULL, e INTEGER NOT NULL,
	state INTEGER NOT NULL, src INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS app_seg (id INTEGER PRIMARY KEY, s INTEGER NOT NULL, e INTEGER NOT NULL,
	app INTEGER NOT NULL, title INTEGER NOT NULL, src INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS doc_seg (id INTEGER PRIMARY KEY, s INTEGER NOT NULL, e INTEGER NOT NULL,
	doc INTEGER NOT NULL, app INTEGER NOT NULL DEFAULT 0, src INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS tag_seg (id INTEGER PRIMARY KEY, s INTEGER NOT NULL, e INTEGER NOT NULL,
	tags TEXT NOT NULL, notes TEXT NOT NULL DEFAULT '', billable INTEGER NOT NULL DEFAULT 0,
	src INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS tag_meta (tag TEXT PRIMARY KEY, color TEXT NOT NULL DEFAULT '', billable INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS shot (ts INTEGER PRIMARY KEY, w INTEGER NOT NULL, h INTEGER NOT NULL, bytes INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS site_icon (domain TEXT PRIMARY KEY, icon BLOB, checked INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS ix_usage_s ON usage_seg(s, e, state);
CREATE INDEX IF NOT EXISTS ix_app_s ON app_seg(s, e, app, title);
CREATE INDEX IF NOT EXISTS ix_doc_s ON doc_seg(s, e, doc, app);
CREATE INDEX IF NOT EXISTS ix_tag_s ON tag_seg(s, e);
CREATE INDEX IF NOT EXISTS ix_app_title ON app_seg(title);
`

// DB is the application database.
type DB struct {
	sql *sql.DB

	mu      sync.Mutex
	apps    map[string]int64
	docs    map[string]int64
	titles  map[string]int64
	maxLen  map[string]*int64
	version atomic.Int64 // bumped on every write, used as a cache key by the UI
	// How long a time away lasts at least to count as off, in seconds (0 =
	// never; see usageRows). Kept in step with the settings by SaveSettings.
	longAway atomic.Int64
}

// Open opens (and migrates) the database at path.
func Open(path string) (*DB, error) {
	dsn := "file:" + path + "?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=8000&_foreign_keys=off&_cache_size=-32000"
	s, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	s.SetMaxOpenConns(6)
	if _, err := s.Exec(schema); err != nil {
		s.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	// Columns added after the first release.
	addColumn(s, "app", "color_auto", "INTEGER NOT NULL DEFAULT 1") // 0 once a color was picked by hand
	addColumn(s, "site_icon", "color", "TEXT NOT NULL DEFAULT ''")
	db := &DB{sql: s, apps: map[string]int64{}, docs: map[string]int64{}, titles: map[string]int64{}, maxLen: map[string]*int64{}}
	for _, t := range []string{TUsage, TApp, TDoc, TTag} {
		var m sql.NullInt64
		s.QueryRow("SELECT MAX(e - s) FROM " + t).Scan(&m)
		v := m.Int64
		db.maxLen[t] = &v
	}
	db.longAway.Store(db.Settings().longAway())
	return db, nil
}

func addColumn(s *sql.DB, table, col, def string) {
	rows, err := s.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return
	}
	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk)
		found = found || name == col
	}
	rows.Close()
	if !found {
		s.Exec("ALTER TABLE " + table + " ADD COLUMN " + col + " " + def)
	}
}

// Close closes the database.
func (db *DB) Close() error {
	db.sql.Exec("PRAGMA optimize")
	return db.sql.Close()
}

// SQL exposes the underlying handle for maintenance tasks.
func (db *DB) SQL() *sql.DB { return db.sql }

// Version returns a counter that changes whenever data is written.
func (db *DB) Version() int64 { return db.version.Load() }

func (db *DB) touch() { db.version.Add(1) }

func (db *DB) noteLen(table string, d int64) {
	p := db.maxLen[table]
	for {
		cur := atomic.LoadInt64(p)
		if d <= cur || atomic.CompareAndSwapInt64(p, cur, d) {
			return
		}
	}
}

// lower bound for s when looking for segments overlapping [a, b).
func (db *DB) lowS(table string, a int64) int64 {
	return a - atomic.LoadInt64(db.maxLen[table]) - 1
}

// Meta reads a key from the meta table.
func (db *DB) Meta(k string) string {
	var v string
	db.sql.QueryRow("SELECT v FROM meta WHERE k = ?", k).Scan(&v)
	return v
}

// SetMeta writes a key into the meta table.
func (db *DB) SetMeta(k, v string) error {
	_, err := db.sql.Exec("INSERT INTO meta(k, v) VALUES(?, ?) ON CONFLICT(k) DO UPDATE SET v = excluded.v", k, v)
	return err
}

// ColorFor derives a stable, pleasant colour from a key.
func ColorFor(key string) string {
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(key)))
	v := h.Sum32()
	hue := float64(v%360) / 360
	sat := 0.55 + float64((v>>9)%20)/100
	lig := 0.50 + float64((v>>17)%12)/100
	r, g, b := hsl(hue, sat, lig)
	return fmt.Sprintf("%02X%02X%02X", r, g, b)
}

func hsl(h, s, l float64) (uint8, uint8, uint8) {
	f := func(n float64) uint8 {
		k := math.Mod(n+h*12, 12)
		a := s * math.Min(l, 1-l)
		v := l - a*math.Max(-1, math.Min(math.Min(k-3, 9-k), 1))
		return uint8(math.Round(v * 255))
	}
	return f(0), f(8), f(4)
}
