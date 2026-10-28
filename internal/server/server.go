// Package server exposes the tracker and database over a local HTTP API
// and serves the web UI.
package server

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"timestamps/internal/favicon"
	"timestamps/internal/store"
	"timestamps/internal/tracker"
	"timestamps/internal/xlsx"
)

// Version is the application version shown in the UI.
const Version = "1.2.0"

// Ports tried in order; the browser extension probes the same list.
var Ports = []int{47613, 47614, 47615, 47616}

// Host is implemented by the desktop shell.
type Host interface {
	Show(route string)
	Notify(title, text string)
	PickFile(title, filter string) string
	PickFolder(title, dir string) string
	Quit()
	Open(target string)
	Reveal(path string)
	ApplySettings(old, cur store.Settings)
	Text(key string) string
}

// Server is the HTTP API.
type Server struct {
	DB      *store.DB
	T       *tracker.Tracker
	Fav     *favicon.Finder
	Host    Host
	Web     fs.FS
	DataDir string
	ExtDir  string
	Port    int
	token   string
	timers  *Timers

	upd    updates
	impMu  sync.Mutex
	imp    map[string]any
	ln     net.Listener
	server *http.Server
}

// Listen binds the first free port from Ports.
func (s *Server) Listen() error {
	var err error
	for _, p := range Ports {
		s.ln, err = net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p))
		if err == nil {
			s.Port = p
			break
		}
	}
	if s.ln == nil {
		return fmt.Errorf("no free port: %w", err)
	}
	b := make([]byte, 16)
	rand.Read(b)
	s.token = hex.EncodeToString(b)
	s.imp = map[string]any{"running": false}
	s.timers = newTimers(s.DB, s.notifyTimer, s.T.Emit, func() int { return s.T.Settings().StopwatchNag })
	go s.backupLoop()
	return nil
}

// backup writes a timestamped copy of the database into DataDir\backups.
func (s *Server) backup() (string, error) {
	dir := filepath.Join(s.DataDir, "backups")
	os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, "timestamps-"+time.Now().Format("2006-01-02_150405")+".db")
	if err := s.DB.Backup(p); err != nil {
		return "", err
	}
	s.DB.SetMeta("backup.last", strconv.FormatInt(time.Now().Unix(), 10))
	return p, nil
}

// backupLoop makes scheduled backups and keeps the newest BackupKeep.
func (s *Server) backupLoop() {
	time.Sleep(2 * time.Minute)
	for {
		st := s.T.Settings()
		last, _ := strconv.ParseInt(s.DB.Meta("backup.last"), 10, 64)
		if st.BackupDays > 0 && time.Since(time.Unix(last, 0)) >= time.Duration(st.BackupDays)*24*time.Hour {
			if _, err := s.backup(); err != nil {
				log.Println("backup:", err)
			} else if st.BackupKeep > 0 {
				files, _ := filepath.Glob(filepath.Join(s.DataDir, "backups", "timestamps-*.db"))
				sort.Strings(files)
				for len(files) > st.BackupKeep {
					os.Remove(files[0])
					files = files[1:]
				}
			}
		}
		time.Sleep(time.Hour)
	}
}

// URL is the address of the UI.
func (s *Server) URL() string { return "http://127.0.0.1:" + strconv.Itoa(s.Port) + "/" }

// Timers exposes the stopwatch/countdown (used by the tray menu).
func (s *Server) Timers() *Timers { return s.timers }

