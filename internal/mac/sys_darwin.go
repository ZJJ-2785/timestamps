// Package mac wraps the macOS calls Timestamps needs: the frontmost app and
// window (NSWorkspace, Accessibility), idle time, browser tabs, screen
// capture, and the app shell (menu bar icon, WKWebView window). The
// Objective-C halves are sys_darwin.m and shell_darwin.m.
package mac

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit -framework UserNotifications -framework ApplicationServices -framework CoreGraphics -framework CoreAudio -framework CoreServices -framework ScriptingBridge -framework UniformTypeIdentifiers
#include "mac.h"
*/
import "C"

import (
	"errors"
	"image"
	"strings"
	"sync"
	"unsafe"
)

// Front is the frontmost app and its focused window.
type Front struct {
	PID      int
	Name     string
	Bundle   string // the .app folder, or the executable of a plain process
	BundleID string
	Title    string
	Window   uint64 // identity of the focused window
	// Frame of the window in global screen points (top-left origin); W is
	// 0 when it is not known.
	X, Y, W, H float64
}

// FrontApp returns the frontmost app and its focused window. Titles need
// the Accessibility permission (or Screen Recording, as a fallback).
func FrontApp() (Front, bool) {
	var f C.TSFront
	if !C.tsFront(&f) {
		return Front{}, false
	}
	defer C.tsFreeFront(&f)
	return Front{
		PID: int(f.pid), Name: C.GoString(f.name), Bundle: C.GoString(f.bundle), BundleID: C.GoString(f.bundleID),
		Title: C.GoString(f.title), Window: uint64(f.window), X: float64(f.x), Y: float64(f.y), W: float64(f.w), H: float64(f.h),
	}, true
}

func goString(p *C.char) string {
	if p == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p)
}

// FocusedDocument returns the file shown by an app's focused window.
func FocusedDocument(pid int) string { return goString(C.tsFocusedDocument(C.int(pid))) }

// BundleID returns the bundle identifier of a running app.
func BundleID(pid int) string { return goString(C.tsBundleID(C.int(pid))) }

// Scripting dictionaries of browsers.
const (
	DialectChromium = 1 // Chrome, Edge, Brave, Vivaldi, Arc, Opera
	DialectSafari   = 2
)

// BrowserURL asks a browser for the address of the tab in its front window.
// macOS asks the user once per browser whether Timestamps may do this.
func BrowserURL(pid, dialect int) (url string, incognito bool) {
	var inc C.bool
	url = goString(C.tsBrowserURL(C.int(pid), C.int(dialect), &inc))
	return url, bool(inc)
}

// SpotlightPath returns the most recently used file called name.
func SpotlightPath(name string) string {
	cs := C.CString(name)
	defer C.free(unsafe.Pointer(cs))
	return goString(C.tsSpotlightPath(cs))
}

// AppIconPNG renders the icon of an app (or any file) as a PNG.
func AppIconPNG(path string, size int) []byte {
	if path == "" {
		return nil
	}
	cs := C.CString(path)
	defer C.free(unsafe.Pointer(cs))
	var n C.int
	p := C.tsAppIconPNG(cs, C.int(size), &n)
	if p == nil {
		return nil
	}
	defer C.free(p)
	return C.GoBytes(p, n)
}

// IdleSeconds is the time since the last keyboard or mouse input.
func IdleSeconds() float64 { return float64(C.tsIdleSeconds()) }

// ScreenLocked reports a locked screen or a switched-away user session.
func ScreenLocked() bool { return bool(C.tsScreenLocked()) }

// SoundPlaying reports whether an app plays through the default output.
func SoundPlaying() bool { return bool(C.tsSoundPlaying()) }

// DisplayAwake reports whether any display is on (not asleep).
func DisplayAwake() bool { return bool(C.tsDisplayAwake()) }

// AccessibilityTrusted reports whether Timestamps may read other apps'
// windows (System Settings → Privacy & Security → Accessibility).
func AccessibilityTrusted() bool { return bool(C.tsAccessibilityTrusted(false)) }

// RequestAccessibility shows the system prompt that adds Timestamps to the
// Accessibility list, and opens that list.
func RequestAccessibility() {
	if !bool(C.tsAccessibilityTrusted(true)) {
		openPane("Privacy_Accessibility")
	}
}

// ScreenCaptureAllowed reports whether Timestamps may record the screen.
// macOS only reports a new permission after the app restarts.
func ScreenCaptureAllowed() bool { return bool(C.tsScreenCaptureAllowed()) }

var askScreen sync.Once

// RequestScreenCapture shows the system prompt for Screen Recording (once
// per run) and opens its list in System Settings.
func RequestScreenCapture() {
	C.tsRequestScreenCapture()
	openPane("Privacy_ScreenCapture")
}

func openPane(name string) {
	cs := C.CString(name)
	defer C.free(unsafe.Pointer(cs))
	C.tsOpenPrivacyPane(cs)
}

// ErrScreenRecording means Screen Recording is not allowed.
var ErrScreenRecording = errors.New("screen recording is not allowed (System Settings → Privacy & Security → Screen Recording)")

// Capture returns the display containing the point (x, y) in global screen
// points (the main display when atPoint is false), or all displays.
func Capture(x, y float64, atPoint, all bool) (*image.RGBA, error) {
	if !ScreenCaptureAllowed() {
		askScreen.Do(func() { C.tsRequestScreenCapture() })
		return nil, ErrScreenRecording
	}
	var w, h C.int
	p := C.tsCapture(C.double(x), C.double(y), C.bool(atPoint), C.bool(all), &w, &h)
	if p == nil {
		return nil, errors.New("the screen could not be captured")
	}
	defer C.free(p)
	n := int(w) * int(h) * 4
	return &image.RGBA{Pix: C.GoBytes(p, C.int(n)), Stride: int(w) * 4, Rect: image.Rect(0, 0, int(w), int(h))}, nil
}

// PreferredLanguage is the user's first language, e.g. "zh-Hans-CN".
func PreferredLanguage() string { return goString(C.tsPreferredLanguage()) }

// Translocated reports whether macOS runs the app from a random read-only
// copy because it was opened where it was downloaded.
func Translocated(exe string) bool { return strings.Contains(exe, "/AppTranslocation/") }
