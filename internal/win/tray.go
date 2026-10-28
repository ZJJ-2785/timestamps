//go:build windows

package win

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type notifyIconData struct {
	Size            uint32
	Wnd             uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GuidItem        windows.GUID
	BalloonIcon     uintptr
}

const (
	nimAdd        = 0
	nimModify     = 1
	nimDelete     = 2
	nimSetVersion = 4

	nifMessage = 0x01
	nifIcon    = 0x02
	nifTip     = 0x04
	nifInfo    = 0x10
	nifShowTip = 0x80

	ninSelect           = WM_USER + 0
	ninKeySelect        = WM_USER + 1
	ninBalloonUserClick = WM_USER + 5

	trayCallback = WM_APP + 10
	// MsgInvoke asks the shell window to run queued functions on the UI thread.
	MsgInvoke = WM_APP + 11
)

// MenuItem describes one entry of the tray context menu.
type MenuItem struct {
	ID        int
	Text      string
	Separator bool
	Disabled  bool
	Checked   bool
	Sub       []MenuItem
}

// Shell owns the hidden top-level window that hosts the tray icon and
// receives session, power and shutdown broadcasts.
type Shell struct {
	hwnd         uintptr
	nid          notifyIconData
	taskbarMsg   uint32
	mu           sync.Mutex
	queue        []func()
	OnLeftClick  func()
	OnMenu       func() []MenuItem
	OnCommand    func(id int)
	OnBalloon    func()
	OnSuspend    func()
	OnResume     func()
	OnEndSession func()
}

var shellInst *Shell

var shellProc = windows.NewCallback(func(hwnd, msg, wp, lp uintptr) uintptr {
	s := shellInst
	if s == nil {
		r, _, _ := pDefWindowProcW.Call(hwnd, msg, wp, lp)
		return r
	}
	switch uint32(msg) {
	case trayCallback:
		switch uint32(lp & 0xffff) {
		// With NOTIFYICON_VERSION_4 one click also sends WM_LBUTTONUP (and a
		// double click WM_LBUTTONDBLCLK); NIN_SELECT is the click itself.
		case ninSelect, ninKeySelect:
			if s.OnLeftClick != nil {
				s.OnLeftClick()
			}
		case WM_CONTEXTMENU, WM_RBUTTONUP:
			s.showMenu()
		case ninBalloonUserClick:
			if s.OnBalloon != nil {
				s.OnBalloon()
			}
		}
		return 0
	case MsgInvoke:
		// One call at a time: a call that opens the app window runs the
		// window's own message loop until it closes, and the calls queued
		// behind it then run from that loop (each Invoke posts its own
		// MsgInvoke) instead of waiting for the window to close.
		for {
			s.mu.Lock()
			if len(s.queue) == 0 {
				s.mu.Unlock()
				break
			}
			f := s.queue[0]
			s.queue = s.queue[1:]
			s.mu.Unlock()
			f()
		}
		return 0
	case WM_POWERBROADCAST:
		switch wp {
		case PBT_APMSUSPEND:
			if s.OnSuspend != nil {
				s.OnSuspend()
			}
		case PBT_APMRESUMEAUTOMATIC, PBT_APMRESUMESUSPEND:
			if s.OnResume != nil {
				s.OnResume()
			}
		}
		return 1
	case WM_QUERYENDSESSION:
		return 1
	case WM_ENDSESSION:
		if wp != 0 && s.OnEndSession != nil {
			s.OnEndSession()
		}
		return 0
	}
	if s.taskbarMsg != 0 && uint32(msg) == s.taskbarMsg {
		s.addIcon()
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
})

// NewShell creates the hidden window and tray icon. Must be called on the
// thread that will pump messages.
func NewShell(tip string, icon uintptr) *Shell {
	s := &Shell{}
	shellInst = s
	inst := windows.Handle(ModuleHandle())
	cls := utf16Ptr("TimestampsShell")
	wc := wndClassEx{Size: uint32(unsafe.Sizeof(wndClassEx{})), WndProc: shellProc, Instance: inst, ClassName: cls}
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	title := utf16Ptr("Timestamps")
	// WS_EX_TOOLWINDOW | WS_EX_NOACTIVATE: never shown in Alt+Tab or activated.
	s.hwnd, _, _ = pCreateWindowExW.Call(0x80|0x08000000, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(title)), 0, 0, 0, 0, 0, 0, 0, uintptr(inst), 0)
	tb := utf16Ptr("TaskbarCreated")
	r, _, _ := pRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(tb)))
	s.taskbarMsg = uint32(r)

	s.nid.Size = uint32(unsafe.Sizeof(s.nid))
	s.nid.Wnd = s.hwnd
	s.nid.ID = 1
	s.nid.Flags = nifMessage | nifIcon | nifTip | nifShowTip
	s.nid.CallbackMessage = trayCallback
	s.nid.Icon = icon
	copyUTF16(s.nid.Tip[:], tip)
	s.addIcon()
	return s
}

