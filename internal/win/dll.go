//go:build windows

package win

import "golang.org/x/sys/windows"

// NewLazySystemDLL 只从 System32 加载，避免程序目录被放进同名 DLL。
var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	comctl32 = windows.NewLazySystemDLL("comctl32.dll")

	procRegisterHotKey             = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey           = user32.NewProc("UnregisterHotKey")
	procSendInput                  = user32.NewProc("SendInput")
	procGetAsyncKeyState           = user32.NewProc("GetAsyncKeyState")
	procGetForegroundWindow        = user32.NewProc("GetForegroundWindow")
	procSetWinEventHook            = user32.NewProc("SetWinEventHook")
	procUnhookWinEvent             = user32.NewProc("UnhookWinEvent")
	procGetMessage                 = user32.NewProc("GetMessageW")
	procTranslateMessage           = user32.NewProc("TranslateMessage")
	procDispatchMessage            = user32.NewProc("DispatchMessageW")
	procPeekMessage                = user32.NewProc("PeekMessageW")
	procPostMessage                = user32.NewProc("PostMessageW")
	procPostQuitMessage            = user32.NewProc("PostQuitMessage")
	procCreateWindowEx             = user32.NewProc("CreateWindowExW")
	procDestroyWindow              = user32.NewProc("DestroyWindow")
	procRegisterClassEx            = user32.NewProc("RegisterClassExW")
	procUnregisterClass            = user32.NewProc("UnregisterClassW")
	procDefWindowProc              = user32.NewProc("DefWindowProcW")
	procCreatePopupMenu            = user32.NewProc("CreatePopupMenu")
	procDestroyMenu                = user32.NewProc("DestroyMenu")
	procAppendMenu                 = user32.NewProc("AppendMenuW")
	procTrackPopupMenu             = user32.NewProc("TrackPopupMenu")
	procSetForegroundWindow        = user32.NewProc("SetForegroundWindow")
	procGetCursorPos               = user32.NewProc("GetCursorPos")
	procMessageBeep                = user32.NewProc("MessageBeep")
	procLoadIcon                   = user32.NewProc("LoadIconW")
	procLoadImage                  = user32.NewProc("LoadImageW")
	procGetSystemMetrics           = user32.NewProc("GetSystemMetrics")
	procRegisterWindowMessage      = user32.NewProc("RegisterWindowMessageW")
	procOpenClipboard              = user32.NewProc("OpenClipboard")
	procCloseClipboard             = user32.NewProc("CloseClipboard")
	procEmptyClipboard             = user32.NewProc("EmptyClipboard")
	procGetClipboardData           = user32.NewProc("GetClipboardData")
	procSetClipboardData           = user32.NewProc("SetClipboardData")
	procEnumClipboardFormats       = user32.NewProc("EnumClipboardFormats")
	procGetClipboardSequenceNumber = user32.NewProc("GetClipboardSequenceNumber")
	procRegisterClipboardFormat    = user32.NewProc("RegisterClipboardFormatW")

	procGlobalAlloc     = kernel32.NewProc("GlobalAlloc")
	procGlobalLock      = kernel32.NewProc("GlobalLock")
	procGlobalUnlock    = kernel32.NewProc("GlobalUnlock")
	procGlobalSize      = kernel32.NewProc("GlobalSize")
	procGlobalFree      = kernel32.NewProc("GlobalFree")
	procSetLastError    = kernel32.NewProc("SetLastError")
	procGetModuleHandle = kernel32.NewProc("GetModuleHandleW")

	procShellNotifyIcon = shell32.NewProc("Shell_NotifyIconW")
	procShellExecute    = shell32.NewProc("ShellExecuteW")

	procIsDialogMessage            = user32.NewProc("IsDialogMessageW")
	procSetWindowText              = user32.NewProc("SetWindowTextW")
	procGetWindowText              = user32.NewProc("GetWindowTextW")
	procGetWindowTextLength        = user32.NewProc("GetWindowTextLengthW")
	procSendMessage                = user32.NewProc("SendMessageW")
	procSetFocus                   = user32.NewProc("SetFocus")
	procGetFocus                   = user32.NewProc("GetFocus")
	procGetParent                  = user32.NewProc("GetParent")
	procMoveWindow                 = user32.NewProc("MoveWindow")
	procGetClientRect              = user32.NewProc("GetClientRect")
	procGetDpiForWindow            = user32.NewProc("GetDpiForWindow")
	procSystemParametersInfoForDpi = user32.NewProc("SystemParametersInfoForDpi")
	procSystemParametersInfo       = user32.NewProc("SystemParametersInfoW")
	procMonitorFromWindow          = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfo             = user32.NewProc("GetMonitorInfoW")
	procSetWindowPos               = user32.NewProc("SetWindowPos")
	procShowWindow                 = user32.NewProc("ShowWindow")
	procInvalidateRect             = user32.NewProc("InvalidateRect")
	procAdjustWindowRectExForDpi   = user32.NewProc("AdjustWindowRectExForDpi")
	procAdjustWindowRectEx         = user32.NewProc("AdjustWindowRectEx")
	procGetClassName               = user32.NewProc("GetClassNameW")
	procGetKeyState                = user32.NewProc("GetKeyState")
	procGetNextDlgTabItem          = user32.NewProc("GetNextDlgTabItem")
	procLoadCursor                 = user32.NewProc("LoadCursorW")

	procCreateFontIndirect   = gdi32.NewProc("CreateFontIndirectW")
	procDeleteObject         = gdi32.NewProc("DeleteObject")
	procInitCommonControlsEx = comctl32.NewProc("InitCommonControlsEx")
)

