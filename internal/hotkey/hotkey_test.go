package hotkey

import "testing"

func TestParseValid(t *testing.T) {
	tests := []struct {
		spec string
		mod  uint32
		vk   uint16
		key  string
		str  string
	}{
		{"Alt+Enter", ModAlt, 0x0D, "Enter", "Alt+Enter"},
		{"alt + enter", ModAlt, 0x0D, "Enter", "Alt+Enter"},
		{"ALT+RETURN", ModAlt, 0x0D, "Enter", "Alt+Enter"},
		{"Ctrl+Alt+T", ModCtrl | ModAlt, 0x54, "T", "Ctrl+Alt+T"},
		{"  win + space ", ModWin, 0x20, "Space", "Win+Space"},
		{"Ctrl+Shift+F12", ModCtrl | ModShift, 0x70 + 11, "F12", "Ctrl+Shift+F12"},
		{"Alt+F1", ModAlt, 0x70, "F1", "Alt+F1"},
		{"Alt+F24", ModAlt, 0x70 + 23, "F24", "Alt+F24"},
		{"Ctrl+1", ModCtrl, 0x31, "1", "Ctrl+1"},
		{"Alt+Left", ModAlt, 0x25, "Left", "Alt+Left"},
		{"Windows+Delete", ModWin, 0x2E, "Delete", "Win+Delete"},
		{"Control+Del", ModCtrl, 0x2E, "Delete", "Ctrl+Delete"},
		{"Ctrl+Alt+Shift+Win+PageUp", ModCtrl | ModAlt | ModShift | ModWin, 0x21, "PageUp", "Ctrl+Alt+Shift+Win+PageUp"},
	}
	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			got, err := Parse(tt.spec)
			if err != nil {
				t.Fatalf("Parse(%q) 返回错误: %v", tt.spec, err)
			}
			if got.Mod != tt.mod || got.VK != tt.vk || got.Key != tt.key {
				t.Fatalf("Parse(%q) = {Mod:%#x VK:%#x Key:%q}, 期望 {Mod:%#x VK:%#x Key:%q}",
					tt.spec, got.Mod, got.VK, got.Key, tt.mod, tt.vk, tt.key)
			}
			if got.String() != tt.str {
				t.Fatalf("String() = %q, 期望 %q", got.String(), tt.str)
			}
		})
	}
}

func TestParseEnterAndReturnAreSame(t *testing.T) {
	enter, err := Parse("Alt+Enter")
	if err != nil {
		t.Fatal(err)
	}
	ret, err := Parse("Alt+Return")
	if err != nil {
		t.Fatal(err)
	}
	if !enter.Same(ret) {
		t.Fatal("Enter 和 Return 应该是同一个热键")
	}
}

func TestParseInvalid(t *testing.T) {
	specs := []string{
		"",
		"   ",
		"Shift+A",
		"Alt",
		"Alt+Shift",
		"Alt+A+B",
		"Alt++Enter",
		"Alt+Foo",
		"Alt+F0",
		"Alt+F25",
		"Alt+Ctrl+Alt+T",
		"+",
		"A",
	}
	for _, spec := range specs {
		t.Run(spec, func(t *testing.T) {
			if _, err := Parse(spec); err == nil {
				t.Fatalf("Parse(%q) 应该失败", spec)
			}
		})
	}
}
