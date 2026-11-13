//go:build windows

package win

import (
	"testing"
	"unsafe"
)

func TestWin32StructSizes(t *testing.T) {
	checks := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"KEYBDINPUT", unsafe.Sizeof(keybdInput{}), 24},
		{"INPUT", unsafe.Sizeof(keyboardEvent{}), 40},
		{"MSG", unsafe.Sizeof(msg{}), 48},
		{"WNDCLASSEXW", unsafe.Sizeof(wndClassEx{}), 80},
		{"NOTIFYICONDATAW", unsafe.Sizeof(notifyIconData{}), 976},
		{"LOGFONTW", unsafe.Sizeof(logFont{}), 92},
		{"NONCLIENTMETRICSW", unsafe.Sizeof(nonClientMetrics{}), 504},
		{"MONITORINFO", unsafe.Sizeof(monitorInfo{}), 40},
		{"TOOLINFOW", unsafe.Sizeof(toolInfo{}), 72},
		{"TCITEMW", unsafe.Sizeof(tabItem{}), 40},
		{"NMHDR", unsafe.Sizeof(nmHdr{}), 24},
	}
	for _, check := range checks {
		if check.got != check.want {
			t.Errorf("%s 大小 = %d，期望 %d", check.name, check.got, check.want)
		}
	}
}

func TestSkipClipboardFormat(t *testing.T) {
	if !skipClipboardFormat(cfBitmap) || !skipClipboardFormat(cfEnhMetafile) {
		t.Fatal("GDI 句柄格式应该跳过")
	}
	if skipClipboardFormat(cfUnicodeText) {
		t.Fatal("文本格式不应该跳过")
	}
}
