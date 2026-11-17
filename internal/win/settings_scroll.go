//go:build windows

package win

import (
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wsExControlParent = 0x00010000

	wmSize            = 0x0005
	wmEraseBkgnd      = 0x0014
	wmGetMinMaxInfo   = 0x0024
	wmNCHitTest       = 0x0084
	wmVScroll         = 0x0115
	wmMouseWheel      = 0x020A
	wmCtlColorEdit    = 0x0133
	wmCtlColorListBox = 0x0134
	wmCtlColorBtn     = 0x0135
	wmCtlColorStatic  = 0x0138

	sbVert = 1

	sbLineUp        = 0
	sbLineDown      = 1
	sbPageUp        = 2
	sbPageDown      = 3
	sbThumbPosition = 4
	sbThumbTrack    = 5
	sbTop           = 6
	sbBottom        = 7

	sifRange    = 0x0001
	sifPage     = 0x0002
	sifPos      = 0x0004
	sifTrackPos = 0x0010

	htLeft        = 10
	htRight       = 11
	htTop         = 12
	htTopLeft     = 13
	htTopRight    = 14
	htBottom      = 15
	htBottomLeft  = 16
	htBottomRight = 17
	htBorder      = 18

	wheelDelta = 120
)

var (
	bodyProc      = windows.NewCallback(bodyWndProc)
	bodyClassName []uint16
	bodyClassAtom uint16
)

// minMaxInfo 对应 MINMAXINFO。五个 POINT，共 40 字节。
type minMaxInfo struct {
	reservedX int32
	reservedY int32
	maxSizeX  int32
	maxSizeY  int32
	maxPosX   int32
	maxPosY   int32
	minTrackX int32
	minTrackY int32
	maxTrackX int32
	maxTrackY int32
}

// scrollInfo 对应 SCROLLINFO。cbSize 到 nTrackPos 共 28 字节。
type scrollInfo struct {
	Size     uint32
	Mask     uint32
	Min      int32
	Max      int32
	Page     uint32
	Pos      int32
	TrackPos int32
}

func unregisterClass(name *[]uint16, atom *uint16, instance windows.Handle) {
	if atom == nil || name == nil || *atom == 0 || len(*name) == 0 {
		return
	}
	procUnregisterClass.Call(uintptr(unsafe.Pointer(&(*name)[0])), uintptr(instance))
	runtime.KeepAlive(*name)
	*atom = 0
}

func registerBodyClass(instance windows.Handle) error {
	if bodyClassAtom != 0 {
		return nil
	}
	bodyClassName = utf16String("transhelper_settings_body")
	cursor, _, _ := procLoadCursor.Call(0, idcArrow)
	wc := wndClassEx{
		Size:       uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:    bodyProc,
		Instance:   instance,
		Cursor:     windows.Handle(cursor),
		Background: colorBtnFaceBrush,
		ClassName:  &bodyClassName[0],
	}
	atom, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	runtime.KeepAlive(&wc)
	runtime.KeepAlive(bodyClassName)
	if atom == 0 {
		return fmt.Errorf("RegisterClassEx 内容区: %w", err)
	}
	bodyClassAtom = uint16(atom)
	return nil
}

func unregisterBodyClass(instance windows.Handle) {
	unregisterClass(&bodyClassName, &bodyClassAtom, instance)
}

func (s *settingsWindow) createBody(instance windows.Handle) error {
	if err := registerBodyClass(instance); err != nil {
		return err
	}
	empty := utf16String("")
	// WS_EX_CONTROLPARENT 让 Tab 能走进里面的输入框。
	// 滚动条按内容显隐，不一直占着位置。
	hwnd, _, err := procCreateWindowEx.Call(
		wsExControlParent,
		uintptr(unsafe.Pointer(&bodyClassName[0])),
		uintptr(unsafe.Pointer(&empty[0])),
		uintptr(wsChild|wsVisible|wsClipChildren|wsClipSiblings|wsVScroll),
		0, 0, 10, 10,
		s.hwnd,
		0,
		uintptr(instance),
		0,
	)
	runtime.KeepAlive(empty)
	runtime.KeepAlive(bodyClassName)
	if hwnd == 0 {
		return fmt.Errorf("创建内容区失败: %w", err)
	}
	s.body = hwnd
	return nil
}

func bodyWndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	s := settingsOwning(hwnd)
	if s == nil {
		r, _, _ := procDefWindowProc.Call(hwnd, message, wParam, lParam)
		return r
	}
	switch message {
	case wmCommand:
		// 投递而不是发送。应用设置会关掉窗口，不能在内容区的窗口过程里把父窗口拆掉。
		if s.hwnd != 0 {
			procPostMessage.Call(s.hwnd, wmCommand, wParam, lParam)
		}
		return 0
	case wmNotify, wmCtlColorStatic, wmCtlColorEdit, wmCtlColorBtn, wmCtlColorListBox:
		// 控件的父窗口是内容区。转给设置窗口，沿用原来的通知和配色处理。
		if s.hwnd != 0 {
			r, _, _ := procSendMessage.Call(s.hwnd, message, wParam, lParam)
			return r
		}
	case wmVScroll:
		s.onVScroll(wParam)
		return 0
	case wmMouseWheel:
		s.onMouseWheel(wParam)
		return 0
	case wmSize:
		s.layoutBody()
		return 0
	case wmEraseBkgnd:
		s.eraseBackground(hwnd, wParam)
		return 1
	}
	r, _, _ := procDefWindowProc.Call(hwnd, message, wParam, lParam)
	return r
}

// settingsOwning 沿着父窗口找到设置窗口。
// 创建内容区时 body 字段还没写上，这时靠再上一层的设置窗口认。
func settingsOwning(hwnd uintptr) *settingsWindow {
	h := globalHost
	if h == nil || h.settings == nil || hwnd == 0 {
		return nil
	}
	s := h.settings
	for hwnd != 0 {
		if hwnd == s.hwnd || (s.body != 0 && hwnd == s.body) {
			return s
		}
		next, _, _ := procGetParent.Call(hwnd)
		if next == 0 || next == hwnd {
			break
		}
		hwnd = next
	}
	return nil
}

func (s *settingsWindow) eraseBackground(hwnd, dc uintptr) {
	var area rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&area)))
	brush := s.backgroundBrush()
	procFillRect.Call(dc, uintptr(unsafe.Pointer(&area)), brush)
	runtime.KeepAlive(&area)
}

func (s *settingsWindow) backgroundBrush() uintptr {
	if s != nil && s.dark && s.bgBrush != 0 {
		return s.bgBrush
	}
	brush, _, _ := procGetSysColorBrush.Call(colorBtnFace)
	return brush
}

func (s *settingsWindow) placeBody(clientW, clientH int) {
	top := s.contentTop()
	bottom := clientH - s.scale(8+layoutBtnH+layoutMargin)
	height := bottom - top
	if height < 0 {
		height = 0
	}
	moveWindow(s.body, 0, top, clientW, height)
}

func (s *settingsWindow) pageContentHeight() int {
	rows := generalBody()
	switch s.page {
	case pageProcesses:
		rows = processBody()
	case pageNetwork:
		rows = networkBody()
	case pageAbout:
		rows = aboutBody()
	}
	return s.scale(rows)
}

// layoutBody 按当前页的高度设置滚动条，再把控件摆到 -scrollPos。
// SetScrollInfo 显隐滚动条时会同步发 WM_SIZE，laying 用来避免重入。
func (s *settingsWindow) layoutBody() {
	if s == nil || s.body == 0 || s.laying {
		return
	}
	s.laying = true
	defer func() { s.laying = false }()

	content := s.pageContentHeight()
	view := clientHeight(s.body)
	s.scrollPos = clampScroll(s.scrollPos, content, view)
	setScrollBar(s.body, s.scrollPos, content, view)
	// 滚动条出现或消失会改客户区宽度，这里再读一次，输入框才不会被裁掉。
	width := clientWidth(s.body)
	top := -s.scrollPos
	s.placeGeneral(top, width)
	s.process.place(s, top, width)
	s.network.place(s, top, width)
	s.placeAbout(top, width)
}

