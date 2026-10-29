//go:build windows

// Package win wraps the handful of Win32 APIs the tracker and shell need.
package win

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	versionD = windows.NewLazySystemDLL("version.dll")
	dwmapi   = windows.NewLazySystemDLL("dwmapi.dll")
	ntdll    = windows.NewLazySystemDLL("ntdll.dll")
	comdlg32 = windows.NewLazySystemDLL("comdlg32.dll")
	wtsapi32 = windows.NewLazySystemDLL("wtsapi32.dll")

	pGetForegroundWindow           = user32.NewProc("GetForegroundWindow")
	pGetWindowTextW                = user32.NewProc("GetWindowTextW")
	pGetWindowTextLengthW          = user32.NewProc("GetWindowTextLengthW")
	pGetWindowThreadProcessId      = user32.NewProc("GetWindowThreadProcessId")
	pGetClassNameW                 = user32.NewProc("GetClassNameW")
	pGetLastInputInfo              = user32.NewProc("GetLastInputInfo")
	pEnumChildWindows              = user32.NewProc("EnumChildWindows")
	pIsWindow                      = user32.NewProc("IsWindow")
	pIsWindowVisible               = user32.NewProc("IsWindowVisible")
	pOpenInputDesktop              = user32.NewProc("OpenInputDesktop")
	pCloseDesktop                  = user32.NewProc("CloseDesktop")
	pGetUserObjectInformationW     = user32.NewProc("GetUserObjectInformationW")
	pMonitorFromWindow             = user32.NewProc("MonitorFromWindow")
	pGetMonitorInfoW               = user32.NewProc("GetMonitorInfoW")
	pGetDC                         = user32.NewProc("GetDC")
	pReleaseDC                     = user32.NewProc("ReleaseDC")
	pDestroyIcon                   = user32.NewProc("DestroyIcon")
	pGetIconInfo                   = user32.NewProc("GetIconInfo")
	pPrivateExtractIconsW          = user32.NewProc("PrivateExtractIconsW")
	pSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	pRegisterClassExW              = user32.NewProc("RegisterClassExW")
	pCreateWindowExW               = user32.NewProc("CreateWindowExW")
	pDefWindowProcW                = user32.NewProc("DefWindowProcW")
	pDestroyWindow                 = user32.NewProc("DestroyWindow")
	pGetMessageW                   = user32.NewProc("GetMessageW")
	pTranslateMessage              = user32.NewProc("TranslateMessage")
	pDispatchMessageW              = user32.NewProc("DispatchMessageW")
	pPostMessageW                  = user32.NewProc("PostMessageW")
	pSendMessageW                  = user32.NewProc("SendMessageW")
	pPostQuitMessage               = user32.NewProc("PostQuitMessage")
	pRegisterWindowMessageW        = user32.NewProc("RegisterWindowMessageW")
	pCreatePopupMenu               = user32.NewProc("CreatePopupMenu")
	pAppendMenuW                   = user32.NewProc("AppendMenuW")
	pTrackPopupMenu                = user32.NewProc("TrackPopupMenu")
	pDestroyMenu                   = user32.NewProc("DestroyMenu")
	pSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	pGetCursorPos                  = user32.NewProc("GetCursorPos")
	pLoadImageW                    = user32.NewProc("LoadImageW")
	pGetSystemMetrics              = user32.NewProc("GetSystemMetrics")
	pShowWindow                    = user32.NewProc("ShowWindow")
	pIsIconic                      = user32.NewProc("IsIconic")
	pMessageBeep                   = user32.NewProc("MessageBeep")
	pMessageBoxW                   = user32.NewProc("MessageBoxW")
	pGetWindowRect                 = user32.NewProc("GetWindowRect")
	pSetWindowPos                  = user32.NewProc("SetWindowPos")
	pGetDpiForWindow               = user32.NewProc("GetDpiForWindow")
	pSendMessageTimeoutW           = user32.NewProc("SendMessageTimeoutW")
	pGetClassLongPtrW              = user32.NewProc("GetClassLongPtrW")
	pGetUserDefaultUILanguage      = kernel32.NewProc("GetUserDefaultUILanguage")
	pGetTickCount                  = kernel32.NewProc("GetTickCount")
	pGetModuleHandleW              = kernel32.NewProc("GetModuleHandleW")
	pQueryFullProcessImageNameW    = kernel32.NewProc("QueryFullProcessImageNameW")

	pCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	pSelectObject           = gdi32.NewProc("SelectObject")
	pDeleteObject           = gdi32.NewProc("DeleteObject")
	pDeleteDC               = gdi32.NewProc("DeleteDC")
	pBitBlt                 = gdi32.NewProc("BitBlt")
	pGetDIBits              = gdi32.NewProc("GetDIBits")
	pGetObjectW             = gdi32.NewProc("GetObjectW")
	pCreateDIBSection       = gdi32.NewProc("CreateDIBSection")
	pGdiFlush               = gdi32.NewProc("GdiFlush")

	pShellNotifyIconW   = shell32.NewProc("Shell_NotifyIconW")
	pShellExecuteW      = shell32.NewProc("ShellExecuteW")
	pCommandLineToArgvW = shell32.NewProc("CommandLineToArgvW")

	pGetFileVersionInfoSizeW = versionD.NewProc("GetFileVersionInfoSizeW")
	pGetFileVersionInfoW     = versionD.NewProc("GetFileVersionInfoW")
	pVerQueryValueW          = versionD.NewProc("VerQueryValueW")

	pDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")

	pNtQueryInformationProcess = ntdll.NewProc("NtQueryInformationProcess")

	pGetOpenFileNameW = comdlg32.NewProc("GetOpenFileNameW")

	pWTSQuerySessionInformationW = wtsapi32.NewProc("WTSQuerySessionInformationW")
)

