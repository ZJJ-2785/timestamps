# User guide

## Using it

Run the app (or `build\timestamps.exe` when built from source). The tray icon
opens the window (left click) or a menu (right click: stopwatch, screenshot,
mark as off, pause tracking, exit). `timestamps.exe --minimized` starts in the
tray; *Start with Windows* in Settings does that at login.

Data lives in `%LOCALAPPDATA%\Timestamps` (`timestamps.db`, `screenshots\`,
`backups\`, `extension\`). Use `--data <folder>` to run with another folder.
The same window also works in any browser at `http://127.0.0.1:47613` while
the app runs.

The Windows setup installs into `%LOCALAPPDATA%\Programs\Timestamps` with a
Start menu shortcut and an uninstaller in *Apps & features* that keeps your
data. `Timestamps-<version>-windows-x64.exe` on the Releases page is the same
app without installing. On a Mac, see the [macOS guide](macos.md).

### Keyboard shortcuts (day view)

F5 today · F7/F8 previous/next day · wheel zoom · Shift+wheel scroll · drag to
select · E tag · Delete clear tags · B billable · Ctrl+C/Ctrl+V copy/paste tag ·
+/− zoom · 0 fit · Shift+0 whole day · Ctrl+F filter · Ctrl+K search · Esc.

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

## Features

Timestamps does what ManicTime does, with a modern interface and a fraction
of the memory.

| ManicTime | Timestamps |
| --- | --- |
| Computer usage timeline (active / away / off) | Same. Away after *N* minutes of no input (retroactive from the last input), lock screen = away, sleep/hibernate gaps, playing sound keeps you active (optional). *Mark as off* (tray or window) records nothing from then until the next input, as if the computer were off. Optionally a long time away (*N* hours or more) counts as off, for a screen that never turns off. |
| Applications timeline with window titles | Same, with real app icons and names from the executable. UWP apps resolved behind ApplicationFrameHost. |
| Documents timeline (files and websites) | Same. **Full file paths** from Office/WPS (COM), Explorer windows, the program's command line and Windows *Recent items*; falls back to the name in the title. **Websites** read from the address bar of Chrome, Edge, Firefox, Brave, Opera, Vivaldi, 360, QQ and others via UI Automation, or from the optional browser extension. |
| — | **Keyboard and mouse use**: key presses, left/right/middle/other clicks, mouse distance and scrolling per day, live in the day view with a chart by hour and the keys and clicks of each application, and a keyboard heatmap, the top keys as a donut and as bars, a sortable table per application and CSV/Excel export in the statistics (like [KeyStats](https://github.com/debugtheworldbot/keyStats)). Keys are kept as totals per key and day: never the order or what was typed, and keys pressed in an app set to Private are left out, except in the first second after switching to it. Nothing is counted while paused or marked off; can be switched off. On a Mac it is off until switched on, and needs Input Monitoring. |
| Screenshots | Automatic, on by default: every 5 min while active, 1280 px JPEG (about 80 KB), unchanged screens skipped, kept 30 days. All configurable, plus "take screenshot now". |
| Tags timeline, tag combinations (`Client, Project, Task`), notes, billable | Same. Drag to select (snaps to activity edges), E to tag, edit/split/delete/copy/paste, recent tags, autocompletion. |
| "What were you doing while away?" | Notification and in-app prompt with recent and quick tags. |
| Stopwatch / countdown | From the tray or sidebar; stopwatch saves its time as a tag; countdown alerts and can tag. |
| Autotagging rules | Rules on app, title, URL, domain or file path (contains / equals / starts / ends / regex), live preview, suggestions lane in the day view, apply to a day. |
| Day view, zoom, filter, day summary | Canvas timeline with wheel zoom, pan, 24-hour overview strip, live "now" line, hover details, filter (Ctrl+F); summaries for the day or the selection. Blocks you can drag, resize and pin. |
| Statistics | Blocks you can drag, resize and pin, as in the day view. Totals with the change from the period before, activity by day/week/month, weekday × hour heatmap, focus sessions and app switches per hour, weekly rhythm (busiest day, peak hour, weekend share), top applications over time, top apps/sites/files/tags with drill-down, attendance table. |
| Timesheets | Tags × days, group by level, rounding (5–60 min, nearest/up/down, never-zero), billable only. |
| Search | Every day where a title, app, file, website, tag or note matches. |
| Export | CSV (UTF-8, Excel-friendly) and real .xlsx for timelines, statistics and timesheets. |
| Settings | Away time, sound, titles, documents, private browsing, tracking schedule, per-app privacy exceptions, startup, theme (light/dark/system), language (English / 简体中文), time zone, duration format, week start. |
| Backups, delete data | Backup now, scheduled backups with retention, delete any range per timeline. |
| — | **Import your ManicTime history** (Settings → Data). Imports only the time before Timestamps started tracking, so nothing is counted twice. |
| — | **Import your KeyStats history** (Settings → Data): daily totals, keys and applications, for the days before Timestamps started counting. KeyStats has no time of day, so those days have no chart by hour. Files from KeyStats for Mac can't be imported yet. |
| — | The same UI also works in any browser at `http://127.0.0.1:47613` while the app runs. |

Not included: ManicTime Server/team sync, calendar timelines (Google/Outlook)
and the password lock.

## Performance

Measured with 4.5 years of imported ManicTime data:

| | Timestamps | ManicTime 5 |
| --- | --- | --- |
| Memory while tracking (window closed) | ~50 MB, 1 process | ~280 MB, 3 processes |
| Database for the same history | 76 MB | 399 MB |
| Open a day | 5–10 ms | — |
| Statistics: month / year / all time | 28 ms / 132 ms / 640 ms | — |
| Search all history | ~0.2 s | — |
| Import 567k ManicTime records | 11.5 s | — |

Closing the window destroys the WebView (freeing its memory); tracking
continues from the tray. `timestamps.exe` is a single 11 MB executable with no
runtime to install (it uses the WebView2 runtime that ships with Windows
10/11).
