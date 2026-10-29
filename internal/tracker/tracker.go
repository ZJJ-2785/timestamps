// Package tracker records computer usage, applications, documents and
// screenshots, ManicTime style.
package tracker

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"timestamps/internal/store"
)

// Live describes what is being tracked right now.
type Live struct {
	State      string `json:"state"` // active | away | paused | schedule | off
	PausedTill int64  `json:"pausedTill"`
	AppID      int64  `json:"appId"`
	IconV      int64  `json:"iconV,omitempty"` // icon cache key, filled in by the server
	App        string `json:"app"`
	Title      string `json:"title"`
	Doc        string `json:"doc"`
	DocKind    int    `json:"docKind"`
	Since      int64  `json:"since"`
	Idle       int64  `json:"idle"`
	UsageSeg   int64  `json:"usageSeg"`
	AppSeg     int64  `json:"appSeg"`
	DocSeg     int64  `json:"docSeg"`
	Now        int64  `json:"now"`
}

// Away is a finished away period waiting to be tagged.
type Away struct {
	S int64 `json:"s"`
	E int64 `json:"e"`
}

// Event notifies listeners about tracker changes.
type Event struct {
	Type string `json:"type"` // seg | away | shot | state
	At   int64  `json:"at"`
}

type proc struct {
	path, exe, name string
	framework       bool // executable carries a framework's identity and icon
	at              time.Time
}

type seg struct {
	id, s, e int64
}

// Tracker samples the foreground window once a second.
type Tracker struct {
	db       *store.DB
	settings atomic.Pointer[store.Settings]
	docs     *docWorker
	Ext      *ExtHub
	Shots    *Shooter

	lastSound atomic.Int64

	mu         sync.Mutex
	usage      seg
	usageState int // -1 none
	app        seg
	appKey     string
	appTitle   string
	appRef     int64
	appName    string
	doc        seg
	docKey     string
	docName    string
	docKind    int
	lastTick   time.Time
	lastFlush  time.Time
	pausedTill time.Time
	paused     bool
	asleep     time.Time // when the machine went to sleep, until it is seen awake
	state      string
	idle       int64
	aways      []Away
	procs      map[uint32]proc
	iconFixed  map[int64]bool

	lastDocHwnd  uintptr
	lastDocTitle string
	lastDocQuery time.Time

	subMu sync.Mutex
	subs  map[chan Event]bool

	stop chan struct{}
	done chan struct{}
}

// New creates a tracker; call Start to begin sampling.
func New(db *store.DB, shotsDir, thumbsDir string) *Tracker {
	t := &Tracker{
		db: db, docs: newDocWorker(), Ext: NewExtHub(), usageState: -1, procs: map[uint32]proc{}, iconFixed: map[int64]bool{},
		subs: map[chan Event]bool{}, stop: make(chan struct{}), done: make(chan struct{}), state: "off",
	}
	t.Shots = NewShooter(db, shotsDir, thumbsDir)
	t.Shots.OnShot = func(ts int64) { t.emit("shot") }
	s := db.Settings()
	t.settings.Store(&s)
	db.FixGenericNames(isGenericAppName)
	return t
}

// SetSettings swaps in new settings.
func (t *Tracker) SetSettings(s store.Settings) { t.settings.Store(&s) }

// Settings returns the active settings.
func (t *Tracker) Settings() store.Settings { return *t.settings.Load() }

// Subscribe returns a channel of tracker events.
func (t *Tracker) Subscribe() chan Event {
	ch := make(chan Event, 16)
	t.subMu.Lock()
	t.subs[ch] = true
	t.subMu.Unlock()
	return ch
}

// Unsubscribe stops event delivery.
func (t *Tracker) Unsubscribe(ch chan Event) {
	t.subMu.Lock()
	delete(t.subs, ch)
	t.subMu.Unlock()
}

func (t *Tracker) emit(typ string) {
	ev := Event{Type: typ, At: time.Now().Unix()}
	t.subMu.Lock()
	for ch := range t.subs {
		select {
		case ch <- ev:
		default:
		}
	}
	t.subMu.Unlock()
}

// Emit lets other components broadcast through the tracker's event bus.
func (t *Tracker) Emit(typ string) { t.emit(typ) }

// Start launches the sampling loop and the audio monitor.
func (t *Tracker) Start() {
	go t.audioLoop()
	go func() {
		defer close(t.done)
		tk := time.NewTicker(time.Second)
		defer tk.Stop()
		t.tick(time.Now())
		for {
			select {
			case <-t.stop:
				return
			case now := <-tk.C:
				t.tick(now)
			}
		}
	}()
}

