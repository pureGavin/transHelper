//go:build windows && uidrive

package win

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"transhelper/internal/config"
)

const (
	driveTimeout = 5 * time.Second
	pollGap      = 30 * time.Millisecond

	bmClick           = 0x00F5
	wmSetText         = 0x000C
	wmGetText         = 0x000D
	wmGetTextLength   = 0x000E
	emGetPasswordChar = 0x00D2
	// TCM_SETCURFOCUS 是 TCM_FIRST + 48。加 49 会变成 TCM_SETMINTABWIDTH，标签不会切换。
	tcmSetCurFocus  = tcmFirst + 48
	smtoAbortIfHung = 0x0002
	sendTimeoutMS   = 2000
	stillActive     = 259
	dialogStaticID  = 0xFFFF
	buildTimeout    = 3 * time.Minute
)

var (
	procEnumChildWindows   = user32.NewProc("EnumChildWindows")
	procGetDlgCtrlID       = user32.NewProc("GetDlgCtrlID")
	procSendMessageTimeout = user32.NewProc("SendMessageTimeoutW")

	enumCB      = windows.NewCallback(enumTop)
	enumChildCB = windows.NewCallback(enumChild)
	enumMu      sync.Mutex
	enumPID     uint32
	enumClass   string
	enumHits    []uintptr

	helperOnce sync.Once
	helperExe  string
	helperErr  error
)

type controlSpec struct {
	name   string
	id     uintptr
	secret bool
	combo  []string
	list   bool
}

// 名字和设置窗口里的控件 ID 绑在一起。ID 改了这里会编译失败。
var controlSpecs = []controlSpec{
	{name: "apply", id: idApply},
	{name: "cancel", id: idCancel},
	{name: "url", id: idURL},
	{name: "model", id: idModel},
	{name: "key", id: idKey, secret: true},
	{name: "showKey", id: idShowKey},
	{name: "timeout", id: idTimeout},
	{name: "theme", id: idTheme, combo: []string{config.ThemeSystem, config.ThemeLight, config.ThemeDark}},
	{name: "test", id: idTest},
	{name: "testStatus", id: idTestStatus},
	{name: "tabs", id: idTabs},
	{name: "link", id: idLink},
	{name: "proxyOn", id: idProxyOn},
	{name: "proxyType", id: idProxyType, combo: []string{"http", "socks5"}},
	{name: "proxyHost", id: idProxyHost},
	{name: "proxyPort", id: idProxyPort},
	{name: "proxyAuth", id: idProxyAuth},
	{name: "proxyUser", id: idProxyUser},
	{name: "proxyPass", id: idProxyPass, secret: true},
	{name: "proxyModel", id: idProxyModel},
	{name: "procApps", id: idProcApps, list: true},
	{name: "procEnabled", id: idProcEnabled, list: true},
	{name: "procAdd", id: idProcAdd},
	{name: "procRemove", id: idProcRemove},
}

var menuByName = map[string]uintptr{
	"settings":     menuSettings,
	"check-update": menuCheckUpdate,
	"auto-check":   menuAutoCheck,
}

var pageNames = []string{"general", "processes", "network", "about"}

// App 是一个正在运行的 transHelper。测试按控件 ID 给它发消息，不点屏幕。
type App struct {
	PID        int
	ConfigPath string
	LogPath    string

	cmd      *exec.Cmd
	waitOnce sync.Once
	waitErr  error
}

// UIState 是设置窗口当前能读到的状态。
// 密码框的正文不会放进来。Windows 会拦住别的进程读取密码框，所以长度也经常是 0。
type UIState struct {
	PID           int                     `json:"pid"`
	SettingsOpen  bool                    `json:"settingsOpen"`
	SettingsCount int                     `json:"settingsCount"`
	SettingsHWND  string                  `json:"settingsHwnd,omitempty"`
	Width         int                     `json:"width,omitempty"`
	Height        int                     `json:"height,omitempty"`
	Page          string                  `json:"page,omitempty"`
	ScrollPos     int                     `json:"scrollPos"`
	ScrollMax     int                     `json:"scrollMax"`
	ScrollPage    int                     `json:"scrollPage"`
	CanScroll     bool                    `json:"canScroll"`
	Controls      map[string]ControlState `json:"controls,omitempty"`
}

