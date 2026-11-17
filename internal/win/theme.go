//go:build windows

package win

import (
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"transhelper/internal/config"
	"transhelper/internal/update"
)

const (
	// COLORREF 是 0x00BBGGRR。
	darkBg    = 0x00202020
	darkEdit  = 0x002B2B2B
	darkText  = 0x00F0F0F0
	darkMuted = 0x00888888
	darkLine  = 0x00C8C8C8
	linkBlue  = 0x00FFB66C

	gwlStyle = ^uintptr(15) // GWL_STYLE = -16

	wmSettingChange = 0x001A
	wmNCDestroy     = 0x0082
	wmPaint         = 0x000F

	cbnSelChange = 1

	bkTransparent = 1

	dtCenter      = 0x0001
	dtVCenter     = 0x0004
	dtWordBreak   = 0x0010
	dtSingleLine  = 0x0020
	dtNoPrefix    = 0x0800
	dtEndEllipsis = 0x8000

	nmCustomDrawCode = -12
	cddsPrePaint     = 1
	cdrfSkipDefault  = 4

	cdisSelected = 0x0001
	cdisDisabled = 0x0004
	cdisHot      = 0x0040

	bpCheckbox = 3

	dwmwaDarkMode    = 20
	dwmwaDarkModeOld = 19

	ttmSetTipBkColor   = wmUser + 19
	ttmSetTipTextColor = wmUser + 20
	colorInfoText      = 23
	colorInfoBk        = 24

	tcmGetItemCount = tcmFirst + 4
	tcmGetItemRect  = tcmFirst + 10
	tcmGetItemW     = tcmFirst + 60

	rdwInvalidate  = 0x0001
	rdwErase       = 0x0004
	rdwAllChildren = 0x0080
	rdwUpdateNow   = 0x0100
	rdwFrame       = 0x0400

	subclassTab  = 1
	subclassLink = 2
)

var settingsSubclassProc uintptr

func init() {
	// 放在 init 里，避免窗口过程引用这个变量时和 NewCallback 形成初始化循环。
	settingsSubclassProc = windows.NewCallback(settingsSubclass)
}

// paintStruct 对应 PAINTSTRUCT。64 位上含末尾对齐是 72 字节。
type paintStruct struct {
	hdc       uintptr
	erase     int32
	rcPaint   rect
	restore   int32
	incUpdate int32
	reserved  [32]byte
}

// nmCustomDraw 对应 NMCUSTOMDRAW。64 位上是 80 字节。
type nmCustomDraw struct {
	from      uintptr
	id        uintptr
	code      uint32
	_         uint32
	stage     uint32
	_         uint32
	hdc       uintptr
	rc        rect
	itemSpec  uintptr
	itemState uint32
	_         uint32
	itemParam uintptr
}

// resolveDark 根据设置和系统当前是否为深色，决定窗口用不用深色。
// 除 light、dark 以外都按跟随系统处理。
func resolveDark(theme string, systemIsDark bool) bool {
	switch theme {
	case config.ThemeDark:
		return true
	case config.ThemeLight:
		return false
	default:
		return systemIsDark
	}
}

// systemDark 读取 Windows 应用主题。读不到时按浅色，和原来的外观一致。
// AppsUseLightTheme 为 0 表示应用使用深色。
func systemDark() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	value, _, err := key.GetIntegerValue("AppsUseLightTheme")
	if err != nil {
		return false
	}
	return value == 0
}

func (s *settingsWindow) previewTheme() {
	s.applyTheme(selectedTheme(s.general.theme))
}

func selectedTheme(hwnd uintptr) string {
	sel, _, _ := procSendMessage.Call(hwnd, cbGetCurSel, 0, 0)
	switch int32(sel) {
	case 1:
		return config.ThemeLight
	case 2:
		return config.ThemeDark
	default:
		return config.ThemeSystem
	}
}

func setThemeSelection(hwnd uintptr, theme string) {
	if hwnd == 0 {
		return
	}
	index := uintptr(0)
	switch theme {
	case config.ThemeLight:
		index = 1
	case config.ThemeDark:
		index = 2
	}
	procSendMessage.Call(hwnd, cbSetCurSel, index, 0)
}