// clampScroll 把滚动位置限制在内容能到达的范围内。
// content 是内容总高，view 是可见高度。内容不高于可见区域时位置为 0。
func clampScroll(pos, content, view int) int {
	if view < 0 {
		view = 0
	}
	if content <= view || pos <= 0 {
		return 0
	}
	maxPos := content - view
	if pos > maxPos {
		return maxPos
	}
	return pos
}

func (s *settingsWindow) onVScroll(wParam uintptr) {
	code := wParam & 0xFFFF
	content := s.pageContentHeight()
	view := clientHeight(s.body)
	line := s.scale(layoutRowH + layoutRowGap)
	if line < 1 {
		line = 1
	}
	page := view - line
	if page < line {
		page = line
	}
	pos := s.scrollPos
	switch code {
	case sbLineUp:
		pos -= line
	case sbLineDown:
		pos += line
	case sbPageUp:
		pos -= page
	case sbPageDown:
		pos += page
	case sbThumbTrack, sbThumbPosition:
		// wParam 的高字只有 16 位。内容更高时要用 GetScrollInfo 的跟踪位置。
		pos = scrollTrackPos(s.body)
	case sbTop:
		pos = 0
	case sbBottom:
		pos = content
	default:
		return
	}
	pos = clampScroll(pos, content, view)
	if pos == s.scrollPos {
		return
	}
	s.scrollPos = pos
	s.layoutBody()
}

func (s *settingsWindow) onMouseWheel(wParam uintptr) {
	delta := int(int16(wParam >> 16))
	if delta == 0 {
		return
	}
	// 高分辨率滚轮一次可能不到 120。攒够一格再滚，避免每条消息都跳一行。
	s.wheelAcc += delta
	notches := s.wheelAcc / wheelDelta
	s.wheelAcc -= notches * wheelDelta
	if notches == 0 {
		return
	}
	line := s.scale(layoutRowH + layoutRowGap)
	if line < 1 {
		line = 1
	}
	next := clampScroll(s.scrollPos-notches*line*3, s.pageContentHeight(), clientHeight(s.body))
	if next == s.scrollPos {
		return
	}
	s.scrollPos = next
	s.layoutBody()
}

// handleSettingsWheel 在消息循环里吃掉滚轮。
// 焦点在输入框上时，滚轮消息发给输入框，单行输入框不会把页面滚起来。
func (h *host) handleSettingsWheel(m *msg) bool {
	s := h.settings
	if s == nil || s.hwnd == 0 || m.Message != wmMouseWheel || !s.owns(m.Hwnd) {
		return false
	}
	if s.dropDownOpen() {
		return false
	}
	s.onMouseWheel(m.WParam)
	return true
}

func (s *settingsWindow) scrollIntoView(hwnd uintptr) {
	if s.body == 0 || hwnd == 0 || !s.inBody(hwnd) {
		return
	}
	ctrl := windowRectOf(hwnd)
	body := windowRectOf(s.body)
	top := int(ctrl.Top - body.Top)
	height := int(ctrl.Bottom - ctrl.Top)
	// 下拉框的窗口高度含收起的列表。滚进视野时只看关闭时的那一行。
	if strings.EqualFold(classNameOf(hwnd), "ComboBox") {
		height = s.scale(layoutRowH)
	}
	view := clientHeight(s.body)
	if height > view {
		height = view
	}
	if height < 1 {
		height = 1
	}
	pos := s.scrollPos
	switch {
	case top < 0:
		pos += top
	case top+height > view:
		pos += top + height - view
	default:
		return
	}
	pos = clampScroll(pos, s.pageContentHeight(), view)
	if pos == s.scrollPos {
		return
	}
	s.scrollPos = pos
	s.layoutBody()
}

