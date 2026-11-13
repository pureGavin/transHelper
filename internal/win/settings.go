//go:build windows

package win

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"transhelper/internal/app"
	"transhelper/internal/config"
)

const (
	settingsStyle = wsCaption | wsSysMenu | wsClipChildren

	wsCaption      = 0x00C00000
	wsSysMenu      = 0x00080000
	wsClipChildren = 0x02000000
	wsChild        = 0x40000000
	wsClipSiblings = 0x04000000
	wsVisible      = 0x10000000
	wsBorder       = 0x00800000
	wsTabStop      = 0x00010000
	wsExTopmost    = 0x00000008

	esAutohScroll = 0x0080
	esPassword    = 0x0020
	esNumber      = 0x2000
	esReadOnly    = 0x0800
	esMultiLine   = 0x0004
	esAutoVScroll = 0x0040
	ssCenterImage = 0x0200
	bsDefPush     = 0x0001
	bsAutoCheck   = 0x0003

	wmDestroy    = 0x0002
	wmCommand    = 0x0111
	wmSetFont    = 0x0030
	wmKeyDown    = 0x0100
	wmDPIChanged = 0x02E0
	dmGetDefID   = 0x0400
	dcHasDefID   = 0x534B

	vkEscape = 0x1B
	vkTab    = 0x09

	idApply   = 1
	idCancel  = 2
	idURL     = 101
	idModel   = 102
	idKey     = 103
	idShowKey = 104
	idTimeout = 105
	idTabs    = 110
	idLink    = 111

	pageGeneral = 0
	pageNetwork = 1
	pageAbout   = 2
	pageCount   = 3

	emSetLimitText     = 0x00C5
	emSetPasswordChar  = 0x00CC
	bmGetCheck         = 0x00F0
	bstChecked         = 1
	spiGetNonClient    = 0x0029
	monitorDefaultNear = 2
	colorBtnFaceBrush  = 16 // COLOR_BTNFACE + 1
	idcArrow           = 32512

	swpNoSize     = 0x0001
	swpNoMove     = 0x0002
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010

	ttsAlwaysTip = 0x01
	ttsNoPrefix  = 0x02
	ttfIDIsHWND  = 0x0001
	ttfSubclass  = 0x0010
	wmUser       = 0x0400
	ttmAddTool   = wmUser + 50
	ttmSetDelay  = wmUser + 3
	ttmSetMaxTip = wmUser + 24
	ttdtAutoPop  = 2
	ttdtInitial  = 3
	iccWin95     = 0x000000FF
	iccLinkClass = 0x00008000

	layoutMargin  = 16
	layoutLabelW  = 88
	layoutGap     = 8
	layoutEditW   = 280
	layoutRowH    = 26
	layoutRowGap  = 8
	layoutBtnW    = 88
	layoutBtnH    = 28
	layoutCheckW  = 72
	layoutTimeW   = 80
	layoutTabTop  = 8
	layoutTabH    = 30
	layoutDigestH = layoutRowH * 2
	layoutLinkH   = layoutRowH * 2

	fwNormal       = 400
	defaultCharset = 1
)

var (
	settingsProc      = windows.NewCallback(settingsWndProc)
	settingsClassName []uint16
	settingsClassAtom uint16
	commonControls    sync.Once
)

// settingsWindow 是托盘菜单里的设置窗口。同一时间只开一个。
// 顶部是三个标签，底部的应用和取消对所有页一起生效。
type settingsWindow struct {
	hwnd     uintptr
	tip      uintptr
	font     uintptr
	dpi      uint32
	base     config.Config
	page     int
	tabs     uintptr
	general  generalPage
	network  networkPage
	about    aboutPage
	applyBtn uintptr
	cancel   uintptr
	tipText  [][]uint16
}

// generalPage 是「通用」页，改模型地址、名称、密钥和超时。
type generalPage struct {
	labels  [4]uintptr
	url     uintptr
	model   uintptr
	key     uintptr
	timeout uintptr
	showKey uintptr
}

func (g generalPage) controls() []uintptr {
	out := []uintptr{g.url, g.model, g.key, g.timeout, g.showKey}
	return append(out, g.labels[:]...)
}

// tipItem 是一条悬停提示。文字要活得和窗口一样久，控件只保存指针。
type tipItem struct {
	hwnd uintptr
	text string
}

