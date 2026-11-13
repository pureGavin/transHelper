//go:build windows

package win

import (
	"os"

	"golang.org/x/sys/windows"

	"transhelper/internal/update"
)

// aboutPage 是「关于」页，显示版本、Go 版本、exe 的 SHA256 和仓库链接。
type aboutPage struct {
	labels  [4]uintptr
	version uintptr
	goVer   uintptr
	digest  uintptr
	link    uintptr
}

func (a aboutPage) controls() []uintptr {
	out := []uintptr{a.version, a.goVer, a.digest, a.link}
	return append(out, a.labels[:]...)
}

func aboutBody() int {
	// 版本和 Go 版本各一行，SHA256 和 GitHub 链接各两行，每行后面留间距。
	return 2*(layoutRowH+layoutRowGap) + layoutDigestH + layoutRowGap + layoutLinkH + layoutRowGap
}

func (s *settingsWindow) createAbout(instance windows.Handle) error {
	labelStyle := uintptr(wsChild | wsVisible | ssCenterImage)
	labels := []string{"版本", "Go 版本", "SHA256", "GitHub"}
	for i, text := range labels {
		hwnd, err := createControl("STATIC", text, labelStyle, 0, 0, 10, 10, s.hwnd, 0, instance)
		if err != nil {
			return err
		}
		s.about.labels[i] = hwnd
	}
	var err error
	if s.about.version, err = createControl("STATIC", "", labelStyle, 0, 0, 10, 10, s.hwnd, 0, instance); err != nil {
		return err
	}
	if s.about.goVer, err = createControl("STATIC", "", labelStyle, 0, 0, 10, 10, s.hwnd, 0, instance); err != nil {
		return err
	}
	// 只读多行，可以选中复制。不设 ES_AUTOHSCROLL，64 位十六进制会折行。
	digestStyle := uintptr(wsChild | wsVisible | wsTabStop | wsBorder | esReadOnly | esMultiLine | esAutoVScroll)
	if s.about.digest, err = createControl("EDIT", "", digestStyle, 0, 0, 10, 10, s.hwnd, 0, instance); err != nil {
		return err
	}
	linkText := `<A HREF="` + update.RepoURL + `">` + update.RepoURL + `</A>`
	linkStyle := uintptr(wsChild | wsVisible | wsTabStop)
	if s.about.link, err = createControl("SysLink", linkText, linkStyle, 0, 0, 10, 10, s.hwnd, idLink, instance); err != nil {
		return err
	}
	return nil
}

func (s *settingsWindow) placeAbout() {
	rows := []struct {
		label  uintptr
		value  uintptr
		height int
	}{
		{s.about.labels[0], s.about.version, layoutRowH},
		{s.about.labels[1], s.about.goVer, layoutRowH},
		{s.about.labels[2], s.about.digest, layoutDigestH},
		{s.about.labels[3], s.about.link, layoutLinkH},
	}
	labelX := s.scale(layoutMargin)
	valueX := s.scale(layoutMargin + layoutLabelW + layoutGap)
	y := s.contentTop()
	for _, row := range rows {
		h := s.scale(row.height)
		moveWindow(row.label, labelX, y, s.scale(layoutLabelW), s.scale(layoutRowH))
		moveWindow(row.value, valueX, y, s.scale(layoutEditW), h)
		y += h + s.scale(layoutRowGap)
	}
}

func (a aboutPage) fill(version, goVersion, digest string) {
	if version == "" {
		version = "dev"
	}
	setWindowText(a.version, version)
	setWindowText(a.goVer, goVersion)
	setWindowText(a.digest, digest)
	procSendMessage.Call(a.digest, emSetLimitText, 128, 0)
}

// openLink 打开写死的仓库地址，不用链接文字里的地址。
func (a aboutPage) openLink() {
	openBrowser(update.RepoURL)
}

// fileDigest 返回当前 exe 的 SHA256。读不到时返回「无法读取」。
// 第一次打开设置时计算，之后沿用结果。
func (h *host) fileDigest() string {
	if h == nil {
		return "无法读取"
	}
	h.digestOnce.Do(func() {
		exe, err := os.Executable()
		if err != nil {
			return
		}
		sum, err := update.FileSHA256(exe)
		if err != nil {
			return
		}
		h.exeDigest = sum
	})
	if h.exeDigest == "" {
		return "无法读取"
	}
	return h.exeDigest
}