// Serve runs the HTTP server until Close.
func (s *Server) Serve() error {
	s.server = &http.Server{Handler: s, ReadHeaderTimeout: 10 * time.Second}
	err := s.server.Serve(s.ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Close stops the server.
func (s *Server) Close() {
	if s.server != nil {
		s.server.Close()
	}
}

func (s *Server) notifyTimer(kind string, secs int64, label string) {
	dur := fmt.Sprintf("%dh %02dm", secs/3600, secs/60%60)
	if kind == "countdown" {
		text := s.Host.Text("countdownDone") + " (" + dur + ")"
		if label != "" {
			text = label + " — " + text
		}
		s.Host.Notify("Timestamps", text)
	} else {
		s.Host.Notify("Timestamps", s.Host.Text("stopwatchRunning")+" "+dur+" "+label)
	}
}

func (s *Server) hostOK(r *http.Request) bool {
	h := r.Host
	p := strconv.Itoa(s.Port)
	return h == "127.0.0.1:"+p || h == "localhost:"+p
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.hostOK(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/ext/"):
		if r.Header.Get("X-Timestamps-Ext") == "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		s.ext(w, r)
	case p == "/api/internal/show" || p == "/api/internal/quit":
		// From another copy of the program (a second start, or the setup).
		if r.Method != http.MethodPost || r.Header.Get("X-Timestamps-Instance") == "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if p == "/api/internal/quit" {
			s.Host.Quit()
		} else {
			s.Host.Show(r.URL.Query().Get("route"))
		}
		w.WriteHeader(http.StatusNoContent)
	case strings.HasPrefix(p, "/api/"):
		c, err := r.Cookie("ts_token")
		if err != nil || c.Value != s.token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodPost && r.Header.Get("X-Requested-With") != "timestamps" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		s.api(w, r, strings.TrimPrefix(p, "/api/"))
	default:
		s.static(w, r)
	}
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || name == "." {
		name = "index.html"
	}
	data, err := fs.ReadFile(s.Web, name)
	if err != nil {
		data, err = fs.ReadFile(s.Web, "index.html")
		name = "index.html"
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	if name == "index.html" {
		http.SetCookie(w, &http.Cookie{Name: "ts_token", Value: s.token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	}
	ct := mime.TypeByExtension(filepath.Ext(name))
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	writeMaybeGzip(w, r, data)
}

func writeMaybeGzip(w http.ResponseWriter, r *http.Request, data []byte) {
	if len(data) > 2048 && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		gz, _ := gzip.NewWriterLevel(w, gzip.BestSpeed)
		gz.Write(data)
		gz.Close()
		return
	}
	w.Write(data)
}

func writeJSON(w http.ResponseWriter, r *http.Request, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	writeMaybeGzip(w, r, b)
}

func fail(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 32<<20)).Decode(v)
}

func qInt(r *http.Request, k string) int64 {
	v, _ := strconv.ParseInt(r.URL.Query().Get(k), 10, 64)
	return v
}

// qRange parses from/to, clamping open-ended ranges to the first recorded day.
func (s *Server) qRange(r *http.Request) (int64, int64, error) {
	a, b, err := qRangeRaw(r)
	if err != nil {
		return a, b, err
	}
	if first := s.DB.Earliest(); first > a && first < b {
		a = first
	}
	return a, b, nil
}

func qRangeRaw(r *http.Request) (int64, int64, error) {
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	if from == "" {
		from = time.Now().Format("2006-01-02")
	}
	if to == "" {
		to = from
	}
	return store.Range(from, to)
}

type dayResp struct {
	*store.Day
	Auto    []store.Tag    `json:"auto"`
	Live    *tracker.Live  `json:"live,omitempty"`
	Version int64          `json:"version"`
	Aways   []tracker.Away `json:"aways"`
}

func (s *Server) api(w http.ResponseWriter, r *http.Request, route string) {
	switch route {
	case "live":
		l := s.T.Live()
		if l.AppID != 0 {
			l.IconV = s.DB.AppIconV(l.AppID)
		}
		writeJSON(w, r, map[string]any{"live": l, "timers": s.timers.State(), "aways": s.T.Aways(), "version": s.DB.Version()})

	case "day":
		date := r.URL.Query().Get("date")
		if date == "" {
			date = time.Now().Format("2006-01-02")
		}
		a, b, err := store.DayRange(date)
		if err != nil {
			fail(w, err)
			return
		}
		etag := fmt.Sprintf(`"%s-%d"`, date, s.DB.Version())
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		d, err := s.DB.Day(a, b)
		if err != nil {
			fail(w, err)
			return
		}
		resp := dayResp{Day: d, Version: s.DB.Version(), Aways: []tracker.Away{}}
		if rules := s.DB.Rules(); len(rules) > 0 {
			resp.Auto, _ = s.DB.AutoTags(a, b, rules)
		}
		now := time.Now().Unix()
		if now >= a && now < b {
			l := s.T.Live()
			resp.Live = &l
			resp.Aways = s.T.Aways()
		}
		w.Header().Set("ETag", etag)
		writeJSON(w, r, resp)

	case "stats":
		a, b, err := s.qRange(r)
		if err != nil {
			fail(w, err)
			return
		}
		st, err := s.DB.Stats(a, b, s.T.Settings().MergeSubdomains)
		if err != nil {
			fail(w, err)
			return
		}
		// The period to compare with, usually the one before.
		if q := r.URL.Query(); q.Get("pfrom") != "" {
			if pa, pb, err := store.Range(q.Get("pfrom"), q.Get("pto")); err == nil {
				if p, err := s.DB.Totals(pa, pb); err == nil {
					st.Prev = &p
				}
			}
		}
		writeJSON(w, r, st)

	case "detail":
		a, b, err := s.qRange(r)
		if err != nil {
			fail(w, err)
			return
		}
		d, err := s.DB.Detail(r.URL.Query().Get("kind"), r.URL.Query().Get("key"), qInt(r, "id"), a, b, s.T.Settings().MergeSubdomains)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, r, d)

	case "timesheet":
		a, b, err := s.qRange(r)
		if err != nil {
			fail(w, err)
			return
		}
		q := r.URL.Query()
		o := store.TimesheetOpts{Level: int(qInt(r, "level")), Round: qInt(r, "round"), RoundMode: q.Get("mode"),
			NeverZero: q.Get("neverZero") == "1", Billable: q.Get("billable") == "1"}
		writeJSON(w, r, s.DB.Timesheet(a, b, o))

	case "search":
		a, b, err := s.qRange(r)
		if err != nil {
			fail(w, err)
			return
		}
		res, err := s.DB.Search(r.URL.Query().Get("q"), a, b)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, r, res)

	case "tags/all":
		writeJSON(w, r, map[string]any{"tags": s.DB.AllTags(), "names": s.DB.TagNames(), "recent": s.DB.RecentTags(s.T.Settings().RecentTags)})

	case "tags/add":
		var t store.Tag
		if err := readJSON(r, &t); err != nil {
			fail(w, err)
			return
		}
		id, err := s.DB.AddTag(t.S, t.E, t.Tags, t.Notes, t.Billable)
		if err != nil {
			fail(w, err)
			return
		}
		s.T.Emit("tags")
		writeJSON(w, r, map[string]any{"id": id})

	case "tags/addMany":
		var req struct {
			Ranges   [][2]int64
			Tags     string
			Notes    string
			Billable bool
		}
		if err := readJSON(r, &req); err != nil {
			fail(w, err)
			return
		}
		for _, rg := range req.Ranges {
			if _, err := s.DB.AddTag(rg[0], rg[1], req.Tags, req.Notes, req.Billable); err != nil {
				fail(w, err)
				return
			}
		}
		s.T.Emit("tags")
		writeJSON(w, r, map[string]any{"count": len(req.Ranges)})

	case "tags/update":
		var t store.Tag
		if err := readJSON(r, &t); err != nil {
			fail(w, err)
			return
		}
		if err := s.DB.UpdateTag(t.ID, t.S, t.E, t.Tags, t.Notes, t.Billable); err != nil {
			fail(w, err)
			return
		}
		s.T.Emit("tags")
		writeJSON(w, r, map[string]any{"ok": true})

	case "tags/delete":
		var req struct{ IDs []int64 }
		if err := readJSON(r, &req); err != nil {
			fail(w, err)
			return
		}
		s.DB.DeleteTags(req.IDs)
		s.T.Emit("tags")
		writeJSON(w, r, map[string]any{"ok": true})

	case "tags/clear":
		var req struct{ S, E int64 }
		if err := readJSON(r, &req); err != nil {
			fail(w, err)
			return
		}
		s.DB.ClearTags(req.S, req.E)
		s.T.Emit("tags")
		writeJSON(w, r, map[string]any{"ok": true})

	case "tags/split":
		var req struct{ ID, At int64 }
		if err := readJSON(r, &req); err != nil {
			fail(w, err)
			return
		}
		s.DB.SplitTag(req.ID, req.At)
		s.T.Emit("tags")
		writeJSON(w, r, map[string]any{"ok": true})

	case "tags/rename":
		var req struct{ From, To string }
		if err := readJSON(r, &req); err != nil {
			fail(w, err)
			return
		}
		if err := s.DB.RenameTag(req.From, req.To); err != nil {
			fail(w, err)
			return
		}
		s.T.Emit("tags")
		writeJSON(w, r, map[string]any{"ok": true})

	case "tags/purge":
		var req struct{ Tags string }
		if err := readJSON(r, &req); err != nil {
			fail(w, err)
			return
		}
		s.DB.DeleteTagEverywhere(req.Tags)
		s.T.Emit("tags")
		writeJSON(w, r, map[string]any{"ok": true})

	case "tags/meta":
		var req struct {
			Tag, Color string
			Billable   bool
		}
		if err := readJSON(r, &req); err != nil {
			fail(w, err)
			return
		}
		s.DB.SetTagMeta(req.Tag, req.Color, req.Billable)
		s.T.Emit("tags")
		writeJSON(w, r, map[string]any{"ok": true})

	case "rules":
		if r.Method == http.MethodPost {
			var rules []store.Rule
			if err := readJSON(r, &rules); err != nil {
				fail(w, err)
				return
			}
			s.DB.SetRules(rules)
			s.T.Emit("tags")
		}
		writeJSON(w, r, s.DB.Rules())

	case "autotag/apply":
		var req struct {
			From, To  string
			Overwrite bool
		}
		if err := readJSON(r, &req); err != nil {
			fail(w, err)
			return
		}
		a, b, err := store.Range(req.From, req.To)
		if err != nil {
			fail(w, err)
			return
		}
		n, err := s.DB.ApplyAutoTags(a, b, req.Overwrite)
		if err != nil {
			fail(w, err)
			return
		}
		s.T.Emit("tags")
		writeJSON(w, r, map[string]any{"count": n})

	case "autotag/preview":
		var req struct {
			From, To string
			Rules    []store.Rule
		}
		if err := readJSON(r, &req); err != nil {
			fail(w, err)
			return
		}
		a, b, err := store.Range(req.From, req.To)
		if err != nil {
			fail(w, err)
			return
		}
		tags, err := s.DB.AutoTags(a, b, req.Rules)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, r, tags)

	case "settings":
		if r.Method == http.MethodPost {
			cur := s.T.Settings()
			next := cur
			if err := readJSON(r, &next); err != nil {
				fail(w, err)
				return
			}
			next.ShotsDir = cur.ShotsDir // changed with shots/folder, which moves the files
			if err := s.DB.SaveSettings(next); err != nil {
				fail(w, err)
				return
			}
			next = s.DB.Settings()
			s.T.SetSettings(next)
			s.Host.ApplySettings(cur, next)
			s.T.Emit("settings")
		}
		writeJSON(w, r, s.T.Settings())

	case "apps":
		writeJSON(w, r, s.DB.Apps())

	case "apps/color":
		var req struct {
			ID    int64
			Color string
		}
		if err := readJSON(r, &req); err != nil {
			fail(w, err)
			return
		}
		s.DB.SetAppColor(req.ID, req.Color)
		writeJSON(w, r, map[string]any{"ok": true})

	case "icon":
		b := s.DB.AppIcon(qInt(r, "id"))
		if b == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		if r.URL.Query().Get("v") != "" {
			w.Header().Set("Cache-Control", "max-age=31536000, immutable") // URL changes with the icon
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		w.Write(b)

	case "siteicon":
		var b []byte
		if s.Fav != nil {
			b = s.Fav.Icon(r.URL.Query().Get("d"))
		}
		if b == nil {
			w.Header().Set("Cache-Control", "max-age=3600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", http.DetectContentType(b))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "max-age=604800")
		w.Write(b)

	case "shot":
		p := s.T.Shots.ShotPath(qInt(r, "ts"), r.URL.Query().Get("thumb") == "1")
		w.Header().Set("Cache-Control", "max-age=31536000, immutable")
		w.Header().Set("Content-Type", "image/jpeg")
		http.ServeFile(w, r, p)

	case "shot/now":
		go s.T.Shots.Capture(s.T.Settings())
		writeJSON(w, r, map[string]any{"ok": true})

	case "shots":
		a, b, err := s.qRange(r)
		if err != nil {
			fail(w, err)
			return
		}
		d, err := s.DB.Day(a, b)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, r, d.Shots)

	case "export":
		a, b, err := s.qRange(r)
		if err != nil {
			fail(w, err)
			return
		}
		tl := r.URL.Query().Get("timeline")
		rows, err := s.DB.ExportRows(tl, a, b)
		if err != nil {
			fail(w, err)
			return
		}
		q := r.URL.Query()
		writeTable(w, fmt.Sprintf("timestamps-%s-%s_%s", tl, q.Get("from"), q.Get("to")), r.URL.Query().Get("format"), rows)

	case "export/table":
		var req struct {
			Name   string
			Format string
			Rows   [][]string
		}
		if err := readJSON(r, &req); err != nil {
			fail(w, err)
			return
		}
		writeTable(w, req.Name, req.Format, req.Rows)

	case "aways/resolve":
		var req struct {
			S, E     int64
			Tags     string
			Notes    string
			Billable bool
		}
		if err := readJSON(r, &req); err != nil {
			fail(w, err)
			return
		}
		if req.Tags != "" {
			s.DB.AddTag(req.S, req.E, req.Tags, req.Notes, req.Billable)
			s.T.Emit("tags")
		}
		s.T.ResolveAway(req.S)
		writeJSON(w, r, map[string]any{"ok": true})

	case "pause":
		var req struct{ Minutes int }
		readJSON(r, &req)
		s.T.Pause(time.Duration(req.Minutes) * time.Minute)
		writeJSON(w, r, s.T.Live())

	case "resume":
		s.T.Resume()
		writeJSON(w, r, s.T.Live())

	case "stopwatch/start":
		var req struct {
			Tags, Notes string
			Start       int64
		}
		readJSON(r, &req)
		s.timers.StartStopwatch(req.Tags, req.Notes, req.Start)
		writeJSON(w, r, s.timers.State())

	case "stopwatch/update":
		var req struct {
			Tags, Notes string
			Start       int64
		}
		readJSON(r, &req)
		s.timers.UpdateStopwatch(req.Tags, req.Notes, req.Start)
		writeJSON(w, r, s.timers.State())

	case "stopwatch/stop":
		var req struct{ Discard bool }
		readJSON(r, &req)
		s.timers.StopStopwatch(!req.Discard)
		s.T.Emit("tags")
		writeJSON(w, r, s.timers.State())

	case "countdown/start":
		var req struct {
			Secs        int64
			Tags, Label string
		}
		readJSON(r, &req)
		if req.Secs > 0 {
			s.timers.StartCountdown(req.Secs, req.Tags, req.Label)
		}
		writeJSON(w, r, s.timers.State())

	case "countdown/cancel":
		s.timers.CancelCountdown()
		writeJSON(w, r, s.timers.State())

	case "events":
		s.events(w, r)

	case "info":
		mt := filepath.Join(os.Getenv("LOCALAPPDATA"), `Finkit\ManicTime\ManicTimeReports.db`)
		if _, err := os.Stat(mt); err != nil {
			mt = ""
		}
		var size int64
		if st, err := os.Stat(filepath.Join(s.DataDir, "timestamps.db")); err == nil {
			size = st.Size()
		}
		if st, err := os.Stat(filepath.Join(s.DataDir, "timestamps.db-wal")); err == nil {
			size += st.Size()
		}
		writeJSON(w, r, map[string]any{
			"version": Version, "dataDir": s.DataDir, "extDir": s.ExtDir, "dbSize": size, "db": s.DB.Info(),
			"manictime": mt, "ext": s.T.Ext.Connected(), "url": s.URL(), "shotsDir": s.T.Shots.Dir(), "shotsSize": dirSize(s.T.Shots.Dir()), "thumbsSize": dirSize(s.T.Shots.ThumbDir()),
			"shotsDefault": s.defaultShotsDir(),
			"lastBackup":   s.DB.Meta("backup.last"),
		})

	case "import/manictime":
		var req struct{ Path string }
		if err := readJSON(r, &req); err != nil || req.Path == "" {
			fail(w, errors.New("missing path"))
			return
		}
		s.startImport(req.Path)
		writeJSON(w, r, s.importState())

	case "import/status":
		writeJSON(w, r, s.importState())

	case "data/delete":
		var req struct {
			From, To  string
			Timelines []string
		}
		if err := readJSON(r, &req); err != nil {
			fail(w, err)
			return
		}
		a, b, err := store.Range(req.From, req.To)
		if err != nil {
			fail(w, err)
			return
		}
		var shots []int64
		for _, tl := range req.Timelines {
			if tl == "shots" {
				shots = s.DB.ShotsBetween(a, b)
			}
		}
		if err := s.DB.DeleteRange(a, b, req.Timelines); err != nil {
			fail(w, err)
			return
		}
		s.T.Shots.RemoveFiles(shots)
		s.T.Emit("tags")
		writeJSON(w, r, map[string]any{"ok": true})

	case "backup":
		p, err := s.backup()
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, r, map[string]any{"path": p})

	case "open":
		var req struct{ Target string }
		if err := readJSON(r, &req); err != nil {
			fail(w, err)
			return
		}
		t := req.Target
		low := strings.ToLower(t)
		if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
			s.Host.Open(t)
		} else if _, err := os.Stat(t); err == nil && filepath.IsAbs(t) {
			// Never launch programs or scripts from here, only documents and folders.
			if blockedExt[strings.ToLower(filepath.Ext(t))] || runnable(t) {
				s.Host.Reveal(t)
			} else {
				s.Host.Open(t)
			}
		} else {
			fail(w, errors.New("not found"))
			return
		}
		writeJSON(w, r, map[string]any{"ok": true})

	case "reveal":
		var req struct{ Path string }
		if err := readJSON(r, &req); err != nil {
			fail(w, err)
			return
		}
		if _, err := os.Stat(req.Path); err != nil || !filepath.IsAbs(req.Path) {
			fail(w, errors.New("not found"))
			return
		}
		s.Host.Reveal(req.Path)
		writeJSON(w, r, map[string]any{"ok": true})

	case "pick-file":
		var req struct{ Title, Filter string }
		readJSON(r, &req)
		writeJSON(w, r, map[string]any{"path": s.Host.PickFile(req.Title, req.Filter)})

	case "update":
		writeJSON(w, r, s.CheckUpdate(r.URL.Query().Get("force") == "1"))

	case "update/install":
		if err := s.installUpdate(); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, r, map[string]any{"ok": true})

	case "pick-folder":
		var req struct{ Title, Dir string }
		readJSON(r, &req)
		writeJSON(w, r, map[string]any{"path": s.Host.PickFolder(req.Title, req.Dir)})

	case "shots/folder":
		// Moves the screenshots to another folder ("" = the default one).
		var req struct{ Dir string }
		if err := readJSON(r, &req); err != nil {
			fail(w, err)
			return
		}
		dir := req.Dir
		if dir == "" {
			dir = s.defaultShotsDir()
		}
		if err := checkShotsDir(dir, s.T.Shots.Dir()); err != nil {
			fail(w, err)
			return
		}
		dir = filepath.Clean(dir)
		st := s.T.Settings()
		st.ShotsDir = dir
		if strings.EqualFold(dir, s.defaultShotsDir()) {
			st.ShotsDir = ""
		}
		if err := s.DB.SaveSettings(st); err != nil {
			fail(w, err)
			return
		}
		s.T.SetSettings(s.DB.Settings())
		err := s.T.Shots.Move(dir)
		s.T.Emit("settings")
		if err != nil {
			fail(w, fmt.Errorf("some screenshots could not be moved: %w", err))
			return
		}
		writeJSON(w, r, map[string]any{"dir": dir})

	default:
		http.NotFound(w, r)
	}
}

