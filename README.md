# Timestamps

A fast, private time tracker for Windows and macOS that does what ManicTime
does, with a modern interface and a fraction of the memory. It records which applications,
documents and websites you use, detects when you are away, takes periodic
screenshots, and lets you tag, report on and export your time. Everything
stays in one local SQLite database.

`timestamps.exe` is a single 10 MB executable with no runtime to install
(it uses the WebView2 runtime that ships with Windows 10/11).

## Download

From [Releases](https://github.com/ZJJ-2785/timestamps/releases), on Windows
get `Timestamps-Setup-<version>.exe`: it installs Timestamps for you only (no
admin rights) into `%LOCALAPPDATA%\Programs\Timestamps`, with a Start menu
shortcut and an uninstaller in *Apps & features* that keeps your data.
`Timestamps-<version>-windows-x64.exe` is the same app without installing.
Neither is code-signed, so Windows SmartScreen may ask once (*More info* →
*Run anyway*). The app checks for new releases (Settings → About) and can
update itself.

The browser extension zips are optional.

On a Mac, get `Timestamps-<version>-macos.zip` from the same page; see
[macOS](#macos).

## Features

| ManicTime | Timestamps |
| --- | --- |
| Computer usage timeline (active / away / off) | Same. Away after *N* minutes of no input (retroactive from the last input), lock screen = away, sleep/hibernate gaps, playing sound keeps you active (optional). *Mark as away* (tray or window) counts you away at once, until the next input. Optionally a long time away (*N* hours or more) counts as off, for a screen that never turns off. |
| Applications timeline with window titles | Same, with real app icons and names from the executable. UWP apps resolved behind ApplicationFrameHost. |
| Documents timeline (files and websites) | Same. **Full file paths** from Office/WPS (COM), Explorer windows, the program's command line and Windows *Recent items*; falls back to the name in the title. **Websites** read from the address bar of Chrome, Edge, Firefox, Brave, Opera, Vivaldi, 360, QQ and others via UI Automation, or from the optional browser extension. |
| Screenshots | Automatic, on by default: every 5 min while active, 1280 px JPEG (about 80 KB), unchanged screens skipped, kept 30 days. All configurable, plus "take screenshot now". |
| Tags timeline, tag combinations (`Client, Project, Task`), notes, billable | Same. Drag to select (snaps to activity edges), E to tag, edit/split/delete/copy/paste, recent tags, autocompletion. |
| "What were you doing while away?" | Notification and in-app prompt with recent and quick tags. |
| Stopwatch / countdown | From the tray or sidebar; stopwatch saves its time as a tag; countdown alerts and can tag. |
| Autotagging rules | Rules on app, title, URL, domain or file path (contains / equals / starts / ends / regex), live preview, suggestions lane in the day view, apply to a day. |
| Day view, zoom, filter, day summary | Canvas timeline with wheel zoom, pan, 24-hour overview strip, live "now" line, hover details, filter (Ctrl+F); summaries for the day or the selection. |
| Statistics | Totals with the change from the period before, activity by day/week/month, weekday × hour heatmap, focus sessions and app switches per hour, weekly rhythm (busiest day, peak hour, weekend share), top applications over time, top apps/sites/files/tags with drill-down, attendance table. |
| Timesheets | Tags × days, group by level, rounding (5–60 min, nearest/up/down, never-zero), billable only. |
| Search | Every day where a title, app, file, website, tag or note matches. |
| Export | CSV (UTF-8, Excel-friendly) and real .xlsx for timelines, statistics and timesheets. |
| Settings | Away time, sound, titles, documents, private browsing, tracking schedule, per-app privacy exceptions, startup, theme (light/dark/system), language (English / 简体中文), duration format, week start. |
| Backups, delete data | Backup now, scheduled backups with retention, delete any range per timeline. |
| — | **Import your ManicTime history** (Settings → Data). Imports only the time before Timestamps started tracking, so nothing is counted twice. |
| — | The same UI also works in any browser at `http://127.0.0.1:47613` while the app runs. |

Not included: ManicTime Server/team sync, calendar timelines (Google/Outlook)
and the password lock.

## Performance (measured on this PC with 4.5 years of imported ManicTime data)

| | Timestamps | ManicTime 5 |
| --- | --- | --- |
| Memory while tracking (window closed) | ~50 MB, 1 process | ~280 MB, 3 processes |
| Database for the same history | 76 MB | 399 MB |
| Open a day | 5–10 ms | — |
| Statistics: month / year / all time | 28 ms / 132 ms / 640 ms | — |
| Search all history | ~0.2 s | — |
| Import 567k ManicTime records | 11.5 s | — |

Closing the window destroys the WebView (freeing its memory); tracking
continues from the tray.

## Using it

Run the exe (or `build\timestamps.exe` when built from source). The tray icon opens the window (left click) or a
menu (right click: stopwatch, screenshot, mark as away, pause tracking, exit).
`timestamps.exe --minimized` starts in the tray; *Start with Windows* in
Settings does that at login.

Data lives in `%LOCALAPPDATA%\Timestamps` (`timestamps.db`, `screenshots\`,
`backups\`, `extension\`). Use `--data <folder>` to run with another folder.

### Browser extension (optional)

Websites are tracked without it. The extension is more precise and can mark
private windows. The app unpacks it to `%LOCALAPPDATA%\Timestamps\extension`;
the same files are in the release zips.

* Chrome / Edge / Brave / Opera / Vivaldi: open `chrome://extensions`, enable
  Developer mode, *Load unpacked*, pick that folder (or the unzipped
  `…-extension-chromium.zip`).
* Firefox: `about:debugging#/runtime/this-firefox` → *Load Temporary Add-on* →
  `extension\firefox\manifest.json` (or the `…-extension-firefox.zip`). It is
  not signed by Mozilla, so release Firefox removes it on restart; Firefox
  Developer Edition/Nightly can keep it with `xpinstall.signatures.required`
  set to `false`.

It only talks to `127.0.0.1`.

### Keyboard shortcuts (day view)

F5 today · F7/F8 previous/next day · wheel zoom · Shift+wheel scroll · drag to
select · E tag · Delete clear tags · B billable · Ctrl+C/Ctrl+V copy/paste tag ·
+/− zoom · 0 fit · Shift+0 whole day · Ctrl+F filter · Ctrl+K search · Esc.

## macOS

One universal app for Apple silicon and Intel Macs, macOS 11.3 or later. It
lives in the menu bar as the day ring (as in the Windows tray); the window
opens from its menu, and the Dock icon, the same live ring, shows only while
the window is open.

### Install

1. Unzip `Timestamps-<version>-macos.zip` and drag **Timestamps** into
   *Applications*.
   Open it from there: an app opened in Downloads runs from a temporary copy
   and can't start at login.
2. The app is signed ad hoc but not notarized by Apple, so macOS blocks the
   first start. Try to open it once, then go to *System Settings → Privacy &
   Security*, scroll down and click **Open Anyway** (on macOS 14 and earlier,
   Control-click the app and choose *Open* instead). Or, in Terminal:

   ```bash
   xattr -dr com.apple.quarantine /Applications/Timestamps.app
   ```

### Permissions

macOS asks for each of these separately. Timestamps keeps working without
them, recording less; missing ones are listed at the top of its menu.

| Permission | What it is for | Without it |
| --- | --- | --- |
| **Accessibility** | Window titles and the document a window shows | Only app names (titles from Screen Recording, if allowed) |
| **Screen Recording** | Screenshots; window titles as a fallback | No screenshots. macOS applies it after Timestamps restarts |
| **Automation** (asked per browser) | The address of the current tab in Safari, Chrome, Edge, Brave, Arc, Vivaldi and Opera | No websites from that browser, unless the extension is installed |
| Notifications | Away prompts, stopwatch and countdown alerts | No banners |

They are in *System Settings → Privacy & Security*. A rebuilt or updated app
is a new app to macOS, so it may have to be allowed again.

### Differences from Windows

* Documents come from the window's open document (what the title bar's
  proxy icon shows), else the file name in the title, looked up with
  Spotlight.
* Firefox has no scripting interface: websites from Firefox need the browser
  extension (same steps as above; the folder is
  `~/Library/Application Support/Timestamps/extension`).
* Shortcuts use ⌘ instead of Ctrl (⌘C/⌘V copy/paste a tag, ⌘F filter, ⌘K
  search); ⌘, opens Settings and ⌘W closes the window.
* Data lives in `~/Library/Application Support/Timestamps`. *Open at login*
  writes `~/Library/LaunchAgents/io.github.zjj2785.timestamps.plist`.
* To import ManicTime history, copy `ManicTimeReports.db` from the Windows PC
  and choose it in Settings → Data.
* *Update* (Settings → About) downloads the new app, puts it in place of the
  old one once Timestamps has quit, and opens it. When it can't replace the
  app (not in a folder you can write to, or opened from Downloads), it opens
  the release page instead. macOS may ask for the permissions again.

## Building

Windows: Go 1.20+, a MinGW-w64 gcc (cgo, for SQLite) and `windres`.

```powershell
.\build.ps1        # build\timestamps.exe
.\build-setup.ps1  # also build\Timestamps-Setup-<version>.exe (tools/setup)
```

macOS: Go 1.20+ and the Xcode command line tools. This builds both
architectures, makes `build/Timestamps.app`, signs it ad hoc and zips it:

```bash
sh build-mac.sh
```

GitHub Actions (`.github/workflows/build.yml`) runs the tests and both builds,
on a Mac and on Windows, for every push.

To release, set the new version in `internal/server/server.go`, `assets/app.rc`,
`assets/app.manifest`, `tools/setup/setup.rc` and the extension manifests,
regenerate the two `rsrc_windows_amd64.syso` files (delete them and run
`.\build-setup.ps1`), write `release-notes/v<version>.md`, then push a
`v<version>` tag. The workflow builds every file and publishes the release
with those notes.

`go run ./tools/genicon .` regenerates the icons (`.ico`, `.icns`, PNGs);
`go test ./...` runs the tests. Set `TIMESTAMPS_WEB_DIR=<repo>\web` to serve the UI from disk
while developing.

## Layout

```
main.go, shell.go        entry point, tray menu, notifications (both platforms)
*_windows.go             tray, app window (WebView2), single instance, autostart
*_darwin.go              menu bar, app window (WKWebView), login item, permissions
internal/win             Win32: windows, processes, idle/lock, icons, capture, tray
internal/mac             macOS (cgo + Objective-C): front app, Accessibility, idle/lock,
                         browser tabs, Spotlight, capture, menu bar, window, dialogs
internal/tracker         1 s sampler, documents, screenshots, extension hub
                         (*_windows.go: UIA/COM/Recent; *_darwin.go: AX/Apple events)
internal/store           SQLite schema, queries, stats, tags, timesheets, search, autotag, import
internal/server          local HTTP API + event stream, stopwatch/countdown, backups
web/                     UI (plain ES modules, no build step), embedded into the exe
extension/               Chromium (MV3) and Firefox extensions, embedded into the exe
```

The API only accepts requests whose Host is `127.0.0.1`/`localhost` and that
carry the session cookie set by the UI (SameSite=Strict); state-changing calls
also need a custom header, so web pages cannot reach it.