// Stop ends tracking and closes all open segments.
func (t *Tracker) Stop() {
	select {
	case <-t.stop:
		return
	default:
	}
	close(t.stop)
	<-t.done
	t.mu.Lock()
	t.closeAll(time.Now().Unix())
	t.mu.Unlock()
}

// Suspend closes segments before the machine sleeps.
func (t *Tracker) Suspend() {
	t.mu.Lock()
	now := time.Now()
	t.closeAll(now.Unix())
	t.lastTick = now
	t.mu.Unlock()
}

// Sleep closes segments as the machine goes to sleep, and records nothing
// more until it is awake with a screen on: macOS samples a few more times
// before it sleeps (with the screen locked, which would start "away"), and
// runs apps now and then while the lid is closed (Power Nap).
func (t *Tracker) Sleep() {
	t.mu.Lock()
	now := time.Now()
	t.closeAll(now.Unix())
	t.lastTick = now
	t.asleep = now.Round(0) // wall clock: the monotonic one stops in sleep
	t.mu.Unlock()
	t.emit("state")
}

// slept reports a gap of more than 30 s between two samples, by the clock
// (Round(0) drops the monotonic reading): macOS's monotonic clock stops while
// the machine sleeps, so a night with the lid closed would look like a second.
func slept(last, now time.Time) bool {
	return !last.IsZero() && now.Round(0).Sub(last.Round(0)) > 30*time.Second
}

// screenOn tells whether a screen is on (replaced in tests).
var screenOn = screenAwake

// sleeping reports whether the machine is still asleep since Sleep: until a
// screen is seen on, and at least 20 s after it went to sleep.
func (t *Tracker) sleeping(now time.Time) bool {
	if t.asleep.IsZero() {
		return false
	}
	if now.Round(0).Sub(t.asleep) >= 20*time.Second && screenOn() {
		t.asleep = time.Time{}
		return false
	}
	return true
}

// Pause stops tracking for d (0 = until resumed).
func (t *Tracker) Pause(d time.Duration) {
	t.mu.Lock()
	t.paused = true
	t.pausedTill = time.Time{}
	if d > 0 {
		t.pausedTill = time.Now().Add(d).Round(0) // by the clock, sleep included
	}
	t.closeAll(time.Now().Unix())
	t.state = "paused"
	t.mu.Unlock()
	t.emit("state")
}

// Resume restarts tracking after Pause.
func (t *Tracker) Resume() {
	t.mu.Lock()
	t.paused = false
	t.mu.Unlock()
	t.emit("state")
}

// Live returns the current tracking state.
func (t *Tracker) Live() Live {
	t.mu.Lock()
	defer t.mu.Unlock()
	l := Live{State: t.state, AppID: t.appRef, App: t.appName, Title: t.appTitle, Doc: t.docName, DocKind: t.docKind,
		Since: t.app.s, Idle: t.idle, UsageSeg: t.usage.id, AppSeg: t.app.id, DocSeg: t.doc.id, Now: time.Now().Unix()}
	if t.paused && !t.pausedTill.IsZero() {
		l.PausedTill = t.pausedTill.Unix()
	}
	if t.state != "active" {
		l.AppID, l.App, l.Title, l.Doc, l.DocKind, l.Since = 0, "", "", "", 0, t.usage.s
	}
	return l
}

// Aways returns away periods that have not been tagged or dismissed.
func (t *Tracker) Aways() []Away {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]Away{}, t.aways...)
}

// ResolveAway removes an away period from the pending list.
func (t *Tracker) ResolveAway(s int64) {
	t.mu.Lock()
	out := t.aways[:0]
	for _, a := range t.aways {
		if a.S != s {
			out = append(out, a)
		}
	}
	t.aways = out
	t.mu.Unlock()
}

func inSchedule(st *store.Settings, now time.Time) bool {
	if !st.ScheduleOn {
		return true
	}
	wd := (int(now.Weekday()) + 6) % 7
	if !st.ScheduleDays[wd] {
		return false
	}
	hm := now.Format("15:04")
	if st.ScheduleStart <= st.ScheduleEnd {
		return hm >= st.ScheduleStart && hm < st.ScheduleEnd
	}
	return hm >= st.ScheduleStart || hm < st.ScheduleEnd
}

