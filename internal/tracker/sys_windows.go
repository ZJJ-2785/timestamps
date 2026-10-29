//go:build windows

package tracker

import (
	"image"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	ole "github.com/go-ole/go-ole"

	"timestamps/internal/win"
)

// The Windows side of the tracker: the foreground window, idle time,
// process details, app icons, playing sound and screen capture.

func isGenericAppName(name string) bool { return win.IsGenericAppName(name) }
func idleMillis() uint32                { return win.IdleMillis() }
func screenLocked() bool                { return win.IsLocked() }
func foregroundWindow() uintptr         { return win.ForegroundWindow() }
func windowVisible(hwnd uintptr) bool   { return win.IsVisible(hwnd) }
func windowTitle(hwnd uintptr) string   { return win.WindowText(hwnd) }

// screenAwake: Windows reports sleep through Suspend, which needs no wake-up
// check (Tracker.Sleep is for macOS).
func screenAwake() bool { return true }

func (t *Tracker) audioLoop() {
	runtime.LockOSThread()
	ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)
	defer ole.CoUninitialize()
	var m audioMeter
	tk := time.NewTicker(time.Second)
	defer tk.Stop()
	n := 0
	for {
		select {
		case <-t.stop:
			m.reset()
			return
		case <-tk.C:
		}
		if !t.settings.Load().TreatSoundAsActive || win.IdleMillis() < 15000 {
			continue
		}
		n++
		if n%300 == 0 {
			m.reset() // pick up default device changes
		}
		if m.peak() > 0.0005 {
			t.lastSound.Store(time.Now().UnixMilli())
		}
	}
}

func (t *Tracker) procInfo(hwnd uintptr) (uint32, proc) {
	pid := win.WindowPID(hwnd)
	if p, ok := t.procs[pid]; ok && time.Since(p.at) < 30*time.Second {
		return pid, p
	}
	path := win.ProcessPath(pid)
	exe := strings.ToLower(filepath.Base(path))
	if exe == "applicationframehost.exe" {
		if _, cpid := win.ResolveUWP(hwnd, pid); cpid != 0 {
			if cp := win.ProcessPath(cpid); cp != "" {
				path = cp
				exe = strings.ToLower(filepath.Base(cp))
			}
		}
	}
	p := proc{path: path, exe: exe, at: time.Now()}
	if path != "" {
		p.name = win.AppDisplayName(path)
		p.framework = win.IsFrameworkExe(path)
	}
	if len(t.procs) > 512 {
		t.procs = map[uint32]proc{}
	}
	// UWP hosts are resolved per window, so don't cache them by pid.
	if !strings.EqualFold(filepath.Base(win.ProcessPath(pid)), "applicationframehost.exe") {
		t.procs[pid] = p
	}
	return pid, p
}

// appIcon picks the icon for a new app: the window's for framework-based
// apps, otherwise the executable's (falling back to the other one).
func appIcon(path string, framework bool, hwnd uintptr) []byte {
	if framework {
		if ic := win.WindowIconPNG(hwnd); ic != nil {
			return ic
		}
	}
	if path != "" {
		if ic := win.IconPNG(path, 48); ic != nil {
			return ic
		}
	}
	return win.WindowIconPNG(hwnd)
}

// grabScreen captures the monitor showing the window fg, or all monitors.
func grabScreen(fg uintptr, all bool) (*image.RGBA, error) {
	var r win.RECT
	if all {
		x, y := win.SystemMetric(76), win.SystemMetric(77)
		r = win.RECT{Left: int32(x), Top: int32(y), Right: int32(x + win.SystemMetric(78)), Bottom: int32(y + win.SystemMetric(79))}
	} else {
		r = win.MonitorRect(fg)
	}
	return win.CaptureRect(r)
}
