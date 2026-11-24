//go:build windows

package win

import (
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	lbsNotify           = 0x0001
	lbsNoIntegralHeight = 0x0100

	lbAddString     = 0x0180
	lbResetContent  = 0x0184
	lbSetCurSel     = 0x0186
	lbGetCurSel     = 0x0188
	lbGetText       = 0x0189
	lbGetTextLen    = 0x018A
	lbGetCount      = 0x018B
	lbGetTopIndex   = 0x018E
	lbGetItemHeight = 0x01A1

	bnClicked    = 0
	lbnSelChange = 1
	lbnDblClk    = 2

	idProcApps    = 301
	idProcAdd     = 302
	idProcRemove  = 303
	idProcEnabled = 304
)

// processPage 是「进程」页。左边是当前有窗口的程序，右边是启用翻译的程序。
type processPage struct {
	runningLabel uintptr
	running      uintptr
	add          uintptr
	remove       uintptr
	enabledLabel uintptr
	enabled      uintptr
	// enabledNames 是右边列表，顺序会写回 target.processes。
	enabledNames []string
	// movedBack 是这次从右边移回左边、当时可能没在运行的进程。刷新左边时保留它们。
	movedBack []string
}

func (p processPage) controls() []uintptr {
	return []uintptr{p.runningLabel, p.running, p.add, p.remove, p.enabledLabel, p.enabled}
}

func processBody() int {
	return 7 * (layoutRowH + layoutRowGap)
}

// processPageWidth 是进程页需要的客户区宽度。
// 每个列表留出约 26 个拉丁字符，再加竖向滚动条。
func processPageWidth() int {
	const listW = 210
	const buttonW = 40
	return layoutMargin*2 + listW*2 + layoutGap*2 + buttonW
}

func (s *settingsWindow) createProcess(instance windows.Handle) error {
	labelStyle := uintptr(wsChild | wsVisible | ssCenterImage)
	listStyle := uintptr(wsChild | wsVisible | wsTabStop | wsBorder | wsVScroll | lbsNotify | lbsNoIntegralHeight)
	btnStyle := uintptr(wsChild | wsVisible | wsTabStop)
	var err error
	if s.process.runningLabel, err = createControl("STATIC", "运行中的程序", labelStyle, 0, 0, 10, 10, s.body, 0, instance); err != nil {
		return err
	}
	if s.process.running, err = createControl("LISTBOX", "", listStyle, 0, 0, 10, 10, s.body, idProcApps, instance); err != nil {
		return err
	}
	if s.process.add, err = createControl("BUTTON", "→", btnStyle, 0, 0, 10, 10, s.body, idProcAdd, instance); err != nil {
		return err
	}
	if s.process.remove, err = createControl("BUTTON", "←", btnStyle, 0, 0, 10, 10, s.body, idProcRemove, instance); err != nil {
		return err
	}
	if s.process.enabledLabel, err = createControl("STATIC", "启用翻译的程序", labelStyle, 0, 0, 10, 10, s.body, 0, instance); err != nil {
		return err
	}
	if s.process.enabled, err = createControl("LISTBOX", "", listStyle, 0, 0, 10, 10, s.body, idProcEnabled, instance); err != nil {
		return err
	}
	s.process.syncButtons()
	return nil
}

func (p *processPage) place(s *settingsWindow, top, width int) {
	margin := s.scale(layoutMargin)
	gap := s.scale(layoutGap)
	labelH := s.scale(layoutRowH)
	btnW := s.scale(40)
	btnH := s.scale(layoutBtnH)
	listH := s.scale(processBody()) - labelH - s.scale(layoutRowGap)
	if listH < labelH {
		listH = labelH
	}
	inner := width - 2*margin
	listW := (inner - btnW - 2*gap) / 2
	if listW < s.scale(40) {
		listW = s.scale(40)
	}
	listTop := top + labelH
	moveWindow(p.runningLabel, margin, top, listW, labelH)
	moveWindow(p.running, margin, listTop, listW, listH)
	btnX := margin + listW + gap
	btnY := listTop + listH/2 - btnH - gap/2
	moveWindow(p.add, btnX, btnY, btnW, btnH)
	moveWindow(p.remove, btnX, btnY+btnH+gap, btnW, btnH)
	rightX := btnX + btnW + gap
	moveWindow(p.enabledLabel, rightX, top, listW, labelH)
	moveWindow(p.enabled, rightX, listTop, listW, listH)
}