const (
	inputKeyboard  = 1
	keyeventfKeyup = 0x0002
	modNoRepeat    = 0x4000

	wmHotkey       = 0x0312
	wmClose        = 0x0010
	wmNotify       = 0x004E
	wmNull         = 0x0000
	wmRButtonUp    = 0x0205
	wmContextMenu  = 0x007B
	wmApp          = 0x8000
	wmTray         = wmApp + 1
	wmShowNotify   = wmApp + 2
	wmUpdateResult = wmApp + 3
	wmInstallReady = wmApp + 4
	pmNoRemove     = 0x0000
	mbIconWarning  = 0x30
	wsExToolWindow = 0x00000080
	wsExNoActivate = 0x08000000
	wsPopup        = 0x80000000

	eventSystemForeground  = 0x0003
	wineventOutOfContext   = 0x0000
	wineventSkipOwnProcess = 0x0002

	cfUnicodeText    = 13
	cfBitmap         = 2
	cfMetafilePict   = 3
	cfPalette        = 9
	cfEnhMetafile    = 14
	cfOwnerDisplay   = 0x0080
	cfDSPBitmap      = 0x0082
	cfDSPMetafile    = 0x0083
	cfDSPEnhMetafile = 0x008E
	gmemMoveable     = 0x0002

	vkControl  = 0x11
	vkMenu     = 0x12
	vkShift    = 0x10
	vkLControl = 0xA2
	vkRControl = 0xA3
	vkLMenu    = 0xA4
	vkRMenu    = 0xA5
	vkLShift   = 0xA0
	vkRShift   = 0xA1
	vkLWin     = 0x5B
	vkRWin     = 0x5C
	vkA        = 0x41
	vkC        = 0x43
	vkV        = 0x56
	vkReturn   = 0x0D
	// vkE8 是未分配的虚拟键。按住 Alt 收到热键时先敲一下它，
	// 否则用户松开 Alt 时，目标窗口只看到单独的 Alt 弹起，可能把焦点移出输入框。
	vkE8 = 0xE8

	nimAdd        = 0x0
	nimModify     = 0x1
	nimDelete     = 0x2
	nimSetVersion = 0x4
	nifMessage    = 0x1
	nifIcon       = 0x2
	nifTip        = 0x4
	nifInfo       = 0x10
	niifInfo      = 0x1
	niifNoSound   = 0x10
	// notifyIconVersion 用旧版回调：lParam 直接是鼠标消息，处理起来不容易和版本 4 的打包格式搞混。
	notifyIconVersion   = 3
	idiApplication      = 32512
	imageIcon           = 1
	lrShared            = 0x00008000
	smCxSmIcon          = 49
	smCySmIcon          = 50
	mfString            = 0
	mfChecked           = 0x0008
	mfSeparator         = 0x0800
	tpmReturnCmd        = 0x0100
	menuCheckUpdate     = 1
	menuAutoCheck       = 2
	menuExit            = 3
	menuSettings        = 4
	ninBalloonHide      = 0x0403
	ninBalloonTimeout   = 0x0404
	ninBalloonUserClick = 0x0405
	swShowNormal        = 1
	swHide              = 0
	// swShowNA 只显示，不把焦点抢走。切标签时用来露出当前页的控件。
	swShowNA = 8

	tcmFirst       = 0x1300
	tcmGetCurSel   = tcmFirst + 11
	tcmSetCurSel   = tcmFirst + 12
	tcmInsertItemW = tcmFirst + 62
	tcifText       = 0x0001

	// nmClick 和 nmReturn 是 NM_CLICK、NM_RETURN，相对 NM_FIRST 的负值。
	nmClick  = -2
	nmReturn = -4

	hotkeySend = 1
	hotkeyOnly = 2
)

type keybdInput struct {
	Vk        uint16
	Scan      uint16
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

// keyboardEvent 对应 Win32 INPUT。联合体按最大的 MOUSEINPUT 对齐，在 64 位上整个结构是 40 字节。
type keyboardEvent struct {
	Type uint32
	_    uint32
	Ki   keybdInput
	_    [8]byte
}

type point struct {
	X int32
	Y int32
}

type msg struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       point
	LPrivate uint32
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// notifyIconData 对应 NOTIFYICONDATAW。64 位上含 hBalloonIcon 的完整结构是 976 字节。
type notifyIconData struct {
	Size             uint32
	Hwnd             uintptr
	ID               uint32
	Flags            uint32
	CallbackMessage  uint32
	Icon             uintptr
	Tip              [128]uint16
	State            uint32
	StateMask        uint32
	Info             [256]uint16
	TimeoutOrVersion uint32
	InfoTitle        [64]uint16
	InfoFlags        uint32
	Item             guid
	BalloonIcon      uintptr
}

func utf16String(s string) []uint16 {
	u, err := windows.UTF16FromString(s)
	if err != nil || len(u) == 0 {
		return []uint16{0}
	}
	return u
}

func copyUTF16(dst []uint16, s string) {
	if len(dst) == 0 {
		return
	}
	u := utf16String(s)
	if len(u) > len(dst) {
		u = u[:len(dst)-1]
		u = append(u, 0)
	}
	copy(dst, u)
}