func utf16Ptr(s string) *uint16 {
	p, _ := windows.UTF16PtrFromString(s)
	return p
}

// RECT mirrors the Win32 RECT struct.
type RECT struct{ Left, Top, Right, Bottom int32 }

// POINT mirrors the Win32 POINT struct.
type POINT struct{ X, Y int32 }

// MSG mirrors the Win32 MSG struct.
type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
	_       uint32
}

const (
	WM_NULL            = 0x0000
	WM_DESTROY         = 0x0002
	WM_CLOSE           = 0x0010
	WM_QUERYENDSESSION = 0x0011
	WM_ENDSESSION      = 0x0016
	WM_SETICON         = 0x0080
	WM_COMMAND         = 0x0111
	WM_POWERBROADCAST  = 0x0218
	WM_LBUTTONUP       = 0x0202
	WM_LBUTTONDBLCLK   = 0x0203
	WM_RBUTTONUP       = 0x0205
	WM_CONTEXTMENU     = 0x007B
	WM_USER            = 0x0400
	WM_APP             = 0x8000

	PBT_APMSUSPEND         = 0x4
	PBT_APMRESUMEAUTOMATIC = 0x12
	PBT_APMRESUMESUSPEND   = 0x7
)

// GetMessage pumps one message; ok is false on WM_QUIT.
func GetMessage(m *MSG) bool {
	r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(m)), 0, 0, 0)
	return int32(r) > 0
}

func TranslateDispatch(m *MSG) {
	pTranslateMessage.Call(uintptr(unsafe.Pointer(m)))
	pDispatchMessageW.Call(uintptr(unsafe.Pointer(m)))
}

func PostMessage(hwnd uintptr, msg uint32, wp, lp uintptr) {
	pPostMessageW.Call(hwnd, uintptr(msg), wp, lp)
}

func PostQuit() { pPostQuitMessage.Call(0) }

// SetDPIAware opts the process into per-monitor v2 DPI awareness so window
// geometry and screen captures use physical pixels.
func SetDPIAware() {
	if pSetProcessDpiAwarenessContext.Find() == nil {
		pSetProcessDpiAwarenessContext.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	}
}

// UILanguage returns the primary language id of the user's UI language.
func UILanguage() uint16 {
	r, _, _ := pGetUserDefaultUILanguage.Call()
	return uint16(r) & 0x3ff
}

