# macOS

One universal app for Apple silicon and Intel Macs, macOS 11.3 or later. It
lives in the menu bar as the day ring (as in the Windows tray); the window
opens from its menu, and the Dock icon, the same live ring, shows only while
the window is open.

## Install

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

## Permissions

macOS asks for each of these separately. Timestamps keeps working without
them, recording less; missing Accessibility or Screen Recording is listed at
the top of its menu.

| Permission | What it is for | Without it |
| --- | --- | --- |
| **Accessibility** | Window titles and the document a window shows | Only app names (titles from Screen Recording, if allowed) |
| **Screen Recording** | Screenshots; window titles as a fallback | No screenshots. macOS applies it after Timestamps restarts |
| **Automation** (asked per browser) | The address of the current tab in Safari, Chrome, Edge, Brave, Arc, Vivaldi and Opera | No websites from that browser, unless the extension is installed |
| Notifications | Away prompts, stopwatch and countdown alerts | No banners |
| **Input Monitoring** (only if *Count keyboard and mouse use* is on) | Counting keys, clicks, mouse distance and scrolling | No counts. Applies after switching the setting off and on |

They are in *System Settings → Privacy & Security*. A rebuilt or updated app
is a new app to macOS, so it may have to be allowed again.

## Differences from Windows

* Documents come from the window's open document (what the title bar's
  proxy icon shows), else the file name in the title, looked up with
  Spotlight.
* Firefox has no scripting interface: websites from Firefox need the browser
  extension (same steps as in the [user guide](guide.md#browser-extension-optional);
  the folder is `~/Library/Application Support/Timestamps/extension`).
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