var blockedExt = map[string]bool{}

func init() {
	for _, e := range strings.Fields(".exe .com .bat .cmd .ps1 .psm1 .vbs .vbe .js .jse .wsf .wsh .msi .msp .scr .hta .lnk .url .cpl .pif .reg .jar .appref-ms .application .msc .inf .sys .dll") {
		blockedExt[e] = true
	}
	if runtime.GOOS == "darwin" {
		for _, e := range strings.Fields(".app .command .tool .sh .zsh .bash .csh .py .rb .pl .pkg .mpkg .scpt .scptd .applescript .workflow .action .terminal .dylib .kext .prefpane .osax .webloc .inetloc .fileloc .dmg") {
			blockedExt[e] = true
		}
	}
}

// runnable reports whether a file is an executable that opening would run
// (macOS opens those in Terminal). Windows has no execute bits.
func runnable(p string) bool {
	if runtime.GOOS == "windows" {
		return false
	}
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0
}

func (s *Server) defaultShotsDir() string { return filepath.Join(s.DataDir, "screenshots") }

// checkShotsDir accepts a folder for screenshots: an absolute path that is
// not the current folder, not inside it or around it, and writable.
func checkShotsDir(dir, cur string) error {
	if !filepath.IsAbs(dir) || (runtime.GOOS == "windows" && filepath.VolumeName(dir) == "") {
		return errors.New("choose a folder on a drive")
	}
	dir, cur = strings.ToLower(filepath.Clean(dir)), strings.ToLower(filepath.Clean(cur))
	sep := string(filepath.Separator)
	switch {
	case dir == cur:
		return errors.New("that is already the screenshot folder")
	case strings.HasPrefix(dir, cur+sep), strings.HasPrefix(cur, strings.TrimSuffix(dir, sep)+sep):
		return errors.New("the new folder can't be inside the current one or contain it")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".timestamps-*")
	if err != nil {
		return fmt.Errorf("can't write to that folder: %w", err)
	}
	f.Close()
	return os.Remove(f.Name())
}

