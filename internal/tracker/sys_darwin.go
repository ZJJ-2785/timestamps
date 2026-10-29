package tracker

import (
	"image"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"timestamps/internal/mac"
)

// The macOS side of the tracker. There are no window handles: the
// "window" passed around is the identity of the frontmost app's focused
// window, and what was seen of it is kept here.

var (
	frontMu sync.Mutex
	fronts  = map[uintptr]mac.Front{}
)

func foregroundWindow() uintptr {
	f, ok := mac.FrontApp()
	if !ok || f.PID <= 0 {
		return 0
	}
	h := uintptr(f.Window)
	frontMu.Lock()
	if len(fronts) > 16 {
		fronts = map[uintptr]mac.Front{}
	}
	fronts[h] = f
	frontMu.Unlock()
	return h
}

func front(h uintptr) (mac.Front, bool) {
	frontMu.Lock()
	defer frontMu.Unlock()
	f, ok := fronts[h]
	return f, ok
}

// The frontmost app is always on screen.
func windowVisible(h uintptr) bool { return true }

func windowTitle(h uintptr) string {
	f, _ := front(h)
	return f.Title
}

func idleMillis() uint32 {
	s := mac.IdleSeconds()
	if s < 0 {
		return 0
	}
	if s > 4e6 {
		s = 4e6
	}
	return uint32(s * 1000)
}

func screenLocked() bool { return mac.ScreenLocked() }

func screenAwake() bool { return mac.DisplayAwake() }

var genericNames = map[string]bool{"electron": true, "java": true, "python": true, "python3": true, "node": true, "wine": true, "wine64-preloader": true}

func isGenericAppName(name string) bool {
	return genericNames[strings.ToLower(strings.TrimSpace(name))]
}

// appKey names an app the way exclusions and the browser list do: the
// bundle's file name in lower case ("google chrome"), as executable names
// can be shared (Visual Studio Code's is "Electron").
func appKey(f mac.Front) string {
	base := filepath.Base(f.Bundle)
	if strings.HasSuffix(strings.ToLower(base), ".app") {
		base = base[:len(base)-4]
	}
	if base == "." || base == "/" {
		return ""
	}
	return strings.ToLower(base)
}

func (t *Tracker) procInfo(h uintptr) (uint32, proc) {
	f, _ := front(h)
	pid := uint32(f.PID)
	if p, ok := t.procs[pid]; ok && time.Since(p.at) < 30*time.Second {
		return pid, p
	}
	p := proc{path: f.Bundle, exe: appKey(f), name: f.Name, at: time.Now()}
	if p.name == "" || isGenericAppName(p.name) {
		p.name = strings.TrimSuffix(filepath.Base(f.Bundle), ".app")
	}
	if len(t.procs) > 512 {
		t.procs = map[uint32]proc{}
	}
	t.procs[pid] = p
	return pid, p
}

// appIcon renders the app's icon as the Finder shows it.
func appIcon(path string, framework bool, h uintptr) []byte {
	return mac.AppIconPNG(path, 64)
}

func (t *Tracker) audioLoop() {
	tk := time.NewTicker(time.Second)
	defer tk.Stop()
	for {
		select {
		case <-t.stop:
			return
		case <-tk.C:
		}
		if !t.settings.Load().TreatSoundAsActive || idleMillis() < 15000 {
			continue
		}
		if mac.SoundPlaying() {
			t.lastSound.Store(time.Now().UnixMilli())
		}
	}
}

// grabScreen captures the display showing the window fg, or all displays.
func grabScreen(fg uintptr, all bool) (*image.RGBA, error) {
	f, ok := front(fg)
	ok = ok && f.W > 0
	return mac.Capture(f.X+f.W/2, f.Y+f.H/2, ok, all)
}
