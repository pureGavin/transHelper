//go:build windows

package win

import (
	"encoding/binary"
	"testing"

	"golang.org/x/sys/windows"
)

func TestClipboardTextRoundTrip(t *testing.T) {
	original, err := backupClipboard()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := restoreClipboard(original); err != nil {
			t.Errorf("还原剪贴板: %v", err)
		}
	})

	const marker = "transhelper-clipboard-test"
	if err := writeClipboardText(marker); err != nil {
		t.Fatal(err)
	}
	got, err := readClipboardText()
	if err != nil {
		t.Fatal(err)
	}
	if got != marker {
		t.Fatalf("读回 %q", got)
	}

	if err := restoreClipboard(original); err != nil {
		t.Fatal(err)
	}
	got, err = readClipboardText()
	if err != nil {
		t.Fatal(err)
	}
	if got != clipboardUnicode(original) {
		t.Fatalf("还原后 = %q，期望 %q", got, clipboardUnicode(original))
	}
}

func clipboardUnicode(items []clipItem) string {
	for _, item := range items {
		if item.format != cfUnicodeText || len(item.data) < 2 {
			continue
		}
		units := make([]uint16, len(item.data)/2)
		for i := range units {
			units[i] = binary.LittleEndian.Uint16(item.data[i*2:])
		}
		return windows.UTF16ToString(units)
	}
	return ""
}
