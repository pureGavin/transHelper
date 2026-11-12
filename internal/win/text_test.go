//go:build windows

package win

import "testing"

func TestNewlines(t *testing.T) {
	if got := toLF("a\r\nb\rc"); got != "a\nb\nc" {
		t.Fatalf("toLF = %q", got)
	}
	if got := toCRLF("a\nb\r\nc"); got != "a\r\nb\r\nc" {
		t.Fatalf("toCRLF = %q", got)
	}
}