func (t *Tracker) exclusion(st *store.Settings, exe string) string {
	for _, x := range st.Exclusions {
		if strings.EqualFold(x.App, exe) {
			return x.Mode
		}
	}
	return ""
}

func (t *Tracker) tick(now time.Time) {
	st := t.settings.Load()
	t.mu.Lock()
	changed := t.step(now, st)
	t.mu.Unlock()
	if changed != "" {
		t.emit(changed)
	}
}

// step runs one sample; it returns the event type to emit, if any.
func (t *Tracker) step(now time.Time, st *store.Settings) string {
	nowS := now.Unix()
	if slept(t.lastTick, now) {
		// The machine slept or the process was frozen: close at the last sample.
		t.closeAll(t.lastTick.Unix())
	}
	t.lastTick = now

	if t.sleeping(now) {
		prev := t.state
		t.closeAll(nowS)
		if prev != t.state {
			return "state"
		}
		return ""
	}

	if t.paused && !t.pausedTill.IsZero() && now.After(t.pausedTill) {
		t.paused = false
	}
	if t.paused || !inSchedule(st, now) {
		prev := t.state
		t.closeAll(nowS)
		t.state = "paused"
		if !t.paused {
			t.state = "schedule"
		}
		if prev != t.state {
			return "state"
		}
		return ""
	}

	idleMs := idleMillis()
	lastInput := now.Add(-time.Duration(idleMs) * time.Millisecond)
	if st.TreatSoundAsActive {
		if ls := time.UnixMilli(t.lastSound.Load()); ls.After(lastInput) {
			lastInput = ls
		}
	}
	t.idle = int64(now.Sub(lastInput) / time.Second)
	locked := screenLocked()
	awayAfter := time.Duration(st.AwayMinutes) * time.Minute
	event := ""

	if locked || now.Sub(lastInput) >= awayAfter {
		switch t.usageState {
		case store.Active:
			cut := lastInput.Unix()
			if cut > nowS {
				cut = nowS
			}
			if cut < t.usage.s {
				cut = t.usage.s
			}
			t.flush(nowS)
			t.db.TruncateFrom(cut)
			t.app, t.doc, t.appKey, t.docKey, t.appTitle = seg{}, seg{}, "", "", ""
			t.usage = seg{id: t.db.BeginUsage(cut, nowS, store.Away), s: cut, e: nowS}
			t.usageState = store.Away
			event = "seg"
		case store.Away:
			t.usage.e = nowS
		default:
			t.usage = seg{id: t.db.BeginUsage(nowS, nowS, store.Away), s: nowS, e: nowS}
			t.usageState = store.Away
			event = "seg"
		}
		t.state = "away"
		t.lastDocHwnd = 0
		if now.Sub(t.lastFlush) >= 10*time.Second {
			t.flush(nowS)
		}
		return event
	}

	// Active.
	switch t.usageState {
	case store.Away:
		awayS := t.usage.s
		t.usage.e = nowS
		t.db.Extend(store.TUsage, t.usage.id, t.usage.s, nowS)
		if st.AwayPrompt && nowS-awayS >= int64(st.AwayPromptMinutes)*60 {
			t.aways = append(t.aways, Away{S: awayS, E: nowS})
			if len(t.aways) > 10 {
				t.aways = t.aways[len(t.aways)-10:]
			}
			defer t.emitAsync("away")
		}
		t.usage = seg{id: t.db.BeginUsage(nowS, nowS, store.Active), s: nowS, e: nowS}
		t.usageState = store.Active
		event = "seg"
	case store.Active:
		t.usage.e = nowS
	default:
		t.usage = seg{id: t.db.BeginUsage(nowS, nowS, store.Active), s: nowS, e: nowS}
		t.usageState = store.Active
		event = "seg"
	}
	t.state = "active"

	hwnd := foregroundWindow()
	// Invisible foreground windows (e.g. hidden helper windows briefly
	// activated by the shell) say nothing about what the user is doing.
	if hwnd != 0 && !windowVisible(hwnd) {
		hwnd = 0
	}
	if hwnd != 0 {
		pid, p := t.procInfo(hwnd)
		excl := t.exclusion(st, p.exe)
		rawTitle := windowTitle(hwnd)
		title := rawTitle
		if !st.TrackTitles || excl == "titles" || excl == "all" {
			title = ""
		}
		if p.exe == "" {
			p.exe, p.name = "unknown", "Unknown"
		}
		if excl == "all" {
			p.exe, p.name, p.path = "private", "Private", ""
		}
		if p.exe != t.appKey || title != t.appTitle {
			t.closeApp(nowS)
			path, framework, fg := p.path, p.framework, hwnd
			ref := t.db.AppID(p.exe, p.name, path, func() []byte { return appIcon(path, framework, fg) })
			// Re-read the icon once per run: apps change icons when they are
			// updated, and older builds may have carried a framework's icon.
			if !t.iconFixed[ref] {
				t.iconFixed[ref] = true
				if ic := appIcon(path, framework, fg); ic != nil {
					t.db.SetAppIcon(ref, ic)
				}
			}
			tid := t.db.TitleID(title)
			t.app = seg{id: t.db.BeginApp(nowS, nowS, ref, tid), s: nowS, e: nowS}
			t.appKey, t.appTitle, t.appRef, t.appName = p.exe, title, ref, p.name
			event = "seg"
		} else {
			t.app.e = nowS
		}

		if st.TrackDocuments && excl != "all" && excl != "titles" {
			browser := browserExes[p.exe]
			need := hwnd != t.lastDocHwnd || rawTitle != t.lastDocTitle || (browser && now.Sub(t.lastDocQuery) >= 3*time.Second)
			if need {
				req := docReq{hwnd: hwnd, pid: pid, exe: p.exe, title: rawTitle, opts: docOpts{addressBar: st.ReadAddressBar, extURL: t.Ext.Match}}
				if d, ok := t.docs.query(req, 900*time.Millisecond); ok {
					t.lastDocHwnd, t.lastDocTitle, t.lastDocQuery = hwnd, rawTitle, now
					if t.setDoc(d, nowS, st) {
						event = "seg"
					}
				}
			} else if t.doc.id != 0 {
				t.doc.e = nowS
			}
		} else if t.doc.id != 0 {
			t.closeDoc(nowS)
			event = "seg"
		}
		t.Shots.maybe(now, st, hwnd, excl != "screenshots" && excl != "all")
	}

	if now.Sub(t.lastFlush) >= 10*time.Second {
		t.flush(nowS)
	}
	return event
}