func (h *host) openSettings() {
	if h.settings != nil && h.settings.hwnd != 0 {
		procShowWindow.Call(h.settings.hwnd, swShowNormal)
		procSetForegroundWindow.Call(h.settings.hwnd)
		return
	}
	cfg := h.cfg
	if h.configPath != "" {
		loaded, err := config.Load(h.configPath)
		if err != nil {
			h.logger.Printf("打开设置时读取配置失败，改用当前配置: %v", err)
		} else {
			cfg = loaded
		}
	}
	s := &settingsWindow{base: cfg, dpi: 96}
	h.settings = s
	if err := s.create(h); err != nil {
		h.logger.Printf("打开设置窗口失败: %v", err)
		if h.settings == s {
			h.settings = nil
		}
		h.Notify("打不开设置窗口")
	}
}

func (s *settingsWindow) create(h *host) error {
	if err := registerSettingsClass(h.instance, h.icon); err != nil {
		return err
	}
	title := utf16String("transHelper 设置")
	// 先藏着创建，摆好位置再显示，避免在屏幕左上角闪一下。
	hwnd, _, err := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(&settingsClassName[0])),
		uintptr(unsafe.Pointer(&title[0])),
		settingsStyle,
		0, 0, 100, 100,
		h.hwnd,
		0,
		uintptr(h.instance),
		uintptr(unsafe.Pointer(s)),
	)
	runtime.KeepAlive(title)
	runtime.KeepAlive(settingsClassName)
	runtime.KeepAlive(s)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowEx 设置窗口: %w", err)
	}
	s.hwnd = hwnd
	if dpi, _, _ := procGetDpiForWindow.Call(hwnd); dpi != 0 {
		s.dpi = uint32(dpi)
	}
	if err := s.createChildren(h); err != nil {
		s.close()
		return err
	}
	s.applyFont()
	s.place()
	s.fit(true)
	s.createTips()
	s.fill(h)
	procShowWindow.Call(s.hwnd, swShowNormal)
	procSetForegroundWindow.Call(s.hwnd)
	procSetFocus.Call(s.general.url)
	return nil
}

func registerSettingsClass(instance windows.Handle, icon uintptr) error {
	if settingsClassAtom != 0 {
		return nil
	}
	settingsClassName = utf16String("transhelper_settings")
	cursor, _, _ := procLoadCursor.Call(0, idcArrow)
	wc := wndClassEx{
		Size:       uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:    settingsProc,
		Instance:   instance,
		Icon:       windows.Handle(icon),
		Cursor:     windows.Handle(cursor),
		Background: colorBtnFaceBrush,
		ClassName:  &settingsClassName[0],
	}
	atom, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	runtime.KeepAlive(&wc)
	runtime.KeepAlive(settingsClassName)
	if atom == 0 {
		return fmt.Errorf("RegisterClassEx 设置窗口: %w", err)
	}
	settingsClassAtom = uint16(atom)
	return nil
}

func unregisterSettingsClass(instance windows.Handle) {
	if settingsClassAtom == 0 || len(settingsClassName) == 0 {
		return
	}
	procUnregisterClass.Call(uintptr(unsafe.Pointer(&settingsClassName[0])), uintptr(instance))
	runtime.KeepAlive(settingsClassName)
	settingsClassAtom = 0
}

func (s *settingsWindow) createChildren(h *host) error {
	// SysLink 和标签控件都要先初始化。放到创建控件之前，只做一次。
	initCommonControls()
	tabStyle := uintptr(wsChild | wsVisible | wsTabStop | wsClipSiblings)
	var err error
	if s.tabs, err = createControl("SysTabControl32", "", tabStyle, 0, 0, 10, 10, s.hwnd, idTabs, h.instance); err != nil {
		return err
	}
	for _, title := range []string{"通用", "网络", "关于"} {
		if err := s.addTab(title); err != nil {
			return err
		}
	}
	procSendMessage.Call(s.tabs, tcmSetCurSel, pageGeneral, 0)
	if err := s.createGeneral(h.instance); err != nil {
		return err
	}
	if err := s.createNetwork(h.instance); err != nil {
		return err
	}
	if err := s.createAbout(h.instance); err != nil {
		return err
	}
	if s.applyBtn, err = createControl("BUTTON", "应用", uintptr(wsChild|wsVisible|wsTabStop|bsDefPush), 0, 0, 10, 10, s.hwnd, idApply, h.instance); err != nil {
		return err
	}
	if s.cancel, err = createControl("BUTTON", "取消", uintptr(wsChild|wsVisible|wsTabStop), 0, 0, 10, 10, s.hwnd, idCancel, h.instance); err != nil {
		return err
	}
	// 父窗口还没显示。先藏起另外两页，避免一闪而过。
	s.showPage(pageGeneral)
	return nil
}

