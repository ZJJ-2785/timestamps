# Building from source

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

`go run ./tools/genicon .` regenerates the icons (`.ico`, `.icns`, PNGs);
`go test ./...` runs the tests. Set `TIMESTAMPS_WEB_DIR=<repo>\web` to serve
the UI from disk while developing.

## Releasing

Set the new version in `internal/server/server.go`, `assets/app.rc`,
`assets/app.manifest`, `tools/setup/setup.rc` and the extension manifests,
regenerate the two `rsrc_windows_amd64.syso` files (delete them and run
`.\build-setup.ps1`), write `release-notes/v<version>.md`, then push a
`v<version>` tag. The workflow builds every file and publishes the release
with those notes.

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
