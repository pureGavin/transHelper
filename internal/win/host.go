//go:build windows

package win

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"transhelper/internal/app"
	"transhelper/internal/config"
	"transhelper/internal/hotkey"
)

// 不用低级键盘钩子 WH_KEYBOARD_LL。它有超时限制，也容易被杀毒软件当成键盘记录器。
// 热键只在白名单进程处于前台时注册，其他程序完全不受影响。

var (
	wndProcCallback  = windows.NewCallback(wndProc)
	winEventCallback = windows.NewCallback(winEventProc)
	globalHost       *host
)

type host struct {
	cfg     config.Config
	logger  *log.Logger
	ctx     context.Context
	cancel  context.CancelFunc
	desktop *Desktop
	runner  *app.Runner

	hwnd             uintptr
	instance         windows.Handle
	className        []uint16
	classAtom        uint16
	hook             uintptr
	icon             uintptr
	taskbarMsg       uint32
	trayAdded        bool
	registered       bool
	conflictNotified bool
	busy             atomic.Bool

	notifyMu sync.Mutex
	pending  []string
}

// Run 创建隐藏窗口、托盘和热键，直到用户退出。
func Run(parent context.Context, cfg config.Config, translator app.Translator, logger *log.Logger) error {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	h := &host{
		cfg:     cfg,
		logger:  logger,
		ctx:     ctx,
		cancel:  cancel,
		desktop: &Desktop{},
	}
	h.runner = &app.Runner{
		Desktop:    h.desktop,
		Translator: translator,
		Notifier:   h,
		Timeout:    cfg.Timeout,
		Log:        logger,
	}
	// 窗口过程在 CreateWindowEx 返回前就会被调用，所以要先放好。
	globalHost = h
	defer func() { globalHost = nil }()

	if err := h.start(); err != nil {
		h.cleanup()
		return err
	}
	go h.watchInterrupt()
	h.loop()
	cancel()
	h.waitBusy(3 * time.Second)
	h.cleanup()
	h.logger.Printf("已退出")
	return nil
}

func (h *host) start() error {
	if err := h.createWindow(); err != nil {
		return err
	}
	h.icon = loadAppIcon()
	h.taskbarMsg = registerMessage("TaskbarCreated")
	h.addTray()
	if err := h.installHook(); err != nil {
		return err
	}
	h.syncHotkeys()
	h.logger.Printf("已启动，前台是白名单进程时热键才会生效")
	return nil
}

func (h *host) createWindow() error {
	r, _, err := procGetModuleHandle.Call(0)
	if r == 0 {
		return fmt.Errorf("GetModuleHandle: %w", err)
	}
	h.instance = windows.Handle(r)
	h.className = utf16String("transhelper_message")
	title := utf16String("transHelper")

	wc := wndClassEx{
		Size:      uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:   wndProcCallback,
		Instance:  h.instance,
		ClassName: &h.className[0],
	}
	atom, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	runtime.KeepAlive(&wc)
	runtime.KeepAlive(h.className)
	if atom == 0 {
		return fmt.Errorf("RegisterClassEx: %w", err)
	}
	h.classAtom = uint16(atom)

	// 不用 HWND_MESSAGE。纯消息窗口收不到广播，资源管理器重启时的 TaskbarCreated 就是广播。
	hwnd, _, err := procCreateWindowEx.Call(
		wsExToolWindow|wsExNoActivate,
		uintptr(unsafe.Pointer(&h.className[0])),
		uintptr(unsafe.Pointer(&title[0])),
		wsPopup,
		0, 0, 0, 0,
		0, 0,
		uintptr(h.instance),
		0,
	)
	runtime.KeepAlive(h.className)
	runtime.KeepAlive(title)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowEx: %w", err)
	}
	h.hwnd = hwnd
	return nil
}

func (h *host) installHook() error {
	r, _, err := procSetWinEventHook.Call(
		eventSystemForeground,
		eventSystemForeground,
		0,
		winEventCallback,
		0, 0,
		wineventOutOfContext|wineventSkipOwnProcess,
	)
	if r == 0 {
		return fmt.Errorf("SetWinEventHook: %w", err)
	}
	h.hook = r
	return nil
}