func (s *settingsWindow) createGeneral(instance windows.Handle) error {
	labelStyle := uintptr(wsChild | wsVisible | ssCenterImage)
	editStyle := uintptr(wsChild | wsVisible | wsTabStop | wsBorder | esAutohScroll)
	labels := []string{"模型地址", "模型名称", "API Key", "超时（秒）"}
	for i, text := range labels {
		hwnd, err := createControl("STATIC", text, labelStyle, 0, 0, 10, 10, s.hwnd, 0, instance)
		if err != nil {
			return err
		}
		s.general.labels[i] = hwnd
	}
	var err error
	if s.general.url, err = createControl("EDIT", "", editStyle, 0, 0, 10, 10, s.hwnd, idURL, instance); err != nil {
		return err
	}
	if s.general.model, err = createControl("EDIT", "", editStyle, 0, 0, 10, 10, s.hwnd, idModel, instance); err != nil {
		return err
	}
	if s.general.key, err = createControl("EDIT", "", editStyle|esPassword, 0, 0, 10, 10, s.hwnd, idKey, instance); err != nil {
		return err
	}
	if s.general.showKey, err = createControl("BUTTON", "显示", uintptr(wsChild|wsVisible|wsTabStop|bsAutoCheck), 0, 0, 10, 10, s.hwnd, idShowKey, instance); err != nil {
		return err
	}
	if s.general.timeout, err = createControl("EDIT", "", editStyle|esNumber, 0, 0, 10, 10, s.hwnd, idTimeout, instance); err != nil {
		return err
	}
	return nil
}

func (s *settingsWindow) addTab(title string) error {
	text := utf16String(title)
	item := tabItem{Mask: tcifText, Text: &text[0]}
	// 序号大于现有个数时，新标签加到末尾。
	index, _, _ := procSendMessage.Call(s.tabs, tcmInsertItemW, uintptr(pageCount), uintptr(unsafe.Pointer(&item)))
	runtime.KeepAlive(&item)
	runtime.KeepAlive(text)
	if int32(index) < 0 {
		return fmt.Errorf("添加标签「%s」失败", title)
	}
	return nil
}

func createControl(className, text string, style uintptr, x, y, w, h int, parent, id uintptr, instance windows.Handle) (uintptr, error) {
	class := utf16String(className)
	name := utf16String(text)
	hwnd, _, err := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(&class[0])),
		uintptr(unsafe.Pointer(&name[0])),
		style,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent,
		id,
		uintptr(instance),
		0,
	)
	runtime.KeepAlive(class)
	runtime.KeepAlive(name)
	if hwnd == 0 {
		return 0, fmt.Errorf("创建%s失败: %w", className, err)
	}
	return hwnd, nil
}

func (s *settingsWindow) fill(h *host) {
	setWindowText(s.general.url, s.base.BaseURL)
	setWindowText(s.general.model, s.base.Model)
	setWindowText(s.general.key, s.base.APIKey)
	seconds := int(s.base.Timeout / time.Second)
	if seconds <= 0 {
		seconds = 20
	}
	setWindowText(s.general.timeout, strconv.Itoa(seconds))
	procSendMessage.Call(s.general.url, emSetLimitText, 2048, 0)
	procSendMessage.Call(s.general.model, emSetLimitText, 256, 0)
	procSendMessage.Call(s.general.key, emSetLimitText, 512, 0)
	procSendMessage.Call(s.general.timeout, emSetLimitText, 3, 0)
	version := ""
	digest := "无法读取"
	if h != nil {
		version = h.version
		digest = h.fileDigest()
	}
	s.about.fill(version, runtime.Version(), digest)
	s.network.fill(s.base)
}

func (s *settingsWindow) scale(v int) int {
	dpi := s.dpi
	if dpi == 0 {
		dpi = 96
	}
	return v * int(dpi) / 96
}

func generalBody() int {
	return 4 * (layoutRowH + layoutRowGap)
}