// ControlState 是一个控件的文字、勾选和是否能操作。
type ControlState struct {
	Text         string   `json:"text,omitempty"`
	TextLen      int      `json:"textLen"`
	Checked      bool     `json:"checked"`
	Enabled      bool     `json:"enabled"`
	Visible      bool     `json:"visible"`
	PasswordChar *int     `json:"passwordChar,omitempty"`
	Value        string   `json:"value,omitempty"`
	Items        []string `json:"items,omitempty"`
	Selected     string   `json:"selected,omitempty"`
	Top          int      `json:"top,omitempty"`
}

// DialogState 是设置窗口弹出的提示框。
type DialogState struct {
	Open bool   `json:"open"`
	Text string `json:"text,omitempty"`
}

type sessionDisk struct {
	PID    int    `json:"pid"`
	Config string `json:"config"`
	Log    string `json:"log"`
}

// Start 编译带 uidrive 标签的 transHelper，写一份测试配置并启动。
// 锁名和用户正在运行的实例分开，所以不会被「已在运行」挡住。
func Start() (*App, error) {
	root, err := ModuleRoot()
	if err != nil {
		return nil, err
	}
	exe, err := ensureHelper(root)
	if err != nil {
		return nil, err
	}
	id := strconv.FormatInt(time.Now().UnixNano(), 36)
	dir := filepath.Join(root, ".tools", "tmp", "uidrive", "run-"+id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	cfgPath := filepath.Join(dir, config.FileName)
	if err := config.Save(cfgPath, config.Default()); err != nil {
		return nil, err
	}
	logPath := filepath.Join(dir, "transhelper.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}
	lock := fmt.Sprintf("uidrive-%d-%s", os.Getpid(), id)
	cmd := exec.Command(exe, "-config", cfgPath)
	cmd.Dir = dir
	cmd.Env = setEnv(os.Environ(), "TRANSHELPER_INSTANCE", lock)
	cmd.Stderr = logFile
	cmd.Stdout = logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("启动测试实例: %w", err)
	}
	// 子进程已经继承句柄，这里关掉父进程这份，避免占着删除。
	logFile.Close()
	app := &App{PID: cmd.Process.Pid, ConfigPath: cfgPath, LogPath: logPath, cmd: cmd}
	if err := poll(driveTimeout, func() bool {
		_, findErr := app.hidden()
		return findErr == nil
	}); err != nil {
		_ = app.Stop()
		return nil, fmt.Errorf("测试实例没有出现窗口: %w", err)
	}
	return app, nil
}

// Attach 接到已经在运行的 transHelper。配置文件按 exe 同目录查找，找不到就留空。
func Attach(pid int) (*App, error) {
	if pid <= 0 {
		return nil, fmt.Errorf("pid 无效")
	}
	if !processAlive(pid) {
		return nil, fmt.Errorf("进程 %d 不在运行", pid)
	}
	app := &App{PID: pid}
	if exe, err := processExe(pid); err == nil {
		path := config.DefaultPath(exe)
		if _, statErr := os.Stat(path); statErr == nil {
			app.ConfigPath = path
		}
	}
	if _, err := app.hidden(); err != nil {
		return nil, err
	}
	return app, nil
}

// ModuleRoot 从当前目录或程序所在目录向上找 go.mod。
func ModuleRoot() (string, error) {
	var starts []string
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	for _, start := range starts {
		dir := start
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return dir, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", fmt.Errorf("找不到项目根目录，请在仓库里运行")
}

// SessionPath 是命令行两次调用之间记住进程的文件。
func SessionPath(root string) string {
	return filepath.Join(root, ".tools", "tmp", "uidrive", "session.json")
}

// SaveSession 把当前进程记下来，供下一次命令行调用。
func SaveSession(root string, app *App) error {
	if err := os.MkdirAll(filepath.Dir(SessionPath(root)), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(sessionDisk{PID: app.PID, Config: app.ConfigPath, Log: app.LogPath}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(SessionPath(root), data, 0o644)
}

// OpenSession 读上一次 start 或 attach 记下的进程。
func OpenSession(root string) (*App, error) {
	data, err := os.ReadFile(SessionPath(root))
	if err != nil {
		return nil, fmt.Errorf("没有会话，先运行 start 或 attach")
	}
	var disk sessionDisk
	if err := json.Unmarshal(data, &disk); err != nil {
		return nil, fmt.Errorf("会话文件损坏: %w", err)
	}
	if !processAlive(disk.PID) {
		return nil, fmt.Errorf("会话里的进程 %d 已经退出", disk.PID)
	}
	app := &App{PID: disk.PID, ConfigPath: disk.Config, LogPath: disk.Log}
	if _, err := app.hidden(); err != nil {
		return nil, err
	}
	return app, nil
}

// Stop 让托盘程序退出。自己启动的进程超时后会被结束。
func (a *App) Stop() error {
	if a == nil || a.PID == 0 {
		return nil
	}
	if processAlive(a.PID) {
		if hwnd, err := a.hidden(); err == nil {
			_ = postMessage(hwnd, wmClose, 0, 0)
		}
	}
	err := a.WaitExit(driveTimeout)
	if err != nil && a.cmd != nil && a.cmd.Process != nil {
		_ = a.cmd.Process.Kill()
		_ = a.WaitExit(2 * time.Second)
	}
	return err
}

// WaitExit 等到进程退出。
func (a *App) WaitExit(timeout time.Duration) error {
	if a.cmd != nil {
		return a.waitCmd(timeout)
	}
	if err := poll(timeout, func() bool { return !processAlive(a.PID) }); err != nil {
		return fmt.Errorf("进程 %d 没有退出", a.PID)
	}
	return nil
}

// Menu 投递托盘菜单命令。name 是 settings、check-update 或 auto-check。
func (a *App) Menu(name string) error {
	id, ok := menuByName[name]
	if !ok {
		return fmt.Errorf("不认识的菜单 %s", name)
	}
	hwnd, err := a.hidden()
	if err != nil {
		return err
	}
	return postMessage(hwnd, wmCommand, id, 0)
}

// BalloonClick 相当于点一下当前托盘通知。
func (a *App) BalloonClick() error {
	hwnd, err := a.hidden()
	if err != nil {
		return err
	}
	return postMessage(hwnd, wmTray, 0, uintptr(ninBalloonUserClick))
}

// Close 关掉设置窗口，不保存。
func (a *App) Close() error {
	hwnd, err := a.requireSettings()
	if err != nil {
		return err
	}
	return postMessage(hwnd, wmClose, 0, 0)
}

// Set 把输入框写成 text。
func (a *App) Set(name, text string) error {
	spec, hwnd, err := a.control(name)
	if err != nil {
		return err
	}
	if spec.combo != nil {
		return fmt.Errorf("%s 是下拉框，用 select", name)
	}
	result, err := writeWindowText(hwnd, text)
	if err != nil {
		return err
	}
	if result == 0 {
		return fmt.Errorf("%s 没有接受文字", name)
	}
	// 密码框会拒绝别的进程读取，读回来是空的，不代表没写进去。
	if spec.secret {
		return nil
	}
	if got := readWindowText(hwnd); got != text {
		return fmt.Errorf("写入 %s 后读到 %q", name, got)
	}
	return nil
}

// Click 投递 BM_CLICK。用投递而不是发送，是因为「应用」可能弹出模态框，发送会把驱动卡住。
func (a *App) Click(name string) error {
	_, hwnd, err := a.control(name)
	if err != nil {
		return err
	}
	if !windowEnabled(hwnd) {
		return fmt.Errorf("%s 当前不可用", name)
	}
	return postMessage(hwnd, bmClick, 0, 0)
}

// Select 选中下拉项，并补发 CBN_SELCHANGE。theme 用 system、light、dark，proxyType 用 http、socks5。
func (a *App) Select(name, value string) error {
	spec, hwnd, err := a.control(name)
	if err != nil {
		return err
	}
	if spec.combo == nil {
		return fmt.Errorf("%s 不是下拉框", name)
	}
	index := -1
	for i, item := range spec.combo {
		if strings.EqualFold(item, value) {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("%s 没有选项 %s", name, value)
	}
	if _, err := sendMessage(hwnd, cbSetCurSel, uintptr(index), 0); err != nil {
		return err
	}
	settings, err := a.requireSettings()
	if err != nil {
		return err
	}
	// 父窗口是内容区，它会把命令再投递给设置窗口。直接投给设置窗口，主题预览能马上发生。
	return postMessage(settings, wmCommand, uintptr(spec.id)|(uintptr(cbnSelChange)<<16), hwnd)
}

// Pick 选中列表里的一项，并通知设置窗口更新箭头按钮。
func (a *App) Pick(name, item string) error {
	spec, hwnd, err := a.control(name)
	if err != nil {
		return err
	}
	if !spec.list {
		return fmt.Errorf("%s 不是列表", name)
	}
	items, _ := readListBox(hwnd)
	index := -1
	for i, text := range items {
		if strings.EqualFold(text, item) {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("%s 里没有 %s", name, item)
	}
	if _, err := sendMessage(hwnd, lbSetCurSel, uintptr(index), 0); err != nil {
		return err
	}
	settings, err := a.requireSettings()
	if err != nil {
		return err
	}
	return postMessage(settings, wmCommand, uintptr(spec.id)|(uintptr(lbnSelChange)<<16), hwnd)
}

// Tab 切换标签。用 TCM_SETCURFOCUS，让控件在自己进程里发出选中通知。
// 跨进程的 WM_NOTIFY 带的是指针，系统不会帮忙拷贝，所以不能从外面直接发。
func (a *App) Tab(name string) error {
	index := -1
	for i, page := range pageNames {
		if page == name {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("不认识的标签 %s", name)
	}
	_, hwnd, err := a.control("tabs")
	if err != nil {
		return err
	}
	if _, err := sendMessage(hwnd, tcmSetCurFocus, uintptr(index), 0); err != nil {
		return err
	}
	return nil
}

// Key 向设置窗口投递 Esc 或 Enter。Ctrl+Tab 读的是真实键盘状态，投递消息模拟不了。
func (a *App) Key(name string) error {
	hwnd, err := a.requireSettings()
	if err != nil {
		return err
	}
	var vk uintptr
	switch strings.ToLower(name) {
	case "esc", "escape":
		vk = vkEscape
	case "enter", "return":
		vk = vkReturn
	default:
		return fmt.Errorf("不认识的按键 %s", name)
	}
	return postMessage(hwnd, wmKeyDown, vk, 0)
}

// Resize 改变设置窗口高度。height 小于最小高度时，窗口会停在最小高度。
func (a *App) Resize(height int) error {
	hwnd, err := a.requireSettings()
	if err != nil {
		return err
	}
	var rc rect
	ok, _, callErr := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	runtime.KeepAlive(&rc)
	if ok == 0 {
		return fmt.Errorf("读取窗口大小: %v", callErr)
	}
	if height <= 0 {
		height = 1
	}
	flags := uintptr(swpNoMove | swpNoZOrder | swpNoActivate)
	r, _, callErr := procSetWindowPos.Call(hwnd, 0, 0, 0, uintptr(rc.Right-rc.Left), uintptr(height), flags)
	if r == 0 {
		return fmt.Errorf("调整窗口大小: %v", callErr)
	}
	return nil
}

// Wheel 滚动设置窗口。正数向下，负数向上。一格是系统的 120。
func (a *App) Wheel(notches int) error {
	hwnd, err := a.requireSettings()
	if err != nil {
		return err
	}
	return postWheel(hwnd, notches, false)
}

// WheelOn 把滚轮投给指定列表。正数向下。坐标放在屏幕外，按这个列表处理，不看窗口摆在哪。
func (a *App) WheelOn(name string, notches int) error {
	spec, hwnd, err := a.control(name)
	if err != nil {
		return err
	}
	if !spec.list {
		return fmt.Errorf("%s 不是列表", name)
	}
	return postWheel(hwnd, notches, true)
}

func postWheel(hwnd uintptr, notches int, offscreen bool) error {
	if notches == 0 {
		return nil
	}
	if notches > 20 {
		notches = 20
	}
	if notches < -20 {
		notches = -20
	}
	// 正的滚轮增量会把内容往上移。要让滚动位置变大，得投一个负增量。
	delta := int16(-notches * wheelDelta)
	wparam := uintptr(uint32(uint16(delta)) << 16)
	var lParam uintptr
	if offscreen {
		// 落在屏幕外。常量转 uint16 会被拒绝，所以先写成变量。
		off := int16(-32000)
		lParam = uintptr(uint16(off)) | uintptr(uint16(off))<<16
	}
	return postMessage(hwnd, wmMouseWheel, wparam, lParam)
}

// State 读取设置窗口。没打开时 SettingsOpen 为 false。
func (a *App) State() (UIState, error) {
	state := UIState{PID: a.PID}
	if !processAlive(a.PID) {
		return state, fmt.Errorf("进程 %d 已经退出", a.PID)
	}
	list := a.settingsList()
	state.SettingsCount = len(list)
	if len(list) == 0 {
		return state, nil
	}
	hwnd := list[0]
	state.SettingsOpen = true
	state.SettingsHWND = strconv.FormatUint(uint64(hwnd), 10)
	var rc rect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	runtime.KeepAlive(&rc)
	state.Width = int(rc.Right - rc.Left)
	state.Height = int(rc.Bottom - rc.Top)
	if tabs := findChild(hwnd, idTabs); tabs != 0 {
		got, _ := sendMessage(tabs, tcmGetCurSel, 0, 0)
		if int(got) >= 0 && int(got) < len(pageNames) {
			state.Page = pageNames[got]
		}
	}
	if body := findClass(hwnd, "transhelper_settings_body"); body != 0 {
		info := scrollInfo{Size: uint32(unsafe.Sizeof(scrollInfo{})), Mask: sifRange | sifPage | sifPos}
		procGetScrollInfo.Call(body, sbVert, uintptr(unsafe.Pointer(&info)))
		runtime.KeepAlive(&info)
		state.ScrollPos = int(info.Pos)
		state.ScrollMax = int(info.Max)
		state.ScrollPage = int(info.Page)
		state.CanScroll = info.Page > 0 && int(info.Max) >= int(info.Page)
	}
	state.Controls = make(map[string]ControlState, len(controlSpecs))
	for _, spec := range controlSpecs {
		child := findChild(hwnd, spec.id)
		if child == 0 {
			continue
		}
		state.Controls[spec.name] = readControl(spec, child)
	}
	return state, nil
}

// Dialog 读取设置窗口拥有的提示框。没有时 Open 为 false。
func (a *App) Dialog() (DialogState, error) {
	var none DialogState
	if !processAlive(a.PID) {
		return none, fmt.Errorf("进程 %d 已经退出", a.PID)
	}
	hwnd, ok := a.dialogWindow()
	if !ok {
		return none, nil
	}
	return DialogState{Open: true, Text: dialogText(hwnd)}, nil
}

// DialogOK 点提示框的确定。
func (a *App) DialogOK() error {
	hwnd, ok := a.dialogWindow()
	if !ok {
		return fmt.Errorf("没有弹窗")
	}
	button := dialogButton(hwnd)
	if button == 0 {
		return fmt.Errorf("弹窗里没有确定按钮: %s", describeChildren(hwnd))
	}
	if err := postMessage(button, bmClick, 0, 0); err != nil {
		return err
	}
	return poll(driveTimeout, func() bool {
		_, still := a.dialogWindow()
		return !still
	})
}

// Log 读测试实例的 stderr。attach 到别人启动的进程时可能没有这份文件。
func (a *App) Log() (string, error) {
	if a.LogPath == "" {
		return "", fmt.Errorf("没有日志文件")
	}
	data, err := os.ReadFile(a.LogPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WaitSettings 等到设置窗口打开或关掉。
func (a *App) WaitSettings(open bool) (UIState, error) {
	var state UIState
	err := poll(driveTimeout, func() bool {
		var readErr error
		state, readErr = a.State()
		if readErr != nil {
			return false
		}
		return state.SettingsOpen == open
	})
	if err != nil {
		if open {
			return state, fmt.Errorf("设置窗口没有打开")
		}
		return state, fmt.Errorf("设置窗口没有关闭")
	}
	return state, nil
}

// WaitDialog 等到提示框出现。
func (a *App) WaitDialog() (DialogState, error) {
	var dialog DialogState
	err := poll(driveTimeout, func() bool {
		var readErr error
		dialog, readErr = a.Dialog()
		return readErr == nil && dialog.Open
	})
	if err != nil {
		return dialog, fmt.Errorf("没有弹出提示")
	}
	return dialog, nil
}

// WaitLog 等到日志里出现 text。
func (a *App) WaitLog(text string) error {
	err := poll(driveTimeout, func() bool {
		body, readErr := a.Log()
		return readErr == nil && strings.Contains(body, text)
	})
	if err != nil {
		return fmt.Errorf("日志里没有 %q", text)
	}
	return nil
}

// WaitControl 等到某个控件满足 match。
func (a *App) WaitControl(name string, match func(ControlState) bool) (ControlState, error) {
	var last ControlState
	seen := false
	err := poll(driveTimeout, func() bool {
		state, readErr := a.State()
		if readErr != nil {
			return false
		}
		ctrl, ok := state.Controls[name]
		if !ok {
			return false
		}
		last = ctrl
		seen = true
		return match(ctrl)
	})
	if err != nil {
		if !seen {
			return last, fmt.Errorf("没有控件 %s", name)
		}
		return last, fmt.Errorf("控件 %s 没有变成期望状态", name)
	}
	return last, nil
}

func (a *App) waitCmd(timeout time.Duration) error {
	done := make(chan struct{})
	go func() {
		a.wait()
		close(done)
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-timer.C:
		return fmt.Errorf("进程 %d 没有在 %s 内退出", a.PID, timeout)
	}
}

func (a *App) wait() {
	a.waitOnce.Do(func() {
		if a.cmd != nil {
			a.waitErr = a.cmd.Wait()
		}
	})
}

func (a *App) hidden() (uintptr, error) {
	list := windowsBy(a.PID, "transhelper_message")
	if len(list) == 0 {
		return 0, fmt.Errorf("进程 %d 没有托盘窗口", a.PID)
	}
	return list[0], nil
}

func (a *App) settingsList() []uintptr {
	// 创建过程中窗口先藏着，填完控件才显示。藏着的那段不能当「已经打开」。
	var out []uintptr
	for _, hwnd := range windowsBy(a.PID, "transhelper_settings") {
		if windowVisible(hwnd) {
			out = append(out, hwnd)
		}
	}
	return out
}

func (a *App) requireSettings() (uintptr, error) {
	list := a.settingsList()
	if len(list) == 0 {
		return 0, fmt.Errorf("设置窗口没有打开")
	}
	return list[0], nil
}

func (a *App) control(name string) (controlSpec, uintptr, error) {
	var spec controlSpec
	found := false
	for _, item := range controlSpecs {
		if item.name == name {
			spec = item
			found = true
			break
		}
	}
	if !found {
		return controlSpec{}, 0, fmt.Errorf("不认识的控件 %s", name)
	}
	settings, err := a.requireSettings()
	if err != nil {
		return controlSpec{}, 0, err
	}
	hwnd := findChild(settings, spec.id)
	if hwnd == 0 {
		return controlSpec{}, 0, fmt.Errorf("找不到控件 %s", name)
	}
	return spec, hwnd, nil
}

func (a *App) dialogWindow() (uintptr, bool) {
	settings, err := a.requireSettings()
	if err != nil {
		return 0, false
	}
	for _, hwnd := range windowsBy(a.PID, "#32770") {
		owner, _, _ := procGetWindow.Call(hwnd, gwOwner)
		if owner == 0 {
			owner, _, _ = procGetParent.Call(hwnd)
		}
		if owner == settings {
			return hwnd, true
		}
	}
	return 0, false
}

func readControl(spec controlSpec, hwnd uintptr) ControlState {
	text := readWindowText(hwnd)
	ctrl := ControlState{
		TextLen: len([]rune(text)),
		Checked: isChecked(hwnd),
		Enabled: windowEnabled(hwnd),
		Visible: windowVisible(hwnd),
	}
	if spec.secret {
		char, _ := sendMessage(hwnd, emGetPasswordChar, 0, 0)
		value := int(char)
		ctrl.PasswordChar = &value
	} else {
		ctrl.Text = text
	}
	if spec.combo != nil {
		got, _ := sendMessage(hwnd, cbGetCurSel, 0, 0)
		if int(got) >= 0 && int(got) < len(spec.combo) {
			ctrl.Value = spec.combo[got]
		}
	}
	if spec.list {
		items, selected := readListBox(hwnd)
		ctrl.Items = items
		ctrl.Selected = selected
		if top, err := sendMessage(hwnd, lbGetTopIndex, 0, 0); err == nil && int32(top) >= 0 {
			ctrl.Top = int(top)
		}
	}
	return ctrl
}

func readListBox(hwnd uintptr) (items []string, selected string) {
	count, err := sendMessage(hwnd, lbGetCount, 0, 0)
	if err != nil || int32(count) <= 0 {
		return nil, ""
	}
	sel, _ := sendMessage(hwnd, lbGetCurSel, 0, 0)
	for i := 0; i < int(count); i++ {
		text := readListItem(hwnd, i)
		items = append(items, text)
		if int(sel) == i {
			selected = text
		}
	}
	return items, selected
}

func readListItem(hwnd uintptr, index int) string {
	n, err := sendMessage(hwnd, lbGetTextLen, uintptr(index), 0)
	if err != nil || int32(n) <= 0 || n > 1<<20 {
		return ""
	}
	buf := make([]uint16, int(n)+1)
	if _, err := sendMessage(hwnd, lbGetText, uintptr(index), uintptr(unsafe.Pointer(&buf[0]))); err != nil {
		return ""
	}
	runtime.KeepAlive(buf)
	return windows.UTF16ToString(buf)
}

func dialogText(hwnd uintptr) string {
	if child := findChild(hwnd, dialogStaticID); child != 0 {
		if text := strings.TrimSpace(readWindowText(child)); text != "" {
			return text
		}
	}
	best := ""
	for _, child := range childrenOf(hwnd) {
		if !strings.EqualFold(classNameOf(child), "Static") {
			continue
		}
		text := strings.TrimSpace(readWindowText(child))
		if len([]rune(text)) > len([]rune(best)) {
			best = text
		}
	}
	return best
}

func findChild(parent, id uintptr) uintptr {
	var found uintptr
	for _, hwnd := range childrenOf(parent) {
		got, _, _ := procGetDlgCtrlID.Call(hwnd)
		if got != id {
			continue
		}
		// 标签放不下时，系统会塞进一个 ID 为 1 的微调按钮，和「应用」撞号。
		if classNameOf(hwnd) == "msctls_updown32" {
			continue
		}
		found = hwnd
	}
	return found
}

func dialogButton(hwnd uintptr) uintptr {
	var fallback uintptr
	for _, child := range childrenOf(hwnd) {
		if !strings.EqualFold(classNameOf(child), "Button") {
			continue
		}
		if fallback == 0 {
			fallback = child
		}
		switch readWindowText(child) {
		case "确定", "OK":
			return child
		}
	}
	if button := findChild(hwnd, 1); button != 0 && strings.EqualFold(classNameOf(button), "Button") {
		return button
	}
	return fallback
}

func describeChildren(hwnd uintptr) string {
	var parts []string
	for _, child := range childrenOf(hwnd) {
		id, _, _ := procGetDlgCtrlID.Call(child)
		parts = append(parts, fmt.Sprintf("%s#%d %q", classNameOf(child), id, readWindowText(child)))
	}
	if len(parts) == 0 {
		return "没有子窗口"
	}
	return strings.Join(parts, "; ")
}

// writeWindowText 用 WM_SETTEXT 写控件。
// SetWindowText 跨进程只会改标题，输入框里的文字不变。
func writeWindowText(hwnd uintptr, text string) (uintptr, error) {
	buf := utf16String(text)
	result, err := sendMessage(hwnd, wmSetText, 0, uintptr(unsafe.Pointer(&buf[0])))
	runtime.KeepAlive(buf)
	return result, err
}

// readWindowText 用 WM_GETTEXT 读控件。
// 别的进程里的输入框没有标题，GetWindowText 只能读到空字符串。
func readWindowText(hwnd uintptr) string {
	n, err := sendMessage(hwnd, wmGetTextLength, 0, 0)
	if err != nil || n > 1<<20 {
		return ""
	}
	buf := make([]uint16, n+1)
	if _, err := sendMessage(hwnd, wmGetText, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0]))); err != nil {
		return ""
	}
	runtime.KeepAlive(buf)
	return windows.UTF16ToString(buf)
}

func findClass(parent uintptr, class string) uintptr {
	for _, hwnd := range childrenOf(parent) {
		if classNameOf(hwnd) == class {
			return hwnd
		}
	}
	return 0
}

func windowsBy(pid int, class string) []uintptr {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumPID = uint32(pid)
	enumClass = class
	enumHits = nil
	procEnumWindows.Call(enumCB, 0)
	out := append([]uintptr(nil), enumHits...)
	enumHits = nil
	return out
}

func childrenOf(parent uintptr) []uintptr {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumHits = nil
	procEnumChildWindows.Call(parent, enumChildCB, 0)
	out := append([]uintptr(nil), enumHits...)
	enumHits = nil
	return out
}

func enumTop(hwnd, _ uintptr) uintptr {
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid != enumPID {
		return 1
	}
	if enumClass != "" && classNameOf(hwnd) != enumClass {
		return 1
	}
	enumHits = append(enumHits, hwnd)
	return 1
}

func enumChild(hwnd, _ uintptr) uintptr {
	enumHits = append(enumHits, hwnd)
	return 1
}

func sendMessage(hwnd, msg, wParam, lParam uintptr) (uintptr, error) {
	if hwnd == 0 {
		return 0, fmt.Errorf("窗口无效")
	}
	var result uintptr
	r, _, err := procSendMessageTimeout.Call(
		hwnd,
		msg,
		wParam,
		lParam,
		smtoAbortIfHung,
		sendTimeoutMS,
		uintptr(unsafe.Pointer(&result)),
	)
	runtime.KeepAlive(&result)
	if r == 0 {
		if err == nil || err == windows.ERROR_SUCCESS {
			return 0, fmt.Errorf("发送消息超时或窗口无响应")
		}
		return 0, err
	}
	return result, nil
}

func postMessage(hwnd, msg, wParam, lParam uintptr) error {
	if hwnd == 0 {
		return fmt.Errorf("窗口无效")
	}
	r, _, err := procPostMessage.Call(hwnd, msg, wParam, lParam)
	if r == 0 {
		if err == nil || err == windows.ERROR_SUCCESS {
			return fmt.Errorf("投递消息失败")
		}
		return err
	}
	return nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == stillActive
}

func processExe(pid int) (string, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	buf := make([]uint16, 32768)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &size); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:size]), nil
}

func poll(timeout time.Duration, cond func() bool) error {
	if timeout <= 0 {
		timeout = driveTimeout
	}
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("等待超过 %s", timeout)
		}
		time.Sleep(pollGap)
	}
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, item := range env {
		if len(item) >= len(prefix) && strings.EqualFold(item[:len(prefix)], prefix) {
			continue
		}
		out = append(out, item)
	}
	return append(out, prefix+value)
}

func ensureHelper(root string) (string, error) {
	helperOnce.Do(func() {
		dir := filepath.Join(root, ".tools", "tmp", "uidrive")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			helperErr = err
			return
		}
		// 每次测试进程用自己的文件名。上一次没退干净时，运行中的 exe 不能被覆盖。
		helperExe = filepath.Join(dir, fmt.Sprintf("transhelper-%d.exe", os.Getpid()))
		ctx, cancel := context.WithTimeout(context.Background(), buildTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "go", "build", "-tags", "uidrive", "-ldflags=-H=windowsgui", "-o", helperExe, "./cmd/transhelper")
		cmd.Dir = root
		cmd.Env = setEnv(os.Environ(), "CGO_ENABLED", "0")
		out, err := cmd.CombinedOutput()
		if ctx.Err() == context.DeadlineExceeded {
			helperErr = fmt.Errorf("编译测试实例超时")
			return
		}
		if err != nil {
			text := strings.TrimSpace(string(out))
			if text == "" {
				helperErr = fmt.Errorf("编译测试实例: %w。先在项目根目录执行 . .\\scripts\\env.ps1", err)
				return
			}
			helperErr = fmt.Errorf("编译测试实例: %w\n%s", err, text)
		}
	})
	return helperExe, helperErr
}