func (h *host) loop() {
	for {
		var m msg
		r, _, err := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		runtime.KeepAlive(&m)
		switch int32(r) {
		case 0:
			return
		case -1:
			h.logger.Printf("GetMessage 失败: %v", err)
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
		runtime.KeepAlive(&m)
	}
}

func (h *host) watchInterrupt() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	defer signal.Stop(ch)
	select {
	case <-ch:
		h.cancel()
		if h.hwnd != 0 {
			procPostMessage.Call(h.hwnd, wmClose, 0, 0)
		}
	case <-h.ctx.Done():
	}
}

func (h *host) waitBusy(d time.Duration) {
	deadline := time.Now().Add(d)
	for h.busy.Load() && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if h.busy.Load() {
		h.logger.Printf("退出时翻译仍在进行，剪贴板可能没有还原")
	}
}

func (h *host) cleanup() {
	h.unregisterHotkeys()
	h.registered = false
	if h.hook != 0 {
		procUnhookWinEvent.Call(h.hook)
		h.hook = 0
	}
	h.deleteTray()
	if h.hwnd != 0 {
		procDestroyWindow.Call(h.hwnd)
		h.hwnd = 0
	}
	if h.classAtom != 0 && len(h.className) > 0 {
		procUnregisterClass.Call(uintptr(unsafe.Pointer(&h.className[0])), uintptr(h.instance))
		runtime.KeepAlive(h.className)
		h.classAtom = 0
	}
}

func wndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	h := globalHost
	if h == nil {
		r, _, _ := procDefWindowProc.Call(hwnd, message, wParam, lParam)
		return r
	}
	switch uint32(message) {
	case wmHotkey:
		h.onHotkey(wParam)
		return 0
	case wmTray:
		if lParam == wmRButtonUp || lParam == wmContextMenu {
			h.showMenu()
		}
		return 0
	case wmShowNotify:
		h.flushNotify()
		return 0
	case wmClose:
		h.cancel()
		procPostQuitMessage.Call(0)
		return 0
	}
	if h.taskbarMsg != 0 && uint32(message) == h.taskbarMsg {
		h.addTray()
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, message, wParam, lParam)
	return r
}

func winEventProc(hook, event, hwnd, idObject, idChild, eventThread, eventTime uintptr) uintptr {
	if globalHost != nil {
		globalHost.syncHotkeys()
	}
	return 0
}

func (h *host) onHotkey(id uintptr) {
	// 即使这次不处理，也要先打断 Alt 或 Win，避免松开时激活菜单或开始菜单。
	maskAltWin()
	if h.ctx.Err() != nil {
		return
	}
	if !h.busy.CompareAndSwap(false, true) {
		h.logger.Printf("上一次翻译还没结束，忽略本次热键")
		return
	}
	h.desktop.Expect = currentForeground()
	action := app.ActionTranslateOnly
	which := "只翻译"
	if id == hotkeySend {
		action = app.ActionTranslateAndSend
		which = "翻译并发送"
	}
	h.logger.Printf("热键触发：%s", which)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer h.busy.Store(false)
		ensureMessageQueue()
		h.runner.Run(h.ctx, action)
	}()
}

func (h *host) syncHotkeys() {
	name := foregroundProcessName()
	want := name != "" && h.cfg.AllowsProcess(name)
	if want == h.registered {
		return
	}
	if want {
		if err := h.registerHotkeys(); err != nil {
			if !h.conflictNotified {
				h.conflictNotified = true
				h.logger.Printf("注册热键失败: %v", err)
				h.Notify("热键被其他程序占用，请换一组或关掉占用它的程序")
			}
			return
		}
		h.registered = true
		h.conflictNotified = false
		h.logger.Printf("前台是 %s，已注册热键", name)
		return
	}
	h.unregisterHotkeys()
	h.registered = false
	if name == "" {
		h.logger.Printf("前台进程无法识别，已注销热键")
		return
	}
	h.logger.Printf("前台是 %s，已注销热键", name)
}

func (h *host) registerHotkeys() error {
	if err := registerOne(h.hwnd, hotkeySend, h.cfg.TranslateAndSend); err != nil {
		return fmt.Errorf("翻译并发送（%s）: %w", h.cfg.TranslateAndSend, err)
	}
	if err := registerOne(h.hwnd, hotkeyOnly, h.cfg.TranslateOnly); err != nil {
		unregisterOne(h.hwnd, hotkeySend)
		return fmt.Errorf("只翻译（%s）: %w", h.cfg.TranslateOnly, err)
	}
	return nil
}