func (s *settingsWindow) clientSize() (int32, int32) {
	w := s.scale(layoutMargin + layoutLabelW + layoutGap + layoutEditW + layoutMargin)
	body := generalBody()
	if about := aboutBody(); about > body {
		body = about
	}
	if net := networkBody(); net > body {
		body = net
	}
	h := s.scale(layoutTabTop + layoutTabH + layoutRowGap + body + 8 + layoutBtnH + layoutMargin)
	return int32(w), int32(h)
}

func (s *settingsWindow) contentTop() int {
	return s.scale(layoutTabTop + layoutTabH + layoutRowGap)
}

func (s *settingsWindow) place() {
	s.placeTabs()
	s.placeGeneral()
	s.network.place(s)
	s.placeAbout()
	s.placeButtons()
}

func (s *settingsWindow) placeTabs() {
	clientW, _ := s.clientSize()
	x := s.scale(layoutMargin)
	w := int(clientW) - 2*x
	if w < s.scale(40) {
		w = s.scale(40)
	}
	// 标签控件只占顶部一条。页面控件画在下面的窗口背景上，
	// 不放进标签控件，避免主题底色不一致，也避免两层控件叠在一起画。
	moveWindow(s.tabs, x, s.scale(layoutTabTop), w, s.scale(layoutTabH))
}

func (s *settingsWindow) placeGeneral() {
	edits := []uintptr{s.general.url, s.general.model, s.general.key, s.general.timeout}
	top := s.contentTop()
	for i := 0; i < 4; i++ {
		y := top + s.scale(i*(layoutRowH+layoutRowGap))
		moveWindow(s.general.labels[i], s.scale(layoutMargin), y, s.scale(layoutLabelW), s.scale(layoutRowH))
		editW := s.scale(layoutEditW)
		switch i {
		case 2:
			editW = s.scale(layoutEditW - layoutCheckW - layoutGap)
		case 3:
			editW = s.scale(layoutTimeW)
		}
		x := s.scale(layoutMargin + layoutLabelW + layoutGap)
		moveWindow(edits[i], x, y, editW, s.scale(layoutRowH))
		if i == 2 {
			moveWindow(s.general.showKey, x+editW+s.scale(layoutGap), y, s.scale(layoutCheckW), s.scale(layoutRowH))
		}
	}
}

func (s *settingsWindow) placeButtons() {
	_, ch := s.clientSize()
	btnY := int(ch) - s.scale(layoutMargin+layoutBtnH)
	clientW, _ := s.clientSize()
	cancelX := int(clientW) - s.scale(layoutMargin) - s.scale(layoutBtnW)
	applyX := cancelX - s.scale(layoutGap) - s.scale(layoutBtnW)
	moveWindow(s.applyBtn, applyX, btnY, s.scale(layoutBtnW), s.scale(layoutBtnH))
	moveWindow(s.cancel, cancelX, btnY, s.scale(layoutBtnW), s.scale(layoutBtnH))
}

func (s *settingsWindow) showPage(page int) {
	s.page = page
	showControls(s.general.controls(), page == pageGeneral)
	s.network.show(page == pageNetwork)
	showControls(s.about.controls(), page == pageAbout)
}

func showControls(hwnds []uintptr, show bool) {
	cmd := uintptr(swHide)
	if show {
		// 不用 SW_SHOW。它会把焦点交给最后一个显示出来的控件。
		cmd = swShowNA
	}
	for _, hwnd := range hwnds {
		if hwnd != 0 {
			procShowWindow.Call(hwnd, cmd)
		}
	}
}

func (s *settingsWindow) currentTab() int {
	if s.tabs == 0 {
		return pageGeneral
	}
	got, _, _ := procSendMessage.Call(s.tabs, tcmGetCurSel, 0, 0)
	if int32(got) < 0 || int(got) >= pageCount {
		return pageGeneral
	}
	return int(got)
}

func (s *settingsWindow) selectPage(page int) {
	if page < 0 || page >= pageCount {
		page = pageGeneral
	}
	if s.tabs != 0 {
		procSendMessage.Call(s.tabs, tcmSetCurSel, uintptr(page), 0)
	}
	s.showPage(page)
	s.focusPage()
}

func (s *settingsWindow) syncPage() {
	s.showPage(s.currentTab())
	s.focusPage()
}

func (s *settingsWindow) cyclePage(back bool) {
	n := s.currentTab()
	if back {
		n = (n + pageCount - 1) % pageCount
	} else {
		n = (n + 1) % pageCount
	}
	s.selectPage(n)
}