func (p *processPage) show(on bool) {
	showControls(p.controls(), on)
}

func (p *processPage) fill(enabled []string) {
	p.enabledNames = append([]string(nil), enabled...)
	p.movedBack = nil
	p.refresh()
}

func (p *processPage) tips() []tipItem {
	return []tipItem{
		{p.running, "只列出当前有窗口的程序"},
		{p.enabled, "不在这里的程序都不开启翻译"},
		{p.add, "把左边选中的程序加入右边"},
		{p.remove, "把右边选中的程序移回左边"},
	}
}

// refresh 按当前运行的程序和右边列表重画两个框。
func (p *processPage) refresh() {
	if p == nil || p.running == 0 || p.enabled == 0 {
		return
	}
	setList(p.running, availableProcesses(windowedProcessNames(), p.movedBack, p.enabledNames))
	setList(p.enabled, p.enabledNames)
	p.syncButtons()
}

func (p *processPage) onCommand(id, code uintptr) {
	switch id {
	case idProcAdd:
		if code == bnClicked {
			p.moveToEnabled()
		}
	case idProcRemove:
		if code == bnClicked {
			p.moveToAvailable()
		}
	case idProcApps:
		if code == lbnDblClk {
			p.moveToEnabled()
			return
		}
		if code == lbnSelChange {
			p.syncButtons()
		}
	case idProcEnabled:
		if code == lbnDblClk {
			p.moveToAvailable()
			return
		}
		if code == lbnSelChange {
			p.syncButtons()
		}
	}
}

func (p *processPage) moveToEnabled() {
	name := selectedListText(p.running)
	if name == "" {
		return
	}
	_, enabled, ok := takeProcess([]string{name}, p.enabledNames, name)
	if !ok {
		return
	}
	p.enabledNames = enabled
	moved, _, _ := takeProcess(p.movedBack, nil, name)
	p.movedBack = moved
	p.refresh()
	selectListName(p.enabled, name)
	p.syncButtons()
}

func (p *processPage) moveToAvailable() {
	name := selectedListText(p.enabled)
	if name == "" {
		return
	}
	enabled, _, ok := takeProcess(p.enabledNames, nil, name)
	if !ok {
		return
	}
	p.enabledNames = enabled
	_, moved, _ := takeProcess([]string{name}, p.movedBack, name)
	p.movedBack = moved
	p.refresh()
	selectListName(p.running, name)
	p.syncButtons()
}

func (p *processPage) syncButtons() {
	enableWindow(p.add, selectedListText(p.running) != "")
	enableWindow(p.remove, selectedListText(p.enabled) != "")
}

func setList(hwnd uintptr, items []string) {
	if hwnd == 0 {
		return
	}
	procSendMessage.Call(hwnd, lbResetContent, 0, 0)
	for _, item := range items {
		text := utf16String(item)
		procSendMessage.Call(hwnd, lbAddString, 0, uintptr(unsafe.Pointer(&text[0])))
		runtime.KeepAlive(text)
	}
}

func selectListName(hwnd uintptr, name string) {
	count, _, _ := procSendMessage.Call(hwnd, lbGetCount, 0, 0)
	if int32(count) <= 0 {
		return
	}
	for i := 0; i < int(count); i++ {
		if strings.EqualFold(listItem(hwnd, i), name) {
			procSendMessage.Call(hwnd, lbSetCurSel, uintptr(i), 0)
			return
		}
	}
}

func selectedListText(hwnd uintptr) string {
	if hwnd == 0 {
		return ""
	}
	sel, _, _ := procSendMessage.Call(hwnd, lbGetCurSel, 0, 0)
	if int32(sel) < 0 {
		return ""
	}
	return listItem(hwnd, int(sel))
}

func listItem(hwnd uintptr, index int) string {
	n, _, _ := procSendMessage.Call(hwnd, lbGetTextLen, uintptr(index), 0)
	if int32(n) <= 0 {
		return ""
	}
	buf := make([]uint16, int(n)+1)
	procSendMessage.Call(hwnd, lbGetText, uintptr(index), uintptr(unsafe.Pointer(&buf[0])))
	runtime.KeepAlive(buf)
	return windows.UTF16ToString(buf)
}