func registerOne(hwnd uintptr, id uintptr, combo hotkey.Combo) error {
	r, _, err := procRegisterHotKey.Call(hwnd, id, uintptr(combo.Mod|modNoRepeat), uintptr(combo.VK))
	if r != 0 {
		return nil
	}
	if err == nil || errorsIsSuccess(err) {
		return fmt.Errorf("RegisterHotKey 失败")
	}
	return err
}

func (h *host) unregisterHotkeys() {
	unregisterOne(h.hwnd, hotkeySend)
	unregisterOne(h.hwnd, hotkeyOnly)
}

func unregisterOne(hwnd, id uintptr) {
	procUnregisterHotKey.Call(hwnd, id)
}

func (h *host) Notify(message string) {
	h.notifyMu.Lock()
	h.pending = append(h.pending, message)
	h.notifyMu.Unlock()
	if h.hwnd != 0 {
		procPostMessage.Call(h.hwnd, wmShowNotify, 0, 0)
	}
}

func (h *host) flushNotify() {
	h.notifyMu.Lock()
	items := h.pending
	h.pending = nil
	h.notifyMu.Unlock()
	for _, item := range items {
		h.balloon(item)
		procMessageBeep.Call(mbIconWarning)
	}
}

func (h *host) baseNID() notifyIconData {
	return notifyIconData{
		Size: uint32(unsafe.Sizeof(notifyIconData{})),
		Hwnd: h.hwnd,
		ID:   1,
	}
}

func (h *host) addTray() {
	if h.hwnd == 0 {
		return
	}
	if h.trayAdded {
		h.deleteTray()
	}
	nid := h.baseNID()
	nid.Flags = nifMessage | nifIcon | nifTip
	nid.CallbackMessage = wmTray
	nid.Icon = h.icon
	copyUTF16(nid.Tip[:], "transHelper")
	r, _, err := procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	runtime.KeepAlive(&nid)
	if r == 0 {
		h.logger.Printf("添加托盘图标失败: %v", err)
		h.trayAdded = false
		return
	}
	h.trayAdded = true
	version := h.baseNID()
	version.TimeoutOrVersion = notifyIconVersion
	procShellNotifyIcon.Call(nimSetVersion, uintptr(unsafe.Pointer(&version)))
	runtime.KeepAlive(&version)
}

func (h *host) deleteTray() {
	if !h.trayAdded || h.hwnd == 0 {
		h.trayAdded = false
		return
	}
	nid := h.baseNID()
	procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	runtime.KeepAlive(&nid)
	h.trayAdded = false
}

func (h *host) balloon(message string) {
	if !h.trayAdded {
		return
	}
	nid := h.baseNID()
	nid.Flags = nifInfo
	nid.InfoFlags = niifInfo | niifNoSound
	copyUTF16(nid.Info[:], message)
	copyUTF16(nid.InfoTitle[:], "transHelper")
	procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
	runtime.KeepAlive(&nid)
}

func (h *host) showMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	label := utf16String("退出")
	procAppendMenu.Call(menu, mfString, menuExit, uintptr(unsafe.Pointer(&label[0])))
	runtime.KeepAlive(label)

	// 菜单不先把窗口拉到前台的话，点别处不会自动关掉。点完再发一个空消息，是 Win32 菜单的老毛病。
	procSetForegroundWindow.Call(h.hwnd)
	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	runtime.KeepAlive(&pt)
	cmd, _, _ := procTrackPopupMenu.Call(menu, tpmReturnCmd, uintptr(pt.X), uintptr(pt.Y), 0, h.hwnd, 0)
	procPostMessage.Call(h.hwnd, wmNull, 0, 0)
	if cmd == menuExit {
		h.cancel()
		procPostQuitMessage.Call(0)
	}
}

func loadAppIcon() uintptr {
	r, _, _ := procLoadIcon.Call(0, idiApplication)
	return r
}

func registerMessage(name string) uint32 {
	u := utf16String(name)
	r, _, _ := procRegisterWindowMessage.Call(uintptr(unsafe.Pointer(&u[0])))
	runtime.KeepAlive(u)
	return uint32(r)
}

func errorsIsSuccess(err error) bool {
	return err == nil || err == windows.ERROR_SUCCESS
}
