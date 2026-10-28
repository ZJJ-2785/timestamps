//go:build windows

package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	ole "github.com/go-ole/go-ole"
	webview2 "github.com/jchv/go-webview2"

	"timestamps/internal/ring"
	"timestamps/internal/store"
	"timestamps/internal/win"
)

type menuItem = win.MenuItem

// native is the Windows shell: notification area icon and WebView2 window.
type native struct {
	shell   *win.Shell
	view    webview2.WebView
	hwnd    uintptr
	opening bool // the window is being created (WebView2 runs messages meanwhile)
	iconBig uintptr
	iconSm  uintptr
	ringKey string  // what the ring icon shows now
	ringSm  uintptr // ring icons (tray and window small, window big), UI thread only
	ringBig uintptr

	balloonRoute string // where clicking the last notification goes ("" = away prompt)
}

// platformText overrides shellText where Windows needs its own words.
var platformText = map[string]map[string]string{}

func systemChinese() bool { return win.UILanguage() == 0x04 }

func systemDark() bool { return win.SystemDarkMode() }

func (a *App) notify(title, text string) { a.shell.Notify(title, text) }
func (a *App) setTip(tip string)         { a.shell.SetTip(tip) }

// notifyRoute shows a notification that opens the window at route when
// clicked ("" = the away prompt).
func (a *App) notifyRoute(title, text, route string) {
	a.shell.Invoke(func() { a.balloonRoute = route })
	a.shell.Notify(title, text)
}

func (a *App) run(showWindow bool) {
	a.iconBig = win.LoadAppIcon(win.SystemMetric(11))
	a.iconSm = win.LoadAppIcon(win.SystemMetric(49))
	a.shell = win.NewShell("Timestamps", a.iconSm)
	a.shell.OnLeftClick = func() { a.openWindow("") }
	a.shell.OnBalloon = func() {
		route := a.balloonRoute
		if route == "" {
			route = "day?away=1"
		}
		a.balloonRoute = ""
		a.openWindow(route)
	}
	a.shell.OnMenu = a.menu
	a.shell.OnCommand = a.command
	a.shell.OnSuspend = a.t.Suspend
	a.shell.OnEndSession = func() {
		a.t.Stop()
		a.db.Close()
	}
	go a.watch()
	go a.tipLoop()
	go a.updateLoop()
	a.ApplySettings(a.t.Settings(), a.t.Settings())
	if showWindow {
		a.shell.Invoke(func() { a.openWindow("") })
	}
	var m win.MSG
	for win.GetMessage(&m) {
		win.TranslateDispatch(&m)
	}
	a.shell.Remove()
}

// Show opens (or focuses) the app window at a route.
func (a *App) Show(route string) {
	a.shell.Invoke(func() { a.openWindow(route) })
}

func (a *App) openWindow(route string) {
	if a.view != nil {
		if route != "" {
			b, _ := json.Marshal(route)
			a.view.Eval("window.__navigate && window.__navigate(" + string(b) + ")")
		}
		win.ShowWindowFront(a.hwnd)
		return
	}
	if a.opening {
		return // a click while the window is starting: it is on its way
	}
	a.opening = true
	// The library shows the window at once, so create it at its final,
	// DPI-scaled size, and have WebView2 start in the page's background
	// color instead of white.
	dpi := win.SystemDPI()
	ww, wh := win.FitScreen(1440*dpi/96, 900*dpi/96)
	os.Setenv("WEBVIEW2_DEFAULT_BACKGROUND_COLOR", "FF"+a.windowBackground())
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath:  filepath.Join(a.dataDir, "webview"),
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title: "Timestamps", Width: uint(ww), Height: uint(wh), IconId: 1, Center: true,
		},
	})
	a.opening = false
	if w == nil {
		// No WebView2 runtime: fall back to the default browser.
		win.ShellOpen(a.srv.URL() + "#/" + route)
		return
	}
	a.view = w
	a.hwnd = uintptr(w.Window())
	w.SetSize(960*dpi/96, 620*dpi/96, webview2.HintMin)
	big, small := a.windowIcons()
	win.SetWindowIcon(a.hwnd, big, small)
	win.SetDarkTitleBar(a.hwnd, a.prefersDark())
	w.Bind("tsSetDark", func(dark bool) { win.SetDarkTitleBar(a.hwnd, dark) })
	w.Bind("tsQuit", func() { a.quit() })
	w.Init("window.__desktop = true;")
	w.Navigate(a.srv.URL() + "#/" + strings.TrimPrefix(route, "/"))
	w.Run() // nested message loop until the window is closed
	a.view, a.hwnd = nil, 0
	if a.exiting {
		win.PostQuit()
		return
	}
	if !a.t.Settings().CloseToTray {
		a.quit()
		return
	}
	if a.db.Meta("ui.trayHint") == "" {
		a.db.SetMeta("ui.trayHint", "1")
		a.shell.Notify("Timestamps", a.Text("stillRunning"))
	}
}