// SetDarkTitleBar toggles the immersive dark title bar of a window.
func SetDarkTitleBar(hwnd uintptr, dark bool) {
	var v int32
	if dark {
		v = 1
	}
	pDwmSetWindowAttribute.Call(hwnd, 20, uintptr(unsafe.Pointer(&v)), 4)
}

// ShellOpen opens a file, folder or URL with its default handler.
func ShellOpen(target string) {
	verb, t := utf16Ptr("open"), utf16Ptr(target)
	pShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(t)), 0, 0, 1)
}

// RevealInExplorer opens Explorer with the given file selected.
func RevealInExplorer(path string) {
	verb, exe, args := utf16Ptr("open"), utf16Ptr("explorer.exe"), utf16Ptr(`/select,"`+path+`"`)
	pShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(exe)), uintptr(unsafe.Pointer(args)), 0, 1)
}

// Beep plays the default notification sound.
func Beep() { pMessageBeep.Call(0x40) }

// MessageBox shows a modal message box (used for fatal start-up errors).
func MessageBox(title, text string) {
	tx, ti := utf16Ptr(text), utf16Ptr(title)
	pMessageBoxW.Call(0, uintptr(unsafe.Pointer(tx)), uintptr(unsafe.Pointer(ti)), 0x10)
}

// ShowWindowFront restores and focuses a top-level window.
func ShowWindowFront(hwnd uintptr) {
	if r, _, _ := pIsIconic.Call(hwnd); r != 0 {
		pShowWindow.Call(hwnd, 9) // SW_RESTORE
	} else {
		pShowWindow.Call(hwnd, 5) // SW_SHOW
	}
	pSetForegroundWindow.Call(hwnd)
}

// SetWindowIcon assigns big and small icons to a window.
func SetWindowIcon(hwnd, big, small uintptr) {
	pSendMessageW.Call(hwnd, WM_SETICON, 1, big)
	pSendMessageW.Call(hwnd, WM_SETICON, 0, small)
}

// FitScreen shrinks a window size to fit the primary screen.
func FitScreen(w, h int) (int, int) {
	sw, sh := SystemMetric(0), SystemMetric(1)
	if w > sw {
		w = sw
	}
	if h > sh-40 {
		h = sh - 40
	}
	return w, h
}

var pGetDpiForSystem = user32.NewProc("GetDpiForSystem")

// SystemDPI returns the DPI of the primary screen (96 = 100%).
func SystemDPI() int {
	if pGetDpiForSystem.Find() != nil {
		return 96
	}
	if r, _, _ := pGetDpiForSystem.Call(); r != 0 {
		return int(r)
	}
	return 96
}

// LoadAppIcon loads icon resource #1 from the executable at the given size.
func LoadAppIcon(size int) uintptr {
	r, _, _ := pLoadImageW.Call(ModuleHandle(), 1, 1, uintptr(size), uintptr(size), 0)
	return r
}

// ModuleHandle returns the executable's instance handle.
func ModuleHandle() uintptr {
	h, _, _ := pGetModuleHandleW.Call(0)
	return h
}

// SystemMetric wraps GetSystemMetrics.
func SystemMetric(i int) int {
	r, _, _ := pGetSystemMetrics.Call(uintptr(i))
	return int(int32(r))
}

// WindowDPI returns the DPI of the monitor a window is on.
func WindowDPI(hwnd uintptr) int {
	if pGetDpiForWindow.Find() != nil {
		return 96
	}
	r, _, _ := pGetDpiForWindow.Call(hwnd)
	if r == 0 {
		return 96
	}
	return int(r)
}

// PlaceWindow moves and sizes a window, centring it on the primary monitor.
func PlaceWindow(hwnd uintptr, w, h int, maximize bool) {
	w, h = FitScreen(w, h)
	sw, sh := SystemMetric(0), SystemMetric(1)
	pSetWindowPos.Call(hwnd, 0, uintptr((sw-w)/2), uintptr((sh-h)/2), uintptr(w), uintptr(h), 0x0004|0x0010)
	if maximize {
		pShowWindow.Call(hwnd, 3)
	}
}