func (s *Shell) addIcon() {
	pShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&s.nid)))
	s.nid.Version = 4
	pShellNotifyIconW.Call(nimSetVersion, uintptr(unsafe.Pointer(&s.nid)))
}

// HWND returns the hidden window handle.
func (s *Shell) HWND() uintptr { return s.hwnd }

// Invoke runs f on the UI thread (asynchronously).
func (s *Shell) Invoke(f func()) {
	s.mu.Lock()
	s.queue = append(s.queue, f)
	s.mu.Unlock()
	PostMessage(s.hwnd, MsgInvoke, 0, 0)
}

// SetTip updates the tray tooltip.
func (s *Shell) SetTip(tip string) {
	s.Invoke(func() {
		s.nid.Flags = nifMessage | nifIcon | nifTip | nifShowTip
		for i := range s.nid.Tip {
			s.nid.Tip[i] = 0
		}
		copyUTF16(s.nid.Tip[:], tip)
		pShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&s.nid)))
	})
}

// SetIcon replaces the tray icon. Call it on the UI thread (see Invoke).
func (s *Shell) SetIcon(icon uintptr) {
	s.nid.Icon = icon
	s.nid.Flags = nifMessage | nifIcon | nifTip | nifShowTip
	pShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&s.nid)))
}

// Notify shows a balloon / toast from the tray icon.
func (s *Shell) Notify(title, text string) {
	s.Invoke(func() {
		s.nid.Flags = nifInfo | nifMessage | nifIcon | nifTip | nifShowTip
		for i := range s.nid.Info {
			s.nid.Info[i] = 0
		}
		for i := range s.nid.InfoTitle {
			s.nid.InfoTitle[i] = 0
		}
		copyUTF16(s.nid.Info[:], text)
		copyUTF16(s.nid.InfoTitle[:], title)
		s.nid.InfoFlags = 0x04 | 0x20 // NIIF_USER | NIIF_LARGE_ICON
		pShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&s.nid)))
		s.nid.Flags = nifMessage | nifIcon | nifTip | nifShowTip
	})
}

// Remove deletes the tray icon.
func (s *Shell) Remove() {
	pShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&s.nid)))
}

func buildMenu(items []MenuItem) uintptr {
	m, _, _ := pCreatePopupMenu.Call()
	for _, it := range items {
		if it.Separator {
			pAppendMenuW.Call(m, 0x800, 0, 0)
			continue
		}
		text := utf16Ptr(it.Text)
		flags := uintptr(0)
		if it.Disabled {
			flags |= 0x3
		}
		if it.Checked {
			flags |= 0x8
		}
		if len(it.Sub) > 0 {
			sub := buildMenu(it.Sub)
			pAppendMenuW.Call(m, flags|0x10, sub, uintptr(unsafe.Pointer(text)))
			continue
		}
		pAppendMenuW.Call(m, flags, uintptr(it.ID), uintptr(unsafe.Pointer(text)))
	}
	return m
}

func (s *Shell) showMenu() {
	if s.OnMenu == nil {
		return
	}
	m := buildMenu(s.OnMenu())
	defer pDestroyMenu.Call(m)
	var pt POINT
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(s.hwnd)
	id, _, _ := pTrackPopupMenu.Call(m, 0x0100|0x0080|0x0002, uintptr(pt.X), uintptr(pt.Y), 0, s.hwnd, 0)
	PostMessage(s.hwnd, WM_NULL, 0, 0)
	if id != 0 && s.OnCommand != nil {
		s.OnCommand(int(id))
	}
}

func copyUTF16(dst []uint16, s string) {
	u, _ := windows.UTF16FromString(s)
	if len(u) > len(dst) {
		u = u[:len(dst)-1]
		u = append(u, 0)
	}
	copy(dst, u)
}
