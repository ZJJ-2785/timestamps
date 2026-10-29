package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"log"
	"os"
	"strings"

	"timestamps/internal/mac"
	"timestamps/internal/ring"
	"timestamps/internal/store"
)

type menuItem = mac.MenuItem

// native is the macOS shell: menu bar icon and WKWebView window.
type native struct {
	shell   *mac.Shell
	ringKey string // what the ring icon shows now
}

// loginLabel names the LaunchAgent that starts Timestamps at login; it is
// also the bundle identifier.
const loginLabel = "io.github.zjj2785.timestamps"

const (
	cmdAllowAccessibility = 100 + iota
	cmdAllowScreen
)

// Labels of the application menu.
var menuLabels = []string{"about", "settings", "hide", "hideOthers", "showAll", "quit", "edit", "undo", "redo", "cut", "copy", "paste",
	"selectAll", "window", "minimize", "zoom", "close", "ok", "cancel"}

// platformText overrides shellText where macOS has its own words.
var platformText = map[string]map[string]string{
	"en": {
		"exit": "Quit Timestamps", "stillRunning": "Timestamps keeps tracking from the menu bar. Choose Quit Timestamps from its menu to stop.",
		"stopwatchStarted":   "Stopwatch started. Stop it from the menu bar or the app.",
		"allowAccessibility": "Allow Accessibility Access…", "allowScreen": "Allow Screen Recording…",
		"axTitle": "Let Timestamps read window titles",
		"axText": "To record window titles and the documents you work on, Timestamps needs Accessibility access. Without it, only app names are recorded.\n\n" +
			"Everything stays on this Mac. You can change this any time in System Settings → Privacy & Security → Accessibility.",
		"axOK": "Open System Settings", "notNow": "Not Now",
		"moveTitle": "Move Timestamps to Applications",
		"moveText":  "macOS runs Timestamps from a temporary copy because it was opened where it was downloaded. Quit it, move it to the Applications folder and open it from there, so it can start at login and keep its permissions.",
		"about":     "About Timestamps", "settings": "Settings…", "hide": "Hide Timestamps", "hideOthers": "Hide Others", "showAll": "Show All",
		"quit": "Quit Timestamps", "edit": "Edit", "undo": "Undo", "redo": "Redo", "cut": "Cut", "copy": "Copy", "paste": "Paste",
		"selectAll": "Select All", "window": "Window", "minimize": "Minimize", "zoom": "Zoom", "close": "Close", "ok": "OK", "cancel": "Cancel",
	},
	"zh": {
		"exit": "退出 Timestamps", "stillRunning": "Timestamps 仍在菜单栏中跟踪。从其菜单中选择“退出 Timestamps”即可停止。",
		"stopwatchStarted":   "秒表已开始。可从菜单栏或应用中停止。",
		"allowAccessibility": "允许辅助功能访问…", "allowScreen": "允许屏幕录制…",
		"axTitle": "允许 Timestamps 读取窗口标题",
		"axText": "要记录窗口标题和您正在处理的文档，Timestamps 需要“辅助功能”权限。没有它，只会记录应用名称。\n\n" +
			"所有数据都保留在这台 Mac 上。您可以随时在“系统设置 → 隐私与安全性 → 辅助功能”中更改。",
		"axOK": "打开系统设置", "notNow": "以后再说",
		"moveTitle": "请将 Timestamps 移到“应用程序”文件夹",
		"moveText":  "Timestamps 是在下载位置直接打开的，macOS 正在从临时副本运行它。请退出，将它移到“应用程序”文件夹后再打开，这样它才能在登录时启动并保留权限。",
		"about":     "关于 Timestamps", "settings": "设置…", "hide": "隐藏 Timestamps", "hideOthers": "隐藏其他", "showAll": "全部显示",
		"quit": "退出 Timestamps", "edit": "编辑", "undo": "撤销", "redo": "重做", "cut": "剪切", "copy": "拷贝", "paste": "粘贴",
		"selectAll": "全选", "window": "窗口", "minimize": "最小化", "zoom": "缩放", "close": "关闭", "ok": "好", "cancel": "取消",
	},
}

func systemChinese() bool { return strings.HasPrefix(mac.PreferredLanguage(), "zh") }

func (a *App) notify(title, text string) { a.notifyRoute(title, text, "") }
func (a *App) setTip(tip string)         { a.shell.SetTip(tip) }

// notifyRoute shows a notification that opens the window at route when
// clicked ("" = the away prompt).
func (a *App) notifyRoute(title, text, route string) {
	if route == "" {
		route = "day?away=1"
	}
	a.shell.Notify(title, text, route)
}

func appearance(theme string) int {
	switch theme {
	case "dark":
		return mac.AppearanceDark
	case "light":
		return mac.AppearanceLight
	}
	return mac.AppearanceSystem
}

func (a *App) run(showWindow bool) {
	labels := map[string]string{}
	for _, k := range menuLabels {
		labels[k] = a.Text(k)
	}
	a.shell = mac.NewShell(labels)
	a.shell.OnMenu = a.macMenu
	a.shell.OnCommand = a.macCommand
	a.shell.OnOpen = a.openWindow
	a.shell.OnLaunched = func() { a.launched(showWindow) }
	a.shell.OnWindowClosed = a.windowClosed
	a.shell.OnSuspend = a.t.Sleep
	a.shell.OnTerminate = a.terminate
	a.shell.SetAppearance(appearance(a.t.Settings().Theme))
	go a.watch()
	go a.tipLoop()
	go a.updateLoop()
	a.shell.Run() // returns only through OnTerminate and exit
}