// updateRing redraws the app's icon, a ring that fills once a day, in the
// tray and on the window when it would look different: every 5 minutes, or
// when tracking pauses or resumes.
func (a *App) updateRing(paused bool) {
	key, frac, c := ringNow(paused)
	if key == a.ringKey {
		return
	}
	sm := win.IconFromImage(ring.Draw(win.SystemMetric(49), frac, c))
	big := win.IconFromImage(ring.Draw(win.SystemMetric(11), frac, c))
	if sm == 0 || big == 0 {
		win.DestroyIcon(sm)
		win.DestroyIcon(big)
		return // tried again on the next round
	}
	a.ringKey = key
	a.shell.Invoke(func() {
		oldSm, oldBig := a.ringSm, a.ringBig
		a.ringSm, a.ringBig = sm, big
		a.shell.SetIcon(sm)
		if a.hwnd != 0 {
			win.SetWindowIcon(a.hwnd, big, sm)
		}
		win.DestroyIcon(oldSm)
		win.DestroyIcon(oldBig)
	})
}

// windowIcons are the icons for the app window: the day ring once drawn.
func (a *App) windowIcons() (big, small uintptr) {
	if a.ringBig != 0 {
		return a.ringBig, a.ringSm
	}
	return a.iconBig, a.iconSm
}

func (a *App) quit() {
	a.shell.Invoke(func() {
		a.exiting = true
		if a.hwnd != 0 {
			win.PostMessage(a.hwnd, win.WM_CLOSE, 0, 0)
			return
		}
		win.PostQuit()
	})
}

// Quit exits the program (asked by the setup before it replaces it).
func (a *App) Quit() { a.shell.Invoke(a.quit) }

// Open opens a file, folder or URL with the default handler.
func (a *App) Open(target string) { win.ShellOpen(target) }

// Reveal shows a file in Explorer.
func (a *App) Reveal(path string) { win.RevealInExplorer(path) }

// PickFile shows a native open dialog.
func (a *App) PickFile(title, filter string) string {
	if filter == "" {
		filter = "ManicTime database (ManicTimeReports.db)\x00*.db\x00All files\x00*.*\x00"
	}
	res := make(chan string, 1)
	go func() {
		runtime.LockOSThread()
		ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
		defer ole.CoUninitialize()
		dir := filepath.Join(os.Getenv("LOCALAPPDATA"), `Finkit\ManicTime`)
		res <- win.OpenFileDialog(a.hwnd, title, filter, dir)
	}()
	return <-res
}

// PickFolder shows the native folder picker, starting in dir.
func (a *App) PickFolder(title, dir string) string {
	res := make(chan string, 1)
	go func() {
		runtime.LockOSThread()
		ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
		defer ole.CoUninitialize()
		res <- win.PickFolderDialog(a.hwnd, title, dir)
	}()
	return <-res
}

// ApplySettings reacts to settings changes that touch the OS.
func (a *App) ApplySettings(old, cur store.Settings) {
	exe, _ := os.Executable()
	cmd := `"` + exe + `" --minimized`
	// Also when the program has moved (e.g. installed by the setup).
	if old.RunAtStartup != cur.RunAtStartup || (cur.RunAtStartup && win.AutostartCommand("Timestamps") != cmd) {
		if err := win.SetAutostart("Timestamps", cmd, cur.RunAtStartup); err != nil {
			log.Println("autostart:", err)
		}
	}
	if old.Theme != cur.Theme && a.hwnd != 0 {
		dark := a.prefersDark()
		a.shell.Invoke(func() { win.SetDarkTitleBar(a.hwnd, dark) })
	}
}