// applyTheme 立刻按 theme 重画设置窗口，不写配置。
func (s *settingsWindow) applyTheme(theme string) {
	if s == nil {
		return
	}
	switch theme {
	case config.ThemeLight, config.ThemeDark, config.ThemeSystem:
	default:
		theme = config.ThemeSystem
	}
	s.themeChoice = theme
	s.dark = resolveDark(theme, systemDark())
	s.ensureBrushes()
	s.applyTitleBar()
	s.applyControlThemes()
	s.applyTipColors()
	s.redrawAll()
}

func (s *settingsWindow) ensureBrushes() {
	s.freeBrushes()
	if !s.dark {
		s.closeButtonTheme()
		return
	}
	s.bgBrush = createSolidBrush(darkBg)
	s.editBrush = createSolidBrush(darkEdit)
	s.lineBrush = createSolidBrush(darkLine)
}

func (s *settingsWindow) freeBrushes() {
	if s == nil {
		return
	}
	for _, brush := range []*uintptr{&s.bgBrush, &s.editBrush, &s.lineBrush} {
		if *brush != 0 {
			procDeleteObject.Call(*brush)
			*brush = 0
		}
	}
}

func createSolidBrush(color uintptr) uintptr {
	brush, _, _ := procCreateSolidBrush.Call(color)
	return brush
}

func (s *settingsWindow) applyTitleBar() {
	if s.hwnd == 0 {
		return
	}
	var enabled int32
	if s.dark {
		enabled = 1
	}
	// 20 是文档里的值。1809 到 20H1 之前只有 19，新的失败再试旧的。
	ret, _, _ := procDwmSetWindowAttribute.Call(
		s.hwnd,
		dwmwaDarkMode,
		uintptr(unsafe.Pointer(&enabled)),
		unsafe.Sizeof(enabled),
	)
	if int32(ret) < 0 {
		procDwmSetWindowAttribute.Call(
			s.hwnd,
			dwmwaDarkModeOld,
			uintptr(unsafe.Pointer(&enabled)),
			unsafe.Sizeof(enabled),
		)
	}
	runtime.KeepAlive(&enabled)
}

func (s *settingsWindow) applyControlThemes() {
	explorer := ""
	field := ""
	if s.dark {
		explorer = "DarkMode_Explorer"
		field = "DarkMode_CFD"
	}
	for _, hwnd := range []uintptr{
		s.applyBtn, s.cancel, s.body, s.tip, s.tabs, s.about.link,
		s.general.showKey, s.network.enable, s.network.auth, s.network.forModel,
		s.process.add, s.process.remove,
	} {
		setWindowTheme(hwnd, explorer)
	}
	for _, hwnd := range []uintptr{
		s.general.url, s.general.model, s.general.key, s.general.timeout, s.general.theme,
		s.network.kind, s.network.host, s.network.port, s.network.user, s.network.pass,
		s.about.digest, s.process.running, s.process.enabled,
	} {
		setWindowTheme(hwnd, field)
	}
}

func setWindowTheme(hwnd uintptr, app string) {
	if hwnd == 0 {
		return
	}
	if app == "" {
		procSetWindowTheme.Call(hwnd, 0, 0)
		return
	}
	name := utf16String(app)
	procSetWindowTheme.Call(hwnd, uintptr(unsafe.Pointer(&name[0])), 0)
	runtime.KeepAlive(name)
}

func (s *settingsWindow) applyTipColors() {
	if s.tip == 0 {
		return
	}
	var bg, fg uintptr
	if s.dark {
		bg = darkBg
		fg = darkText
	} else {
		bg, _, _ = procGetSysColor.Call(colorInfoBk)
		fg, _, _ = procGetSysColor.Call(colorInfoText)
	}
	procSendMessage.Call(s.tip, ttmSetTipBkColor, bg, 0)
	procSendMessage.Call(s.tip, ttmSetTipTextColor, fg, 0)
}

func (s *settingsWindow) redrawAll() {
	if s.hwnd == 0 {
		return
	}
	procRedrawWindow.Call(s.hwnd, 0, 0, rdwInvalidate|rdwErase|rdwAllChildren|rdwFrame|rdwUpdateNow)
}

func (s *settingsWindow) onSettingChange(lParam uintptr) {
	if s.themeChoice != config.ThemeSystem {
		return
	}
	if name := utf16At(lParam, 64); name != "" && !strings.EqualFold(name, "ImmersiveColorSet") {
		return
	}
	s.applyTheme(config.ThemeSystem)
}