func (s *settingsWindow) inBody(hwnd uintptr) bool {
	for hwnd != 0 && hwnd != s.hwnd {
		if hwnd == s.body {
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

// resizeForDPI 按新旧 DPI 的比例改高度，位置不动，也不去读系统建议的矩形。
func (s *settingsWindow) resizeForDPI(old uint32) {
	if s.hwnd == 0 || old == 0 {
		return
	}
	rc := windowRectOf(s.hwnd)
	height := int(rc.Bottom-rc.Top) * int(s.dpi) / int(old)
	width, minH, maxH := s.trackSize()
	if height < minH {
		height = minH
	}
	if height > maxH {
		height = maxH
	}
	procSetWindowPos.Call(s.hwnd, 0, 0, 0, uintptr(width), uintptr(height), swpNoMove|swpNoZOrder)
}

func (s *settingsWindow) minClientHeight() int32 {
	body := 2 * (layoutRowH + layoutRowGap)
	return int32(s.contentTop() + s.scale(body) + s.scale(8+layoutBtnH+layoutMargin))
}

func (s *settingsWindow) trackSize() (width, minH, maxH int) {
	clientW, clientH := s.clientSize()
	frameW, naturalH := frameSize(clientW, clientH, s.dpi)
	_, minFrame := frameSize(clientW, s.minClientHeight(), s.dpi)
	return int(frameW), int(minFrame), int(naturalH)
}

func (s *settingsWindow) onGetMinMaxInfo(lParam uintptr) {
	size := int(unsafe.Sizeof(minMaxInfo{}))
	raw := copyFromMemory(lParam, size)
	if len(raw) < size {
		return
	}
	var info minMaxInfo
	copy(structBytes(&info), raw)
	width, minH, maxH := s.trackSize()
	if width < 1 || minH < 1 || maxH < minH {
		return
	}
	info.minTrackX = int32(width)
	info.minTrackY = int32(minH)
	info.maxTrackX = int32(width)
	info.maxTrackY = int32(maxH)
	copyToMemory(lParam, structBytes(&info))
}

func structBytes(v *minMaxInfo) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(v)), unsafe.Sizeof(*v))
}

// adjustHitTest 去掉左右和四个角的拖动，只留上下边。
func adjustHitTest(code uintptr) uintptr {
	switch code {
	case htLeft, htRight:
		return htBorder
	case htTopLeft, htTopRight:
		return htTop
	case htBottomLeft, htBottomRight:
		return htBottom
	default:
		return code
	}
}

func setScrollBar(hwnd uintptr, pos, content, view int) {
	if hwnd == 0 {
		return
	}
	if content < 1 {
		content = 1
	}
	if view < 1 {
		view = 1
	}
	if pos < 0 {
		pos = 0
	}
	info := scrollInfo{
		Size: uint32(unsafe.Sizeof(scrollInfo{})),
		Mask: sifRange | sifPage | sifPos,
		Min:  0,
		Max:  int32(content - 1),
		Page: uint32(view),
		Pos:  int32(pos),
	}
	procSetScrollInfo.Call(hwnd, sbVert, uintptr(unsafe.Pointer(&info)), 1)
	runtime.KeepAlive(&info)
}

func scrollTrackPos(hwnd uintptr) int {
	info := scrollInfo{
		Size: uint32(unsafe.Sizeof(scrollInfo{})),
		Mask: sifTrackPos,
	}
	procGetScrollInfo.Call(hwnd, sbVert, uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(&info)
	return int(info.TrackPos)
}

func clientSizeOf(hwnd uintptr) (int, int) {
	var area rect
	if hwnd != 0 {
		procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&area)))
	}
	runtime.KeepAlive(&area)
	if area.Right < 0 {
		area.Right = 0
	}
	if area.Bottom < 0 {
		area.Bottom = 0
	}
	return int(area.Right), int(area.Bottom)
}

func clientWidth(hwnd uintptr) int {
	w, _ := clientSizeOf(hwnd)
	return w
}

func clientHeight(hwnd uintptr) int {
	_, h := clientSizeOf(hwnd)
	return h
}

func windowRectOf(hwnd uintptr) rect {
	var area rect
	if hwnd != 0 {
		procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&area)))
	}
	runtime.KeepAlive(&area)
	return area
}
