//go:build windows

package win

import (
	"strings"
	"testing"
	"unsafe"

	"transhelper/internal/config"
)

func TestResolveDark(t *testing.T) {
	if !resolveDark(config.ThemeDark, false) {
		t.Fatal("深色应该保持深色")
	}
	if resolveDark(config.ThemeLight, true) {
		t.Fatal("浅色不应该跟着系统变成深色")
	}
	if !resolveDark(config.ThemeSystem, true) || resolveDark(config.ThemeSystem, false) {
		t.Fatal("跟随系统应该用系统当前的深浅")
	}
	if !resolveDark("其他", true) || resolveDark("其他", false) {
		t.Fatal("不认识的值应该按跟随系统处理")
	}
}

func TestCheckboxPart(t *testing.T) {
	if checkboxPart(false, true, false, false) != 1 {
		t.Fatal("未勾选常态应该是 1")
	}
	if checkboxPart(false, true, true, false) != 2 || checkboxPart(false, true, false, true) != 3 {
		t.Fatal("未勾选的悬停和按下状态不对")
	}
	if checkboxPart(false, false, true, true) != 4 {
		t.Fatal("禁用应该盖过悬停和按下")
	}
	if checkboxPart(true, true, false, false) != 5 || checkboxPart(true, false, false, false) != 8 {
		t.Fatal("勾选状态应该从 5 开始")
	}
}

func TestBreakAtSlash(t *testing.T) {
	got := breakAtSlash("https://github.com/pureGavin/transHelper")
	if !strings.Contains(got, "\n") || strings.Contains(got, "\n/") {
		t.Fatalf("链接没有在斜杠后换行: %q", got)
	}
	if breakAtSlash("没有斜杠") != "没有斜杠" {
		t.Fatal("没有斜杠时不应该拆开")
	}
	if breakAtSlash("ab") != "ab" {
		t.Fatal("太短的文字不应该拆开")
	}
}

func TestThemeStructLayout(t *testing.T) {
	if unsafe.Sizeof(paintStruct{}) != 72 {
		t.Fatalf("PAINTSTRUCT = %d，期望 72", unsafe.Sizeof(paintStruct{}))
	}
	if unsafe.Offsetof(paintStruct{}.rcPaint) != 12 {
		t.Fatalf("rcPaint 偏移 = %d，期望 12", unsafe.Offsetof(paintStruct{}.rcPaint))
	}
	if unsafe.Sizeof(nmCustomDraw{}) != 80 {
		t.Fatalf("NMCUSTOMDRAW = %d，期望 80", unsafe.Sizeof(nmCustomDraw{}))
	}
	if unsafe.Offsetof(nmCustomDraw{}.hdc) != 32 || unsafe.Offsetof(nmCustomDraw{}.rc) != 40 {
		t.Fatalf("NMCUSTOMDRAW hdc=%d rc=%d", unsafe.Offsetof(nmCustomDraw{}.hdc), unsafe.Offsetof(nmCustomDraw{}.rc))
	}
}
