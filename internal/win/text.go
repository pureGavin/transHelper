//go:build windows

package win

import "strings"

// toLF 把剪贴板里的 CRLF 收成 LF，再送给模型。
func toLF(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// toCRLF 写回输入框前把换行转回 Windows 的 CRLF。
func toCRLF(s string) string {
	return strings.ReplaceAll(toLF(s), "\n", "\r\n")
}