// launched opens the window and, the first time, explains the
// Accessibility permission.
func (a *App) launched(showWindow bool) {
	if showWindow {
		a.openWindow("")
	}
	exe, _ := os.Executable()
	if mac.Translocated(exe) {
		mac.Alert(a.Text("moveTitle"), a.Text("moveText"))
	} else if cur := a.t.Settings(); cur.RunAtStartup && !mac.LoginItem(loginLabel, exe, loginArgs) {
		a.setLoginItem(true) // the app was moved
	}
	if !mac.AccessibilityTrusted() && a.db.Meta("mac.accessibilityAsked") == "" {
		a.db.SetMeta("mac.accessibilityAsked", "1")
		if mac.Ask(a.Text("axTitle"), a.Text("axText"), a.Text("axOK"), a.Text("notNow")) {
			mac.RequestAccessibility()
		}
	}
}

// macMenu is the tray menu, led by what still needs a permission.
func (a *App) macMenu() []menuItem {
	var items []menuItem
	if !mac.AccessibilityTrusted() {
		items = append(items, menuItem{ID: cmdAllowAccessibility, Text: a.Text("allowAccessibility")})
	}
	if a.t.Settings().ShotsOn && !mac.ScreenCaptureAllowed() {
		items = append(items, menuItem{ID: cmdAllowScreen, Text: a.Text("allowScreen")})
	}
	if len(items) > 0 {
		items = append(items, menuItem{Separator: true})
	}
	return append(items, a.menu()...)
}

func (a *App) macCommand(id int) {
	switch id {
	case cmdAllowAccessibility:
		mac.RequestAccessibility()
	case cmdAllowScreen:
		mac.RequestScreenCapture()
	default:
		a.command(id)
	}
}

// Show opens (or focuses) the app window at a route.
func (a *App) Show(route string) { a.openWindow(route) }

func (a *App) openWindow(route string) {
	script := ""
	if route != "" {
		b, _ := json.Marshal(route)
		script = "window.__navigate && window.__navigate(" + string(b) + ")"
	}
	a.shell.ShowWindow(a.srv.URL()+"#/"+strings.TrimPrefix(route, "/"), script, a.windowBackground())
}

func (a *App) windowClosed() {
	if a.exiting {
		return
	}
	if !a.t.Settings().CloseToTray {
		a.quit()
		return
	}
	if a.db.Meta("ui.trayHint") == "" {
		a.db.SetMeta("ui.trayHint", "1")
		a.notify("Timestamps", a.Text("stillRunning"))
	}
}

func (a *App) quit() {
	a.exiting = true
	a.shell.Quit()
}

// Quit exits the app (to install an update).
func (a *App) Quit() { a.quit() }

// terminate runs as the app quits (from the menu, ⌘Q, logout or shutdown).
func (a *App) terminate() {
	a.exiting = true
	a.t.Stop()
	a.srv.Close()
	if a.srv.Fav != nil {
		a.srv.Fav.Close()
	}
	a.db.Close()
	log.Println("exited")
}

// Open opens a file, folder or URL with the default app.
func (a *App) Open(target string) { mac.Open(target) }

// Reveal shows a file in the Finder.
func (a *App) Reveal(path string) { mac.Reveal(path) }

// PickFile shows an open panel. filter uses the Windows form
// ("Name\x00*.ext;*.ext2\x00..."): its first pattern limits the choice.
func (a *App) PickFile(title, filter string) string {
	exts := []string{"db"} // ManicTimeReports.db
	if filter != "" {
		exts = nil
		if parts := strings.Split(filter, "\x00"); len(parts) > 1 {
			for _, p := range strings.Split(parts[1], ";") {
				if e := strings.TrimPrefix(strings.TrimSpace(p), "*."); e != "" && e != "*" {
					exts = append(exts, e)
				}
			}
		}
	}
	return mac.PickFile(title, exts, "")
}

// PickFolder shows an open panel for folders, starting in dir.
func (a *App) PickFolder(title, dir string) string { return mac.PickFolder(title, dir) }

var loginArgs = []string{"--minimized"}

func (a *App) setLoginItem(on bool) {
	exe, _ := os.Executable()
	if on && mac.Translocated(exe) {
		a.notify(a.Text("moveTitle"), a.Text("moveText"))
		return
	}
	if err := mac.SetLoginItem(loginLabel, exe, loginArgs, on); err != nil {
		log.Println("autostart:", err)
	}
}

// ApplySettings reacts to settings changes that touch the OS.
func (a *App) ApplySettings(old, cur store.Settings) {
	if old.RunAtStartup != cur.RunAtStartup {
		a.setLoginItem(cur.RunAtStartup)
	}
	if old.Theme != cur.Theme {
		a.shell.SetAppearance(appearance(cur.Theme))
	}
	if cur.ShotsOn && !old.ShotsOn && !mac.ScreenCaptureAllowed() {
		mac.RequestScreenCapture()
	}
}

func systemDark() bool { return mac.SystemDark() }

// updateRing redraws the app's icon, a ring that fills once a day, in the
// menu bar and the Dock when it would look different: every 5 minutes, or
// when tracking pauses or resumes.
func (a *App) updateRing(paused bool) {
	key, frac, c := ringNow(paused)
	if key == a.ringKey {
		return
	}
	a.ringKey = key
	a.shell.SetStatusIcon(18, pngBytes(ring.Draw(18, frac, c)), pngBytes(ring.Draw(36, frac, c)))
	a.shell.SetAppIcon(pngBytes(ring.DrawIcon(512, frac, c)))
}

func pngBytes(img image.Image) []byte {
	var b bytes.Buffer
	if png.Encode(&b, img) != nil {
		return nil
	}
	return b.Bytes()
}
