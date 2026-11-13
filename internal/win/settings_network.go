//go:build windows

package win

import (
	"runtime"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"transhelper/internal/config"
)

// networkPage 是「网络」页。未启用代理时，下面的控件变灰，已填的值仍会保存。
type networkPage struct {
	enable    uintptr
	typeLabel uintptr
	kind      uintptr
	hostLabel uintptr
	host      uintptr
	portLabel uintptr
	port      uintptr
	auth      uintptr
	userLabel uintptr
	user      uintptr
	passLabel uintptr
	pass      uintptr
	forModel  uintptr
}

func (n *networkPage) controls() []uintptr {
	return []uintptr{
		n.enable, n.typeLabel, n.kind, n.hostLabel, n.host, n.portLabel, n.port,
		n.auth, n.userLabel, n.user, n.passLabel, n.pass, n.forModel,
	}
}

func networkBody() int {
	return 7 * (layoutRowH + layoutRowGap)
}

func (s *settingsWindow) createNetwork(instance windows.Handle) error {
	labelStyle := uintptr(wsChild | wsVisible | ssCenterImage)
	editStyle := uintptr(wsChild | wsVisible | wsTabStop | wsBorder | esAutohScroll)
	checkStyle := uintptr(wsChild | wsVisible | wsTabStop | bsAutoCheck)
	var err error
	if s.network.enable, err = createControl("BUTTON", "启用代理", checkStyle, 0, 0, 10, 10, s.hwnd, idProxyOn, instance); err != nil {
		return err
	}
	if s.network.typeLabel, err = createControl("STATIC", "类型", labelStyle, 0, 0, 10, 10, s.hwnd, 0, instance); err != nil {
		return err
	}
	// 下拉列表的高度在 place 里给。这里先占个位置。
	comboStyle := uintptr(wsChild | wsVisible | wsTabStop | wsVScroll | cbsDropDownList | cbsHasStrings)
	if s.network.kind, err = createControl("COMBOBOX", "", comboStyle, 0, 0, 10, 10, s.hwnd, idProxyType, instance); err != nil {
		return err
	}
	for _, item := range []string{"HTTP", "SOCKS5"} {
		text := utf16String(item)
		procSendMessage.Call(s.network.kind, cbAddString, 0, uintptr(unsafe.Pointer(&text[0])))
		runtime.KeepAlive(text)
	}
	if s.network.hostLabel, err = createControl("STATIC", "地址", labelStyle, 0, 0, 10, 10, s.hwnd, 0, instance); err != nil {
		return err
	}
	if s.network.host, err = createControl("EDIT", "", editStyle, 0, 0, 10, 10, s.hwnd, idProxyHost, instance); err != nil {
		return err
	}
	if s.network.portLabel, err = createControl("STATIC", "端口", labelStyle, 0, 0, 10, 10, s.hwnd, 0, instance); err != nil {
		return err
	}
	if s.network.port, err = createControl("EDIT", "", editStyle|esNumber, 0, 0, 10, 10, s.hwnd, idProxyPort, instance); err != nil {
		return err
	}
	if s.network.auth, err = createControl("BUTTON", "需要身份验证", checkStyle, 0, 0, 10, 10, s.hwnd, idProxyAuth, instance); err != nil {
		return err
	}
	if s.network.userLabel, err = createControl("STATIC", "用户名", labelStyle, 0, 0, 10, 10, s.hwnd, 0, instance); err != nil {
		return err
	}
	if s.network.user, err = createControl("EDIT", "", editStyle, 0, 0, 10, 10, s.hwnd, idProxyUser, instance); err != nil {
		return err
	}
	if s.network.passLabel, err = createControl("STATIC", "密码", labelStyle, 0, 0, 10, 10, s.hwnd, 0, instance); err != nil {
		return err
	}
	if s.network.pass, err = createControl("EDIT", "", editStyle|esPassword, 0, 0, 10, 10, s.hwnd, idProxyPass, instance); err != nil {
		return err
	}
	if s.network.forModel, err = createControl("BUTTON", "模型请求也走代理", checkStyle, 0, 0, 10, 10, s.hwnd, idProxyModel, instance); err != nil {
		return err
	}
	return nil
}

func (n *networkPage) place(s *settingsWindow) {
	top := s.contentTop()
	row := func(i int) int {
		return top + s.scale(i*(layoutRowH+layoutRowGap))
	}
	labelX := s.scale(layoutMargin)
	valueX := s.scale(layoutMargin + layoutLabelW + layoutGap)
	labelW := s.scale(layoutLabelW)
	rowH := s.scale(layoutRowH)
	fullW := s.scale(layoutLabelW + layoutGap + layoutEditW)
	moveWindow(n.enable, labelX, row(0), fullW, rowH)
	moveWindow(n.typeLabel, labelX, row(1), labelW, rowH)
	// 组合框的高度包含收起后的下拉列表。列表弹出时盖住下面的行，收起后只有一行高。
	moveWindow(n.kind, valueX, row(1), s.scale(140), s.scale(140))
	gap := s.scale(layoutGap)
	portW := s.scale(layoutTimeW)
	portLabelW := s.scale(40)
	hostW := s.scale(layoutEditW) - portLabelW - portW - gap
	if hostW < s.scale(80) {
		hostW = s.scale(80)
	}
	moveWindow(n.hostLabel, labelX, row(2), labelW, rowH)
	moveWindow(n.host, valueX, row(2), hostW, rowH)
	portLabelX := valueX + hostW + gap
	moveWindow(n.portLabel, portLabelX, row(2), portLabelW, rowH)
	moveWindow(n.port, portLabelX+portLabelW, row(2), portW, rowH)
	moveWindow(n.auth, labelX, row(3), fullW, rowH)
	moveWindow(n.userLabel, labelX, row(4), labelW, rowH)
	moveWindow(n.user, valueX, row(4), s.scale(layoutEditW), rowH)
	moveWindow(n.passLabel, labelX, row(5), labelW, rowH)
	moveWindow(n.pass, valueX, row(5), s.scale(layoutEditW), rowH)
	moveWindow(n.forModel, labelX, row(6), fullW, rowH)
}

