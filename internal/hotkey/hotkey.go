// Package hotkey 把配置里的热键字符串解析成修饰键和虚拟键码。
// 数值与 Win32 的 MOD_*、VK_* 一致，但这里不调用任何系统 API，方便单测。
package hotkey

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// ModAlt 对应 Win32 MOD_ALT。
	ModAlt uint32 = 0x0001
	// ModCtrl 对应 Win32 MOD_CONTROL。
	ModCtrl uint32 = 0x0002
	// ModShift 对应 Win32 MOD_SHIFT。
	ModShift uint32 = 0x0004
	// ModWin 对应 Win32 MOD_WIN。
	ModWin uint32 = 0x0008
)

// Combo 是解析后的热键。
type Combo struct {
	// Mod 是修饰键的组合，取 ModAlt、ModCtrl、ModShift、ModWin。
	Mod uint32
	// VK 是主键的虚拟键码。
	VK uint16
	// Key 是主键的显示名，例如 "Enter"、"T"。
	Key string
}

// Same 报告两个热键是否会注册成同一个系统热键。
// "Alt+Enter" 和 "Alt+Return" 算同一个。
func (c Combo) Same(other Combo) bool {
	return c.Mod == other.Mod && c.VK == other.VK
}

// String 返回规范化后的写法，修饰键顺序固定为 Ctrl、Alt、Shift、Win。
func (c Combo) String() string {
	parts := make([]string, 0, 5)
	if c.Mod&ModCtrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if c.Mod&ModAlt != 0 {
		parts = append(parts, "Alt")
	}
	if c.Mod&ModShift != 0 {
		parts = append(parts, "Shift")
	}
	if c.Mod&ModWin != 0 {
		parts = append(parts, "Win")
	}
	if c.Key != "" {
		parts = append(parts, c.Key)
	}
	return strings.Join(parts, "+")
}

// Parse 把 "Alt+Enter" 这种写法解析成 Combo。
// 大小写不敏感，加号两边可以有空格。必须恰好有一个主键，
// 并且至少带 Ctrl、Alt、Win 中的一个：只带 Shift 会挡住正常打字。
func Parse(spec string) (Combo, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return Combo{}, fmt.Errorf("热键不能为空")
	}

	parts := strings.Split(spec, "+")
	var combo Combo
	var seenMod uint32
	keys := 0
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return Combo{}, fmt.Errorf("热键 %q 的格式无效", spec)
		}
		token := strings.ToLower(part)
		mod, ok := modifier(token)
		if ok {
			if seenMod&mod != 0 {
				return Combo{}, fmt.Errorf("热键 %q 里有重复的修饰键", spec)
			}
			seenMod |= mod
			combo.Mod |= mod
			continue
		}
		name, vk, ok := primaryKey(token)
		if !ok {
			return Combo{}, fmt.Errorf("热键 %q 里有无法识别的按键 %q", spec, part)
		}
		keys++
		if keys > 1 {
			return Combo{}, fmt.Errorf("热键 %q 只能有一个主键", spec)
		}
		combo.Key = name
		combo.VK = vk
	}
	if keys != 1 {
		return Combo{}, fmt.Errorf("热键 %q 缺少主键", spec)
	}
	if combo.Mod&(ModCtrl|ModAlt|ModWin) == 0 {
		return Combo{}, fmt.Errorf("热键 %q 至少要带 Ctrl、Alt 或 Win 中的一个", spec)
	}
	return combo, nil
}

func modifier(token string) (uint32, bool) {
	switch token {
	case "ctrl", "control":
		return ModCtrl, true
	case "alt":
		return ModAlt, true
	case "shift":
		return ModShift, true
	case "win", "windows":
		return ModWin, true
	default:
		return 0, false
	}
}

func primaryKey(token string) (string, uint16, bool) {
	if len(token) == 1 {
		r := token[0]
		switch {
		case r >= 'a' && r <= 'z':
			return strings.ToUpper(token), uint16(r - 'a' + 0x41), true
		case r >= '0' && r <= '9':
			return token, uint16(r), true
		}
	}
	if strings.HasPrefix(token, "f") && len(token) > 1 {
		n, err := strconv.Atoi(token[1:])
		if err == nil && n >= 1 && n <= 24 {
			return "F" + strconv.Itoa(n), uint16(0x70 + n - 1), true
		}
	}
	if key, ok := namedKeys[token]; ok {
		return key.name, key.vk, true
	}
	return "", 0, false
}

type namedKey struct {
	name string
	vk   uint16
}

// namedKeys 的键码与 Win32 VK_* 一致。
var namedKeys = map[string]namedKey{
	"enter":    {"Enter", 0x0D},
	"return":   {"Enter", 0x0D},
	"space":    {"Space", 0x20},
	"tab":      {"Tab", 0x09},
	"left":     {"Left", 0x25},
	"up":       {"Up", 0x26},
	"right":    {"Right", 0x27},
	"down":     {"Down", 0x28},
	"home":     {"Home", 0x24},
	"end":      {"End", 0x23},
	"pageup":   {"PageUp", 0x21},
	"pagedown": {"PageDown", 0x22},
	"insert":   {"Insert", 0x2D},
	"delete":   {"Delete", 0x2E},
	"del":      {"Delete", 0x2E},
}