func (s *settingsWindow) focusPage() {
	if s.hwnd == 0 || s.tabs == 0 {
		return
	}
	next, _, _ := procGetNextDlgTabItem.Call(s.hwnd, s.tabs, 0)
	if next != 0 && next != s.applyBtn && next != s.cancel {
		procSetFocus.Call(next)
		return
	}
	procSetFocus.Call(s.tabs)
}

func (s *settingsWindow) dropDownOpen() bool {
	return s.network.dropped()
}

func (s *settingsWindow) fit(center bool) {
	cw, ch := s.clientSize()
	ow, oh := frameSize(cw, ch, s.dpi)
	if !center {
		procSetWindowPos.Call(s.hwnd, 0, 0, 0, uintptr(ow), uintptr(oh), swpNoMove|swpNoZOrder)
		return
	}
	x, y := s.centeredOrigin(ow, oh)
	procSetWindowPos.Call(s.hwnd, 0, uintptr(x), uintptr(y), uintptr(ow), uintptr(oh), uintptr(swpNoZOrder))
}

func (s *settingsWindow) centeredOrigin(width, height int32) (int32, int32) {
	mon, _, _ := procMonitorFromWindow.Call(s.hwnd, monitorDefaultNear)
	var info monitorInfo
	info.Size = uint32(unsafe.Sizeof(info))
	ok, _, _ := procGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(&info)
	if ok == 0 || info.Work.Right <= info.Work.Left {
		return 80, 80
	}
	x := info.Work.Left + (info.Work.Right-info.Work.Left-width)/2
	y := info.Work.Top + (info.Work.Bottom-info.Work.Top-height)/2
	return x, y
}

func frameSize(clientW, clientH int32, dpi uint32) (int32, int32) {
	bounds := rect{Right: clientW, Bottom: clientH}
	ret, _, _ := procAdjustWindowRectExForDpi.Call(
		uintptr(unsafe.Pointer(&bounds)),
		settingsStyle,
		0,
		0,
		uintptr(dpi),
	)
	if ret == 0 {
		procAdjustWindowRectEx.Call(
			uintptr(unsafe.Pointer(&bounds)),
			settingsStyle,
			0,
			0,
		)
	}
	return bounds.Right - bounds.Left, bounds.Bottom - bounds.Top
}

func (s *settingsWindow) applyFont() {
	font := s.messageFont()
	if font == 0 {
		return
	}
	for _, hwnd := range s.controls() {
		if hwnd != 0 {
			procSendMessage.Call(hwnd, wmSetFont, font, 1)
		}
	}
	if s.font != 0 {
		procDeleteObject.Call(s.font)
	}
	s.font = font
}

func (s *settingsWindow) controls() []uintptr {
	out := []uintptr{s.tabs, s.applyBtn, s.cancel}
	out = append(out, s.general.controls()...)
	out = append(out, s.network.controls()...)
	out = append(out, s.about.controls()...)
	return out
}

func (s *settingsWindow) messageFont() uintptr {
	var metrics nonClientMetrics
	metrics.Size = uint32(unsafe.Sizeof(metrics))
	ret, _, _ := procSystemParametersInfoForDpi.Call(
		spiGetNonClient,
		uintptr(metrics.Size),
		uintptr(unsafe.Pointer(&metrics)),
		0,
		uintptr(s.dpi),
	)
	if ret == 0 {
		ret, _, _ = procSystemParametersInfo.Call(
			spiGetNonClient,
			uintptr(metrics.Size),
			uintptr(unsafe.Pointer(&metrics)),
			0,
		)
	}
	runtime.KeepAlive(&metrics)
	var lf logFont
	if ret != 0 {
		lf = metrics.MessageFont
	} else {
		lf.Height = -int32(s.scale(12))
		lf.Weight = fwNormal
		lf.CharSet = defaultCharset
		copyUTF16(lf.FaceName[:], "Segoe UI")
	}
	font, _, _ := procCreateFontIndirect.Call(uintptr(unsafe.Pointer(&lf)))
	runtime.KeepAlive(&lf)
	return font
}

