package tracker

import (
	"net/url"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"timestamps/internal/store"
)

// Doc is the document detected in the foreground window.
type Doc struct {
	Kind    int    // store.KindFile or store.KindWeb, 0 = none
	Key     string // full path or URL
	Name    string // file name or page title
	Grp     string // file name or domain
	Private bool
}

// IsBrowser reports whether an executable is a web browser.
func IsBrowser(exe string) bool { return browserExes[exe] }

var docExt = map[string]bool{}

func init() {
	for _, e := range strings.Fields(`doc docx docm dot dotx rtf odt wps wpt xls xlsx xlsm xlsb csv et ett ods ppt pptx pptm pps ppsx odp dps dpt
		pdf ofd txt md markdown log ini cfg conf json jsonc yaml yml toml xml html htm css scss less js mjs cjs ts tsx jsx vue svelte
		go py rb rs java kt kts c h cc cpp hpp cs fs vb php swift m mm sql sh bat cmd ps1 psm1 lua r dart scala clj ex exs erl hs
		ipynb tex bib epub mobi azw3 djvu chm png jpg jpeg gif bmp webp tif tiff svg ico heic raw psd psb ai eps indd xd fig sketch
		cdr afdesign afphoto mp3 wav flac m4a aac ogg mp4 mkv avi mov wmv flv webm m4v dwg dxf dwf rvt rfa ipt iam idw skp 3dm step stp
		igs iges stl obj fbx blend max ma mb c4d prt sldprt sldasm zip rar 7z tar gz bz2 xz iso vsd vsdx mpp one pub xps eml msg vcf
		unity uproject sln csproj vcxproj drawio xmind mm mmap kml gpx bak dat`) {
		docExt[e] = true
	}
}

var reFileToken = regexp.MustCompile(`([^\\/:*?"<>|\t]+?\.([A-Za-z0-9]{1,10}))(?:\s*[\[(].*?[\])])?\s*$`)

// fileFromTitle finds a file name (or full path) in a window title, e.g.
// "Report.docx - Word" or "● main.go - project - Visual Studio Code".
func fileFromTitle(title string) string {
	t := strings.TrimSpace(title)
	for _, p := range []string{"● ", "• ", "*", "○ "} {
		t = strings.TrimPrefix(t, p)
	}
	for _, sep := range []string{" - ", " — ", " – ", " | "} {
		t = strings.ReplaceAll(t, sep, "\x00")
	}
	for _, part := range strings.Split(t, "\x00") {
		part = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(part), "*"))
		part = strings.TrimSuffix(part, " [Read-Only]")
		part = strings.TrimSuffix(part, " [只读]")
		part = strings.TrimSuffix(part, " (Read-Only)")
		if len(part) > 2 && part[1] == ':' && (part[2] == '\\' || part[2] == '/') {
			if docExt[strings.TrimPrefix(strings.ToLower(filepath.Ext(part)), ".")] {
				return part
			}
		}
		if m := reFileToken.FindStringSubmatch(part); m != nil {
			if docExt[strings.ToLower(m[2])] && !strings.Contains(m[1], "://") {
				return strings.TrimSpace(m[1])
			}
		}
	}
	return ""
}

type docReq struct {
	hwnd  uintptr
	pid   uint32
	exe   string
	title string
	opts  docOpts
	resp  chan Doc
}

type docOpts struct {
	addressBar bool
	extURL     func(title string) (url string, private bool, ok bool)
}

// detector finds the document shown in a window. Each platform has one;
// it lives on the worker's locked thread and is only used from there.
type detector interface {
	detect(r docReq) Doc
	close()
}

// docWorker answers document queries on a thread of its own, so a slow or
// hung application can't stall the tracker.
type docWorker struct {
	reqs chan docReq
	mu   sync.Mutex
	busy time.Time
}

func newDocWorker() *docWorker {
	w := &docWorker{}
	w.start()
	return w
}

func (w *docWorker) start() {
	w.reqs = make(chan docReq, 1)
	reqs := w.reqs
	go func() {
		runtime.LockOSThread()
		d := newDetector()
		defer d.close()
		for r := range reqs {
			w.mu.Lock()
			w.busy = time.Now()
			w.mu.Unlock()
			doc := d.detect(r)
			w.mu.Lock()
			w.busy = time.Time{}
			w.mu.Unlock()
			r.resp <- doc
		}
	}()
}