func utf16At(addr uintptr, maxChars int) string {
	if addr == 0 || maxChars <= 0 {
		return ""
	}
	raw := copyFromMemory(addr, maxChars*2)
	if len(raw) < 2 {
		return ""
	}
	units := make([]uint16, 0, 16)
	for i := 0; i+1 < len(raw); i += 2 {
		char := uint16(raw[i]) | uint16(raw[i+1])<<8
		if char == 0 {
			break
		}
		units = append(units, char)
	}
	return windows.UTF16ToString(units)
}

func (s *settingsWindow) controlColor(message, dc, ctrl uintptr) uintptr {
	if !s.dark {
		return 0
	}
	brush := s.bgBrush
	bg := uintptr(darkBg)
	if message == wmCtlColorEdit || message == wmCtlColorListBox {
		brush = s.editBrush
		bg = darkEdit
	}
	if brush == 0 {
		return 0
	}
	color := uintptr(darkText)
	if !windowEnabled(ctrl) {
		color = darkMuted
	}
	procSetTextColor.Call(dc, color)
	procSetBkColor.Call(dc, bg)
	return brush
}

func windowEnabled(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	ret, _, _ := procIsWindowEnabled.Call(hwnd)
	return ret != 0
}

func (s *settingsWindow) hookSubclass(hwnd, id uintptr) {
	if hwnd == 0 {
		return
	}
	procSetWindowSubclass.Call(hwnd, settingsSubclassProc, id, 0)
}

func settingsSubclass(hwnd, message, wParam, lParam, id, _ uintptr) uintptr {
	if message == wmNCDestroy {
		procRemoveWindowSubclass.Call(hwnd, settingsSubclassProc, id)
	}
	s := settingsOwning(hwnd)
	if s != nil && s.dark {
		switch id {
		case subclassTab:
			switch message {
			case wmEraseBkgnd:
				s.eraseBackground(hwnd, wParam)
				return 1
			case wmPaint:
				s.paintTabs(hwnd)
				return 0
			}
		case subclassLink:
			switch message {
			case wmEraseBkgnd:
				s.eraseBackground(hwnd, wParam)
				return 1
			case wmPaint:
				s.paintLink(hwnd)
				return 0
			}
		}
	}
	ret, _, _ := procDefSubclassProc.Call(hwnd, message, wParam, lParam)
	return ret
}

func (s *settingsWindow) paintTabs(hwnd uintptr) {
	dc, ps := beginPaint(hwnd)
	if dc == 0 {
		return
	}
	defer endPaint(hwnd, &ps)
	var area rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&area)))
	if s.bgBrush != 0 {
		procFillRect.Call(dc, uintptr(unsafe.Pointer(&area)), s.bgBrush)
	}
	runtime.KeepAlive(&area)
	count, _, _ := procSendMessage.Call(hwnd, tcmGetItemCount, 0, 0)
	selected, _, _ := procSendMessage.Call(hwnd, tcmGetCurSel, 0, 0)
	old := selectFont(dc, s.font)
	procSetBkMode.Call(dc, bkTransparent)
	for i := uintptr(0); i < count; i++ {
		var item rect
		procSendMessage.Call(hwnd, tcmGetItemRect, i, uintptr(unsafe.Pointer(&item)))
		brush := s.bgBrush
		if int32(i) == int32(selected) {
			brush = s.editBrush
		}
		if brush != 0 {
			procFillRect.Call(dc, uintptr(unsafe.Pointer(&item)), brush)
		}
		if int32(i) == int32(selected) && s.lineBrush != 0 {
			bar := item
			bar.Top = bar.Bottom - int32(s.scale(2))
			if bar.Top < item.Top {
				bar.Top = item.Top
			}
			procFillRect.Call(dc, uintptr(unsafe.Pointer(&bar)), s.lineBrush)
			runtime.KeepAlive(&bar)
		}
		label := utf16String(tabLabel(hwnd, i))
		procSetTextColor.Call(dc, darkText)
		procDrawText.Call(
			dc,
			uintptr(unsafe.Pointer(&label[0])),
			^uintptr(0),
			uintptr(unsafe.Pointer(&item)),
			dtCenter|dtVCenter|dtSingleLine|dtNoPrefix,
		)
		runtime.KeepAlive(label)
		runtime.KeepAlive(&item)
	}
	if old != 0 {
		procSelectObject.Call(dc, old)
	}
}