func (s *settingsWindow) createTips() {
	initTooltips()
	className := utf16String("tooltips_class32")
	tip, _, _ := procCreateWindowEx.Call(
		wsExTopmost,
		uintptr(unsafe.Pointer(&className[0])),
		0,
		uintptr(wsPopup|ttsAlwaysTip|ttsNoPrefix),
		0, 0, 0, 0,
		s.hwnd,
		0,
		0,
		0,
	)
	runtime.KeepAlive(className)
	if tip == 0 {
		if globalHost != nil {
			globalHost.logger.Printf("创建悬停提示失败")
		}
		return
	}
	s.tip = tip
	procSetWindowPos.Call(tip, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
	s.updateTipWidth()
	// 停在输入框上超过 0.8 秒才显示，10 秒后自动消失。
	procSendMessage.Call(tip, ttmSetDelay, ttdtInitial, 800)
	procSendMessage.Call(tip, ttmSetDelay, ttdtAutoPop, 10000)

	items := []tipItem{
		{s.general.url, "OpenAI 兼容接口地址，末尾保留 /v1"},
		{s.general.model, "要和模型服务上的名称一致"},
		{s.general.key, "不需要时留空，只保存在本机配置文件"},
		{s.general.timeout, "单次翻译最多等待 1 到 120 秒，超时不改动输入框"},
		{s.about.digest, "和 Release 说明里的值核对"},
	}
	items = append(items, s.network.tips()...)
	s.tipText = make([][]uint16, len(items))
	for i, item := range items {
		if item.hwnd == 0 || item.text == "" {
			continue
		}
		// 文字要活得和窗口一样久。控件只保存指针，不一定自己复制。
		s.tipText[i] = utf16String(item.text)
		info := toolInfo{
			Size:  uint32(unsafe.Sizeof(toolInfo{})),
			Flags: ttfIDIsHWND | ttfSubclass,
			Hwnd:  s.hwnd,
			ID:    item.hwnd,
			Text:  &s.tipText[i][0],
		}
		procSendMessage.Call(tip, ttmAddTool, 0, uintptr(unsafe.Pointer(&info)))
		runtime.KeepAlive(&info)
	}
}

func (s *settingsWindow) updateTipWidth() {
	if s.tip == 0 {
		return
	}
	procSendMessage.Call(s.tip, ttmSetMaxTip, 0, uintptr(s.scale(280)))
}

func initCommonControls() {
	commonControls.Do(func() {
		icc := initCommonControlsEx{Size: uint32(unsafe.Sizeof(initCommonControlsEx{})), ICC: iccWin95 | iccLinkClass}
		procInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&icc)))
		runtime.KeepAlive(&icc)
	})
}

func initTooltips() {
	initCommonControls()
}

func (s *settingsWindow) onNotify(wParam, lParam uintptr) {
	switch wParam {
	case idTabs:
		// 不读 lParam。标签页当前选中哪一页，问控件自己就行。
		s.syncPage()
	case idLink:
		code := notifyCode(lParam)
		if code == nmClick || code == nmReturn {
			s.about.openLink()
		}
	}
}

// notifyCode 从 WM_NOTIFY 的 lParam 里取出 NMHDR.code。
// 按字节拷出来，不把消息参数转回指针，避免 go vet 的 unsafeptr 报警。
func notifyCode(lParam uintptr) int32 {
	buf := copyFromMemory(lParam, int(unsafe.Sizeof(nmHdr{})))
	if len(buf) < 20 {
		return 0
	}
	return int32(binary.LittleEndian.Uint32(buf[16:20]))
}

func (s *settingsWindow) apply() {
	h := globalHost
	if s.hwnd == 0 || h == nil {
		return
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(windowText(s.general.timeout)))
	if err != nil || seconds < 1 || seconds > 120 {
		s.reject(pageGeneral, s.general.timeout, "超时必须是 1 到 120 的整数")
		return
	}
	cfg := s.base
	cfg.BaseURL = strings.TrimSpace(windowText(s.general.url))
	cfg.Model = strings.TrimSpace(windowText(s.general.model))
	cfg.APIKey = strings.TrimSpace(windowText(s.general.key))
	cfg.Timeout = time.Duration(seconds) * time.Second
	// 窗口开着时，菜单可能已经改过自动检查。以内存里的为准，避免应用时把它改回去。
	cfg.AutoCheckUpdate = h.cfg.AutoCheckUpdate
	if cfg.BaseURL == "" {
		s.reject(pageGeneral, s.general.url, "请填写模型地址")
		return
	}
	if cfg.Model == "" {
		s.reject(pageGeneral, s.general.model, "请填写模型名称")
		return
	}
	if err := config.Save(h.configPath, cfg); err != nil {
		s.alert(err.Error())
		return
	}
	loaded, err := config.Load(h.configPath)
	if err != nil {
		s.alert(err.Error())
		return
	}
	h.applyConfig(loaded)
	s.close()
	h.info("应用设置成功")
}