func dirSize(dir string) int64 {
	var n int64
	filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

func writeTable(w http.ResponseWriter, name, format string, rows [][]string) {
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) {
			return '_'
		}
		return r
	}, name)
	if format == "xlsx" {
		var buf bytes.Buffer
		xlsx.Write(&buf, "Timestamps", rows)
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.xlsx"`)
		w.Write(buf.Bytes())
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.csv"`)
	w.Write([]byte("\xEF\xBB\xBF"))
	cw := csv.NewWriter(w)
	cw.WriteAll(rows)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	ch := s.T.Subscribe()
	defer s.T.Unsubscribe(ch)
	hb := time.NewTicker(15 * time.Second)
	defer hb.Stop()
	fmt.Fprintf(w, "retry: 2000\n\n")
	fl.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			b, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		case <-hb.C:
			fmt.Fprintf(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

func (s *Server) importState() map[string]any {
	s.impMu.Lock()
	defer s.impMu.Unlock()
	out := map[string]any{}
	for k, v := range s.imp {
		out[k] = v
	}
	return out
}

func (s *Server) startImport(path string) {
	s.impMu.Lock()
	if s.imp["running"] == true {
		s.impMu.Unlock()
		return
	}
	s.imp = map[string]any{"running": true, "stage": "starting", "done": 0, "total": 0, "path": path}
	s.impMu.Unlock()
	go func() {
		start := time.Now()
		counts, err := s.DB.ImportManicTime(path, func(stage string, done, total int64) {
			s.impMu.Lock()
			s.imp["stage"], s.imp["done"], s.imp["total"] = stage, done, total
			s.impMu.Unlock()
		})
		s.impMu.Lock()
		s.imp["running"] = false
		s.imp["finished"] = true
		s.imp["seconds"] = time.Since(start).Seconds()
		if err != nil {
			s.imp["error"] = err.Error()
		} else {
			s.imp["counts"] = counts
		}
		s.impMu.Unlock()
		s.T.Emit("tags")
	}()
}

func (s *Server) ext(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/ext/hello":
		writeJSON(w, r, map[string]any{"app": "timestamps", "version": Version})
	case "/ext/report":
		var rep tracker.ExtReport
		if err := readJSON(r, &rep); err != nil {
			fail(w, err)
			return
		}
		s.T.Ext.Report(rep)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}