func tabLabel(hwnd, index uintptr) string {
	buf := make([]uint16, 32)
	item := tabItem{Mask: tcifText, Text: &buf[0], TextMax: int32(len(buf))}
	procSendMessage.Call(hwnd, tcmGetItemW, index, uintptr(unsafe.Pointer(&item)))
	runtime.KeepAlive(&item)
	runtime.KeepAlive(buf)
	text := windows.UTF16ToString(buf)
	if text != "" {
		return text
	}
	names := []string{"通用", "网络", "关于"}
	if int(index) < len(names) {
		return names[index]
	}
	return ""
}

func (s *settingsWindow) paintLink(hwnd uintptr) {
	dc, ps := beginPaint(hwnd)
	if dc == 0 {
		return
	}
	defer endPaint(hwnd, &ps)
	var area rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&area)))
	if s.bgBrush != 0 {
		procFillRect.Call(dc, uintptr(unsafe.Pointer(&area)), s.bgBrush)
	}
	old := selectFont(dc, s.font)
	procSetBkMode.Call(dc, bkTransparent)
	procSetTextColor.Call(dc, linkBlue)
	// 链接没有空格，普通折行不会断开。超宽时在靠近中间的斜杠后换行。
	label := utf16String(wrapAtSlash(dc, update.RepoURL, int(area.Right-area.Left)))
	procDrawText.Call(
		dc,
		uintptr(unsafe.Pointer(&label[0])),
		^uintptr(0),
		uintptr(unsafe.Pointer(&area)),
		dtWordBreak|dtNoPrefix,
	)
	runtime.KeepAlive(label)
	runtime.KeepAlive(&area)
	if old != 0 {
		procSelectObject.Call(dc, old)
	}
}

// wrapAtSlash 在文字比宽度更宽时，于最靠近中间的斜杠后面换行。
func wrapAtSlash(dc uintptr, text string, width int) string {
	if width <= 0 || textWidth(dc, text) <= width {
		return text
	}
	return breakAtSlash(text)
}

func breakAtSlash(text string) string {
	runes := []rune(text)
	if len(runes) < 4 {
		return text
	}
	mid := len(runes) / 2
	best := -1
	for i, r := range runes {
		if r != '/' || i == 0 || i == len(runes)-1 {
			continue
		}
		if best < 0 || absInt(i-mid) < absInt(best-mid) {
			best = i
		}
	}
	if best < 0 {
		return text
	}
	return string(runes[:best+1]) + "\n" + string(runes[best+1:])
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func textWidth(dc uintptr, text string) int {
	if dc == 0 || text == "" {
		return 0
	}
	units := utf16String(text)
	var size point
	procGetTextExtentPoint32.Call(
		dc,
		uintptr(unsafe.Pointer(&units[0])),
		uintptr(len(units)-1),
		uintptr(unsafe.Pointer(&size)),
	)
	runtime.KeepAlive(units)
	runtime.KeepAlive(&size)
	if size.X < 0 {
		return 0
	}
	return int(size.X)
}

func beginPaint(hwnd uintptr) (uintptr, paintStruct) {
	var ps paintStruct
	dc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	runtime.KeepAlive(&ps)
	return dc, ps
}

func endPaint(hwnd uintptr, ps *paintStruct) {
	procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(ps)))
	runtime.KeepAlive(ps)
}

func selectFont(dc, font uintptr) uintptr {
	if dc == 0 || font == 0 {
		return 0
	}
	old, _, _ := procSelectObject.Call(dc, font)
	return old
}

// paintCheckboxNotify 在深色时自己画复选框。系统主题下的复选框文字不吃 WM_CTLCOLOR。
func (s *settingsWindow) paintCheckboxNotify(lParam uintptr) (uintptr, bool) {
	size := int(unsafe.Sizeof(nmCustomDraw{}))
	raw := copyFromMemory(lParam, size)
	if len(raw) < size {
		return 0, false
	}
	var info nmCustomDraw
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&info)), size), raw)
	if int32(info.code) != nmCustomDrawCode || !isAutoCheckbox(info.from) || info.stage != cddsPrePaint {
		return 0, false
	}
	s.paintCheckbox(info.from, info.hdc, info.rc, info.itemState)
	return cdrfSkipDefault, true
}

func isAutoCheckbox(hwnd uintptr) bool {
	if hwnd == 0 || !strings.EqualFold(classNameOf(hwnd), "Button") {
		return false
	}
	style, _, _ := procGetWindowLongPtr.Call(hwnd, gwlStyle)
	return style&0x0F == bsAutoCheck
}