func (h *host) applyConfig(cfg config.Config) {
	if h.newTranslator == nil {
		return
	}
	translator := h.newTranslator(cfg)
	if translator == nil {
		return
	}
	h.cfg = cfg
	h.runner = &app.Runner{
		Desktop:    h.desktop,
		Translator: translator,
		Notifier:   h,
		Timeout:    cfg.Timeout,
		Log:        h.logger,
	}
	if h.registered {
		h.unregisterHotkeys()
		h.registered = false
	}
	h.conflictNotified = false
	h.syncHotkeys()
	h.syncAutoCheck()
}

func (s *settingsWindow) syncPassword() {
	state, _, _ := procSendMessage.Call(s.general.showKey, bmGetCheck, 0, 0)
	char := uintptr('*')
	if state == bstChecked {
		char = 0
	}
	procSendMessage.Call(s.general.key, emSetPasswordChar, char, 0)
	procInvalidateRect.Call(s.general.key, 0, 1)
}

// reject 切到出错的那一页，把焦点放回控件，再提示。
func (s *settingsWindow) reject(page int, focus uintptr, text string) {
	s.selectPage(page)
	if focus != 0 {
		procSetFocus.Call(focus)
	}
	s.alert(text)
}

func (s *settingsWindow) alert(text string) {
	body := utf16String(text)
	title := utf16String("transHelper")
	windows.MessageBox(windows.HWND(s.hwnd), &body[0], &title[0], windows.MB_OK|windows.MB_ICONWARNING)
	runtime.KeepAlive(body)
	runtime.KeepAlive(title)
}

func (s *settingsWindow) close() {
	if s == nil || s.hwnd == 0 {
		return
	}
	procDestroyWindow.Call(s.hwnd)
}

func (s *settingsWindow) onDestroy() {
	if s.tip != 0 {
		procDestroyWindow.Call(s.tip)
		s.tip = 0
	}
	if s.font != 0 {
		procDeleteObject.Call(s.font)
		s.font = 0
	}
	// 先摘掉引用，再清 hwnd。销毁过程中如果还有消息进来，不会把这个窗口认成新窗口。
	if globalHost != nil && globalHost.settings == s {
		globalHost.settings = nil
	}
	s.hwnd = 0
}

func (s *settingsWindow) onDPI(wParam, _ uintptr) {
	dpi := uint32(wParam & 0xFFFF)
	if dpi < 96 {
		dpi = 96
	}
	s.dpi = dpi
	s.applyFont()
	s.place()
	s.updateTipWidth()
	// 只改大小，不读系统给的矩形指针。位置保持不动。
	s.fit(false)
}

// handleSettingsKey 处理设置窗口的 Tab、Esc 和回车。
// 它不是系统对话框，IsDialogMessage 对 Esc 和 Tab 不稳定，所以这几个键自己处理。
func (h *host) handleSettingsKey(m *msg) bool {
	s := h.settings
	if s == nil || s.hwnd == 0 || m.Message != wmKeyDown || !s.owns(m.Hwnd) {
		return false
	}
	ctrl, _, _ := procGetKeyState.Call(vkControl)
	ctrlDown := ctrl&0x8000 != 0
	switch m.WParam {
	case vkEscape:
		// 下拉框展开时，Esc 先收起列表，不关窗口。
		if s.dropDownOpen() {
			return false
		}
		s.close()
		return true
	case vkTab:
		if ctrlDown {
			shift, _, _ := procGetKeyState.Call(vkShift)
			s.cyclePage(shift&0x8000 != 0)
			return true
		}
		shift, _, _ := procGetKeyState.Call(vkShift)
		s.nextTab(shift&0x8000 != 0)
		return true
	case vkReturn:
		if s.dropDownOpen() {
			return false
		}
		if m.Hwnd == s.about.link {
			s.about.openLink()
			return true
		}
		if !isButton(m.Hwnd) {
			s.apply()
			return true
		}
	}
	return false
}

func (s *settingsWindow) owns(hwnd uintptr) bool {
	for hwnd != 0 {
		if hwnd == s.hwnd {
			return true
		}
		next, _, _ := procGetParent.Call(hwnd)
		if next == 0 || next == hwnd {
			return false
		}
		hwnd = next
	}
	return false
}