func (n *networkPage) show(on bool) {
	showControls(n.controls(), on)
}

func (n *networkPage) fill(cfg config.Config) {
	setChecked(n.enable, cfg.Proxy.Enabled)
	index := uintptr(0)
	if strings.EqualFold(cfg.Proxy.Type, "socks5") {
		index = 1
	}
	procSendMessage.Call(n.kind, cbSetCurSel, index, 0)
	setWindowText(n.host, cfg.Proxy.Host)
	if cfg.Proxy.Port > 0 {
		setWindowText(n.port, strconv.Itoa(cfg.Proxy.Port))
	} else {
		setWindowText(n.port, "")
	}
	setChecked(n.auth, cfg.Proxy.Auth)
	setWindowText(n.user, cfg.Proxy.Username)
	setWindowText(n.pass, cfg.Proxy.Password)
	setChecked(n.forModel, cfg.Proxy.ForModel)
	procSendMessage.Call(n.host, emSetLimitText, 255, 0)
	procSendMessage.Call(n.port, emSetLimitText, 5, 0)
	procSendMessage.Call(n.user, emSetLimitText, 256, 0)
	procSendMessage.Call(n.pass, emSetLimitText, 256, 0)
	n.syncEnabled()
}

func (n *networkPage) read() (config.Proxy, uintptr, string, bool) {
	proxy := config.Proxy{Type: "http"}
	if sel, _, _ := procSendMessage.Call(n.kind, cbGetCurSel, 0, 0); int32(sel) == 1 {
		proxy.Type = "socks5"
	}
	proxy.Enabled = isChecked(n.enable)
	proxy.Host = strings.TrimSpace(windowText(n.host))
	proxy.Auth = isChecked(n.auth)
	proxy.Username = strings.TrimSpace(windowText(n.user))
	// 密码不去掉首尾空格，和配置解析一致。
	proxy.Password = windowText(n.pass)
	proxy.ForModel = isChecked(n.forModel)
	portText := strings.TrimSpace(windowText(n.port))
	if portText == "" {
		proxy.Port = 0
	} else {
		port, err := strconv.Atoi(portText)
		if err != nil || port < 0 || port > 65535 {
			return proxy, n.port, "端口必须是 0 到 65535 的整数", true
		}
		proxy.Port = port
	}
	if proxy.Host != "" && !proxyHostOK(proxy.Host) {
		return proxy, n.host, "代理地址不要写协议、路径或空格", true
	}
	if !proxy.Enabled {
		return proxy, 0, "", false
	}
	if proxy.Host == "" {
		return proxy, n.host, "请填写代理地址", true
	}
	if proxy.Port < 1 {
		return proxy, n.port, "端口必须是 1 到 65535 的整数", true
	}
	if proxy.Auth && proxy.Username == "" {
		return proxy, n.user, "请填写代理用户名", true
	}
	return proxy, 0, "", false
}

func proxyHostOK(host string) bool {
	if strings.Contains(host, "://") || strings.Contains(host, "/") || strings.Contains(host, "@") {
		return false
	}
	return !strings.ContainsAny(host, "[] \t\r\n")
}

func (n *networkPage) onCommand(id uintptr) {
	switch id {
	case idProxyOn, idProxyAuth:
		n.syncEnabled()
	}
}

func (n *networkPage) syncEnabled() {
	on := isChecked(n.enable)
	for _, hwnd := range []uintptr{n.typeLabel, n.kind, n.hostLabel, n.host, n.portLabel, n.port, n.auth, n.forModel} {
		enableWindow(hwnd, on)
	}
	// 代理没开时，用户名和密码跟着变灰。开了之后还要看身份验证。
	enableWindow(n.userLabel, on && isChecked(n.auth))
	enableWindow(n.user, on && isChecked(n.auth))
	enableWindow(n.passLabel, on && isChecked(n.auth))
	enableWindow(n.pass, on && isChecked(n.auth))
}

func (n *networkPage) dropped() bool {
	if n.kind == 0 {
		return false
	}
	state, _, _ := procSendMessage.Call(n.kind, cbGetDroppedState, 0, 0)
	return state != 0
}

func (n *networkPage) tips() []tipItem {
	return []tipItem{
		{n.enable, "关闭时沿用 HTTPS_PROXY 等环境变量"},
		{n.kind, "HTTP 或 SOCKS5"},
		{n.host, "代理服务器的主机名或 IP，不要写协议和端口"},
		{n.port, "1 到 65535"},
		{n.auth, "勾选后填写用户名和密码"},
		{n.user, "代理要求身份验证时填写"},
		{n.pass, "只保存在本机配置文件"},
		{n.forModel, "模型在本机或内网时不要勾选"},
	}
}

func enableWindow(hwnd uintptr, on bool) {
	if hwnd == 0 {
		return
	}
	flag := uintptr(0)
	if on {
		flag = 1
	}
	procEnableWindow.Call(hwnd, flag)
}

func isChecked(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	state, _, _ := procSendMessage.Call(hwnd, bmGetCheck, 0, 0)
	return state == bstChecked
}

func setChecked(hwnd uintptr, on bool) {
	if hwnd == 0 {
		return
	}
	flag := uintptr(0)
	if on {
		flag = bstChecked
	}
	procSendMessage.Call(hwnd, bmSetCheck, flag, 0)
}