func (s *settingsWindow) paintCheckbox(hwnd, dc uintptr, bounds rect, state uint32) {
	if s.bgBrush != 0 {
		procFillRect.Call(dc, uintptr(unsafe.Pointer(&bounds)), s.bgBrush)
	}
	box := int32(s.scale(13))
	if box < 11 {
		box = 11
	}
	top := bounds.Top + (bounds.Bottom-bounds.Top-box)/2
	if top < bounds.Top {
		top = bounds.Top
	}
	boxRc := rect{Left: bounds.Left, Top: top, Right: bounds.Left + box, Bottom: top + box}
	enabled := windowEnabled(hwnd) && state&cdisDisabled == 0
	s.drawCheckBox(hwnd, dc, boxRc, isChecked(hwnd), enabled, state&cdisHot != 0, state&cdisSelected != 0)
	textRc := bounds
	textRc.Left = boxRc.Right + int32(s.scale(6))
	old := selectFont(dc, s.font)
	color := uintptr(darkText)
	if !enabled {
		color = darkMuted
	}
	procSetTextColor.Call(dc, color)
	procSetBkMode.Call(dc, bkTransparent)
	label := utf16String(windowText(hwnd))
	procDrawText.Call(
		dc,
		uintptr(unsafe.Pointer(&label[0])),
		^uintptr(0),
		uintptr(unsafe.Pointer(&textRc)),
		dtVCenter|dtSingleLine|dtNoPrefix|dtEndEllipsis,
	)
	focus, _, _ := procGetFocus.Call()
	if focus == hwnd {
		procDrawFocusRect.Call(dc, uintptr(unsafe.Pointer(&textRc)))
	}
	runtime.KeepAlive(label)
	runtime.KeepAlive(&textRc)
	runtime.KeepAlive(&bounds)
	runtime.KeepAlive(&boxRc)
	if old != 0 {
		procSelectObject.Call(dc, old)
	}
}

func (s *settingsWindow) drawCheckBox(hwnd, dc uintptr, box rect, checked, enabled, hot, pressed bool) {
	theme := s.openButtonTheme(hwnd)
	if theme != 0 {
		ret, _, _ := procDrawThemeBackground.Call(
			theme,
			dc,
			bpCheckbox,
			checkboxPart(checked, enabled, hot, pressed),
			uintptr(unsafe.Pointer(&box)),
			0,
		)
		runtime.KeepAlive(&box)
		if int32(ret) >= 0 {
			return
		}
	}
	s.drawCheckFallback(dc, box, checked)
}

// checkboxPart 是 BP_CHECKBOX 的状态号。未勾选从 1 起，勾选从 5 起。
func checkboxPart(checked, enabled, hot, pressed bool) uintptr {
	part := uintptr(1)
	if checked {
		part = 5
	}
	switch {
	case !enabled:
		return part + 3
	case pressed:
		return part + 2
	case hot:
		return part + 1
	default:
		return part
	}
}

func (s *settingsWindow) drawCheckFallback(dc uintptr, box rect, checked bool) {
	if s.editBrush != 0 {
		procFillRect.Call(dc, uintptr(unsafe.Pointer(&box)), s.editBrush)
	}
	if s.lineBrush != 0 {
		procFrameRect.Call(dc, uintptr(unsafe.Pointer(&box)), s.lineBrush)
	}
	if !checked {
		runtime.KeepAlive(&box)
		return
	}
	mark := utf16String("✓")
	procSetTextColor.Call(dc, darkText)
	procSetBkMode.Call(dc, bkTransparent)
	procDrawText.Call(
		dc,
		uintptr(unsafe.Pointer(&mark[0])),
		^uintptr(0),
		uintptr(unsafe.Pointer(&box)),
		dtCenter|dtVCenter|dtSingleLine|dtNoPrefix,
	)
	runtime.KeepAlive(mark)
	runtime.KeepAlive(&box)
}

func (s *settingsWindow) openButtonTheme(hwnd uintptr) uintptr {
	if s.buttonTheme != 0 {
		return s.buttonTheme
	}
	name := utf16String("Button")
	theme, _, _ := procOpenThemeData.Call(hwnd, uintptr(unsafe.Pointer(&name[0])))
	runtime.KeepAlive(name)
	s.buttonTheme = theme
	return theme
}

func (s *settingsWindow) closeButtonTheme() {
	if s == nil || s.buttonTheme == 0 {
		return
	}
	procCloseThemeData.Call(s.buttonTheme)
	s.buttonTheme = 0
}