func (t *Tracker) emitAsync(typ string) { go t.emit(typ) }

func (t *Tracker) setDoc(d Doc, nowS int64, st *store.Settings) bool {
	if d.Kind == 0 {
		if t.doc.id != 0 {
			t.closeDoc(nowS)
			return true
		}
		return false
	}
	if d.Private && !st.TrackPrivate {
		d = Doc{Kind: store.KindWeb, Key: "private://browsing", Name: "Private browsing", Grp: "private"}
	}
	if d.Key == t.docKey {
		t.doc.e = nowS
		return false
	}
	t.closeDoc(nowS)
	ref := t.db.DocID(d.Kind, d.Key, d.Name, d.Grp)
	t.doc = seg{id: t.db.BeginDoc(nowS, nowS, ref, t.appRef), s: nowS, e: nowS}
	t.docKey, t.docName, t.docKind = d.Key, d.Name, d.Kind
	return true
}

func (t *Tracker) closeApp(nowS int64) {
	if t.app.id != 0 {
		t.db.Extend(store.TApp, t.app.id, t.app.s, nowS)
	}
	t.app, t.appKey, t.appTitle = seg{}, "", ""
}

func (t *Tracker) closeDoc(nowS int64) {
	if t.doc.id != 0 {
		t.db.Extend(store.TDoc, t.doc.id, t.doc.s, nowS)
	}
	t.doc, t.docKey, t.docName, t.docKind = seg{}, "", "", 0
}

func (t *Tracker) closeAll(at int64) {
	t.closeApp(at)
	t.closeDoc(at)
	if t.usage.id != 0 {
		t.db.Extend(store.TUsage, t.usage.id, t.usage.s, at)
	}
	t.usage, t.usageState = seg{}, -1
	t.lastDocHwnd = 0
	t.state = "off"
}

func (t *Tracker) flush(nowS int64) {
	t.lastFlush = time.Unix(nowS, 0)
	if t.usage.id != 0 {
		t.db.Extend(store.TUsage, t.usage.id, t.usage.s, nowS)
	}
	if t.app.id != 0 {
		t.db.Extend(store.TApp, t.app.id, t.app.s, nowS)
	}
	if t.doc.id != 0 {
		t.db.Extend(store.TDoc, t.doc.id, t.doc.s, nowS)
	}
}
