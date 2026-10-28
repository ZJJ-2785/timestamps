package main

import (
	"fmt"
	"image/color"
	"time"

	"timestamps/internal/ring"
	"timestamps/internal/server"
	"timestamps/internal/store"
	"timestamps/internal/tracker"
)

// App is the desktop shell: tray (menu bar) icon, app window and
// notifications. What differs per platform lives in native and in
// shell_windows.go / shell_darwin.go.
type App struct {
	db      *store.DB
	t       *tracker.Tracker
	srv     *server.Server
	dataDir string
	exiting bool
	wake    chan struct{} // tipLoop redraws now rather than at its next round
	native
}

const (
	cmdOpen = iota + 1
	cmdStopwatchStart
	cmdStopwatchStop
	cmdStopwatchDiscard
	cmdShot
	cmdPause15
	cmdPause60
	cmdPauseForever
	cmdResume
	cmdStats
	cmdExit
)

func newApp(db *store.DB, t *tracker.Tracker, srv *server.Server, dataDir string) *App {
	return &App{db: db, t: t, srv: srv, dataDir: dataDir, wake: make(chan struct{}, 1)}
}

func (a *App) lang() string {
	l := a.t.Settings().Language
	if l == "auto" || l == "" {
		if systemChinese() {
			return "zh"
		}
		return "en"
	}
	return l
}

// Text returns a localized shell string.
func (a *App) Text(key string) string {
	if s, ok := platformText[a.lang()][key]; ok {
		return s
	}
	if s, ok := shellText[a.lang()][key]; ok {
		return s
	}
	return shellText["en"][key]
}

func fmtClock(secs int64) string {
	return fmt.Sprintf("%d:%02d:%02d", secs/3600, secs/60%60, secs%60)
}

func (a *App) menu() []menuItem {
	items := []menuItem{{ID: cmdOpen, Text: a.Text("open")}, {ID: cmdStats, Text: a.Text("statistics")}, {Separator: true}}
	st := a.srv.Timers().State()
	if sw, _ := st["stopwatch"].(*server.Stopwatch); sw != nil {
		label := a.Text("stopStopwatch") + "  " + fmtClock(time.Now().Unix()-sw.Start)
		if sw.Tags != "" {
			label += "  · " + sw.Tags
		}
		items = append(items, menuItem{ID: cmdStopwatchStop, Text: label}, menuItem{ID: cmdStopwatchDiscard, Text: a.Text("discardStopwatch")})
	} else {
		items = append(items, menuItem{ID: cmdStopwatchStart, Text: a.Text("startStopwatch")})
	}
	items = append(items, menuItem{ID: cmdShot, Text: a.Text("takeShot")})
	if l := a.t.Live(); l.State == "paused" {
		label := a.Text("resume")
		if l.PausedTill > 0 {
			label += "  (" + time.Unix(l.PausedTill, 0).Format("15:04") + ")"
		}
		items = append(items, menuItem{ID: cmdResume, Text: label})
	} else {
		items = append(items, menuItem{Text: a.Text("pause"), Sub: []menuItem{
			{ID: cmdPause15, Text: a.Text("pause15")},
			{ID: cmdPause60, Text: a.Text("pause60")},
			{ID: cmdPauseForever, Text: a.Text("pauseForever")},
		}})
	}
	return append(items, menuItem{Separator: true}, menuItem{ID: cmdExit, Text: a.Text("exit")})
}

func (a *App) command(id int) {
	tm := a.srv.Timers()
	switch id {
	case cmdOpen:
		a.openWindow("")
	case cmdStats:
		a.openWindow("stats")
	case cmdStopwatchStart:
		tm.StartStopwatch("", "", 0)
		a.notify("Timestamps", a.Text("stopwatchStarted"))
	case cmdStopwatchStop:
		tm.StopStopwatch(true)
		a.t.Emit("tags")
	case cmdStopwatchDiscard:
		tm.StopStopwatch(false)
	case cmdShot:
		go a.t.Shots.Capture(a.t.Settings())
	case cmdPause15:
		a.t.Pause(15 * time.Minute)
	case cmdPause60:
		a.t.Pause(time.Hour)
	case cmdPauseForever:
		a.t.Pause(0)
	case cmdResume:
		a.t.Resume()
	case cmdExit:
		a.quit()
	}
}

// watch turns tracker events into notifications.
func (a *App) watch() {
	ch := a.t.Subscribe()
	for ev := range ch {
		if ev.Type == "state" {
			// Paused or resumed: the ring turns gray, or back, at once.
			select {
			case a.wake <- struct{}{}:
			default:
			}
			continue
		}
		if ev.Type != "away" {
			continue
		}
		aw := a.t.Aways()
		if len(aw) == 0 {
			continue
		}
		last := aw[len(aw)-1]
		mins := (last.E - last.S) / 60
		text := fmt.Sprintf(a.Text("awayText"), time.Unix(last.S, 0).Format("15:04"), time.Unix(last.E, 0).Format("15:04"), mins/60, mins%60)
		a.notifyRoute(a.Text("awayTitle"), text, "") // clicking it opens the away prompt
	}
}