// query asks the worker for the document in a window, giving up after
// timeout. A worker stuck in a call for too long is replaced.
func (w *docWorker) query(r docReq, timeout time.Duration) (Doc, bool) {
	r.resp = make(chan Doc, 1)
	w.mu.Lock()
	busy := w.busy
	w.mu.Unlock()
	if !busy.IsZero() && time.Since(busy) > 20*time.Second {
		w.start() // abandon the hung thread
	}
	select {
	case w.reqs <- r:
	default:
		return Doc{}, false
	}
	select {
	case d := <-r.resp:
		return d, true
	case <-time.After(timeout):
		return Doc{}, false
	}
}

func fileDoc(p string) Doc {
	base := filepath.Base(p)
	return Doc{Kind: store.KindFile, Key: p, Name: base, Grp: base}
}

var browserSuffix = regexp.MustCompile(`\s+[-—–]\s+(Google Chrome|Microsoft Edge|Brave|Opera|Vivaldi|Chromium|Mozilla Firefox|Firefox|Waterfox|LibreWolf|Floorp|Zen Browser|360安全浏览器|360极速浏览器|QQ浏览器|搜狗高速浏览器|Yandex|Arc|Thorium)(\s+(Beta|Dev|Canary|Nightly|Developer Edition))?$`)

// With several profiles, Chrome and Edge on a Mac add the profile after the
// browser name: "Page - Google Chrome - Work". Only the two-word names, so a
// page title that merely mentions "Opera" or "Brave" keeps its last part.
var profileAfter = regexp.MustCompile(`\s+[-—–]\s+(Google Chrome|Microsoft Edge|Chromium)(\s+(Beta|Dev|Canary))?\s+[-—–]\s+[^-—–]+$`)

// Edge on Windows puts the profile before its name: "Page - Personal -
// Microsoft Edge".
var edgeProfile = regexp.MustCompile(`\s+[-—–]\s+[^-—–]+\s+[-—–]\s+Microsoft Edge(\s+(Beta|Dev|Canary))?$`)

var morePages = regexp.MustCompile(`\s+(and \d+ more pages?|和另外\s*\d+\s*个页面|和其他\s*\d+\s*个页面|及其他\s*\d+\s*个页面)(\s+[-—–]\s+.*)?$`)

// PageTitle strips the browser name, profile and "and N more pages" from a
// browser window title.
func PageTitle(title string) string {
	t := strings.ReplaceAll(title, "​", "") // Edge puts a zero-width space in "Microsoft Edge"
	t = profileAfter.ReplaceAllString(t, "")
	t = edgeProfile.ReplaceAllString(t, "")
	t = browserSuffix.ReplaceAllString(t, "")
	t = morePages.ReplaceAllString(t, "")
	return strings.TrimSpace(t)
}

var reHostLike = regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+(:\d+)?(/|$)|^localhost(:\d+)?(/|$)|^\d+\.\d+\.\d+\.\d+(:\d+)?(/|$)`)

// webDoc normalises an address bar value into a website document.
func webDoc(raw, page string, private bool) Doc {
	if raw == "" || strings.ContainsAny(raw, " \t\n") {
		return Doc{}
	}
	low := strings.ToLower(raw)
	for _, p := range []string{"chrome://", "edge://", "about:", "brave://", "opera://", "vivaldi://", "chrome-extension://", "moz-extension://",
		"extension://", "ms-browser-extension://", "view-source:", "devtools://", "data:", "blob:", "javascript:", "chrome-search://", "edge-extension://"} {
		if strings.HasPrefix(low, p) {
			return Doc{}
		}
	}
	if !strings.Contains(low, "://") {
		if !reHostLike.MatchString(raw) {
			return Doc{}
		}
		raw = "https://" + raw
	} else if strings.HasPrefix(low, "file://") {
		if p := fileURLToPath(raw); p != "" {
			return fileDoc(p)
		}
		return Doc{}
	}
	if i := strings.IndexByte(raw, '#'); i > 0 {
		raw = raw[:i]
	}
	name := page
	if name == "" {
		name = raw
	}
	return Doc{Kind: store.KindWeb, Key: raw, Name: name, Grp: store.Domain(raw), Private: private}
}

func fileURLToPath(u string) string {
	if !strings.HasPrefix(strings.ToLower(u), "file:///") {
		return ""
	}
	rest := u[8:] // C:/...
	if runtime.GOOS != "windows" {
		rest = u[7:] // keep the root slash
	}
	p, err := url.PathUnescape(rest)
	if err != nil {
		return ""
	}
	return filepath.FromSlash(p)
}