func (s *settingsWindow) nextTab(previous bool) {
	focus, _, _ := procGetFocus.Call()
	var back uintptr
	if previous {
		back = 1
	}
	next, _, _ := procGetNextDlgTabItem.Call(s.hwnd, focus, back)
	if next != 0 {
		procSetFocus.Call(next)
	}
}

func settingsWndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	s := settingsFor(hwnd)
	if s == nil {
		r, _, _ := procDefWindowProc.Call(hwnd, message, wParam, lParam)
		return r
	}
	switch message {
	case wmCommand:
		switch wParam & 0xFFFF {
		case idApply:
			s.apply()
		case idCancel:
			s.close()
		case idShowKey:
			s.syncPassword()
		default:
			s.network.onCommand(wParam & 0xFFFF)
		}
		return 0
	case wmNotify:
		s.onNotify(wParam, lParam)
		return 0
	case wmClose:
		s.close()
		return 0
	case wmDestroy:
		s.onDestroy()
		return 0
	case wmDPIChanged:
		s.onDPI(wParam, lParam)
		return 0
	case dmGetDefID:
		return uintptr(idApply) | uintptr(dcHasDefID)<<16
	}
	r, _, _ := procDefWindowProc.Call(hwnd, message, wParam, lParam)
	return r
}

// settingsFor 找到这个 hwnd 对应的设置窗口。
// 只有一个设置窗口，不把 Go 指针塞进 GWLP_USERDATA 再取回来。
// CreateWindow 返回前窗口过程就会被调用，那时 hwnd 还没记到结构体上。
func settingsFor(hwnd uintptr) *settingsWindow {
	h := globalHost
	if h == nil || h.settings == nil || hwnd == 0 {
		return nil
	}
	s := h.settings
	if s.hwnd == hwnd || s.hwnd == 0 {
		return s
	}
	return nil
}

func isButton(hwnd uintptr) bool {
	return strings.EqualFold(classNameOf(hwnd), "Button")
}

func classNameOf(hwnd uintptr) string {
	buf := make([]uint16, 64)
	procGetClassName.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf)
}

func windowText(hwnd uintptr) string {
	n, _, _ := procGetWindowTextLength.Call(hwnd)
	buf := make([]uint16, n+1)
	procGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf)
}

func setWindowText(hwnd uintptr, text string) {
	u := utf16String(text)
	procSetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&u[0])))
	runtime.KeepAlive(u)
}

func moveWindow(hwnd uintptr, x, y, w, h int) {
	procMoveWindow.Call(hwnd, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 1)
}

type rect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type logFont struct {
	Height         int32
	Width          int32
	Escapement     int32
	Orientation    int32
	Weight         int32
	Italic         byte
	Underline      byte
	StrikeOut      byte
	CharSet        byte
	OutPrecision   byte
	ClipPrecision  byte
	Quality        byte
	PitchAndFamily byte
	FaceName       [32]uint16
}

type nonClientMetrics struct {
	Size              uint32
	BorderWidth       int32
	ScrollWidth       int32
	ScrollHeight      int32
	CaptionWidth      int32
	CaptionHeight     int32
	CaptionFont       logFont
	SmCaptionWidth    int32
	SmCaptionHeight   int32
	SmCaptionFont     logFont
	MenuWidth         int32
	MenuHeight        int32
	MenuFont          logFont
	StatusFont        logFont
	MessageFont       logFont
	PaddedBorderWidth int32
}

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

// nmHdr 对应 NMHDR。64 位上含末尾对齐是 24 字节。
type nmHdr struct {
	From uintptr
	ID   uintptr
	Code uint32
	_    uint32
}

// tabItem 对应 TCITEMW。64 位上 pszText 按 8 字节对齐，整个结构是 40 字节。
type tabItem struct {
	Mask      uint32
	State     uint32
	StateMask uint32
	Text      *uint16
	TextMax   int32
	Image     int32
	Param     uintptr
}

// toolInfo 对应 TOOLINFOW。64 位上含 lpReserved 的结构是 72 字节。
// 少了最后的指针，cbSize 对不上，悬停提示会添加失败。
type toolInfo struct {
	Size     uint32
	Flags    uint32
	Hwnd     uintptr
	ID       uintptr
	Left     int32
	Top      int32
	Right    int32
	Bottom   int32
	Instance uintptr
	Text     *uint16
	Param    uintptr
	Reserved uintptr
}

type initCommonControlsEx struct {
	Size uint32
	ICC  uint32
}

const hwndTopmost = ^uintptr(0)