// updateLoop looks for a new release a minute after start and then twice a
// day, and tells once per version when there is one.
func (a *App) updateLoop() {
	time.Sleep(time.Minute)
	for {
		if a.t.Settings().UpdateCheck {
			if u := a.srv.CheckUpdate(false); u.Available && a.db.Meta("update.told") != u.Latest {
				a.db.SetMeta("update.told", u.Latest)
				a.notifyRoute(a.Text("updateTitle"), fmt.Sprintf(a.Text("updateText"), u.Latest), "settings/about")
			}
		}
		time.Sleep(12 * time.Hour)
	}
}

func (a *App) tipLoop() {
	for {
		l := a.t.Live()
		day := time.Now().Format("2006-01-02")
		tip := "Timestamps"
		if s, e, err := store.DayRange(day); err == nil {
			if st, err := a.db.Stats(s, e, false); err == nil {
				tip += fmt.Sprintf(" · %s %dh %02dm", a.Text("activeToday"), st.Active/3600, st.Active/60%60)
			}
		}
		if l.State == "paused" {
			tip += " · " + a.Text("pausedTip")
		}
		a.setTip(tip)
		a.updateRing(l.State == "paused")
		select {
		case <-a.wake:
		case <-time.After(30 * time.Second):
		}
	}
}

// ringNow is what the app's icon, a ring that fills once a day, shows now.
func ringNow(paused bool) (key string, frac float64, c color.NRGBA) {
	return ringAt(time.Now(), paused)
}

// ringAt is the ring at time t: the clock time in 5-minute steps, as the
// window's logo draws it (web/js/main.js drawLogo), so on the days the clocks
// change it follows the clock too. Its color is gray while paused; key
// changes whenever the drawing would.
func ringAt(t time.Time, paused bool) (key string, frac float64, c color.NRGBA) {
	step := (t.Hour()*60 + t.Minute()) / 5
	frac = float64(step) / (24 * 60 / 5)
	c = ring.Color(frac)
	if paused {
		c = ring.Gray
	}
	return fmt.Sprint(step, paused), frac, c
}

// Page background of the color themes (as in web/app.css): light, dark.
var themeBG = map[string][2]string{
	"classic": {"f4f5f8", "0c0f15"}, "dkhd": {"ffeef4", "1c1827"},
	"onedark": {"", "21252b"}, "dracula": {"", "21222c"}, "tokyonight": {"", "16161e"}, "githubdark": {"", "010409"}, "gruvboxdark": {"", "1d2021"},
	"githublight": {"f6f8fa", ""}, "onelight": {"eaeaeb", ""}, "solarized": {"eee8d5", ""}, "latte": {"e6e9ef", ""}, "rosepinedawn": {"f2e9e1", ""},
}

// windowBackground is the page background (RRGGBB) of the theme in use.
func (a *App) windowBackground() string {
	st, i := a.t.Settings(), 0
	id := st.LightTheme
	if a.prefersDark() {
		id, i = st.DarkTheme, 1
	}
	if c := themeBG[id][i]; c != "" {
		return c
	}
	return themeBG["classic"][i]
}

func (a *App) prefersDark() bool {
	switch a.t.Settings().Theme {
	case "dark":
		return true
	case "light":
		return false
	}
	return systemDark()
}

// Notify shows a tray notification.
func (a *App) Notify(title, text string) { a.notify(title, text) }

var shellText = map[string]map[string]string{
	"en": {
		"open": "Open Timestamps", "statistics": "Statistics", "startStopwatch": "Start stopwatch",
		"stopStopwatch": "Stop stopwatch", "discardStopwatch": "Discard stopwatch", "takeShot": "Take screenshot now",
		"pause": "Pause tracking", "pause15": "For 15 minutes", "pause60": "For 1 hour", "pauseForever": "Until I resume",
		"resume": "Resume tracking", "exit": "Exit", "stillRunning": "Timestamps keeps tracking in the notification area. Right-click the icon to exit.",
		"updateTitle": "Update available", "updateText": "Timestamps %s is ready. Click to update.",
		"awayTitle": "Welcome back", "awayText": "You were away %s–%s (%dh %02dm). Click to tag this time.",
		"countdownDone": "Countdown finished", "stopwatchRunning": "Stopwatch running", "stopwatchStarted": "Stopwatch started. Stop it from the tray icon or the app.",
		"activeToday": "active today", "pausedTip": "tracking paused",
	},
	"zh": {
		"open": "打开 Timestamps", "statistics": "统计", "startStopwatch": "开始秒表",
		"stopStopwatch": "停止秒表", "discardStopwatch": "放弃秒表", "takeShot": "立即截图",
		"pause": "暂停跟踪", "pause15": "15 分钟", "pause60": "1 小时", "pauseForever": "直到我恢复",
		"resume": "恢复跟踪", "exit": "退出", "stillRunning": "Timestamps 仍在通知区域中跟踪。右键单击图标可退出。",
		"updateTitle": "有可用更新", "updateText": "Timestamps %s 已发布。点击即可更新。",
		"awayTitle": "欢迎回来", "awayText": "您离开了 %s–%s（%d 小时 %02d 分）。点击为这段时间添加标签。",
		"countdownDone": "倒计时结束", "stopwatchRunning": "秒表运行中", "stopwatchStarted": "秒表已开始。可从托盘图标或应用中停止。",
		"activeToday": "今日活动", "pausedTip": "已暂停跟踪",
	},
}
