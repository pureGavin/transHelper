//go:build windows

package win

import (
	"time"

	"transhelper/internal/app"
)

// Desktop 实现 app.Desktop。同一次触发里只能有一个调用方。
type Desktop struct {
	// Expect 是热键发生时的前台窗口。等待修饰键期间如果它变了，就不再模拟按键。
	Expect uintptr

	backup []clipItem
	backed bool
	pasted bool
}

// Capture 备份剪贴板并读出输入框里的文本。
func (d *Desktop) Capture() (string, uintptr, error) {
	fg := d.Expect
	if fg == 0 {
		fg = currentForeground()
	}
	if err := waitModifiersReleased(modifierWait); err != nil {
		return "", 0, err
	}
	if currentForeground() != fg {
		return "", 0, app.ErrForegroundChanged
	}
	items, err := backupClipboard()
	if err != nil {
		return "", 0, err
	}
	d.backup = items
	d.backed = true
	d.pasted = false
	text, err := copySelection()
	if err != nil {
		return "", 0, err
	}
	return text, fg, nil
}

// ForegroundChanged 报告前台窗口是不是已经不是捕获时的那个。
func (d *Desktop) ForegroundChanged(token uintptr) bool {
	if token == 0 {
		return true
	}
	return currentForeground() != token
}

// Paste 把文本写进剪贴板，再全选并粘贴。文本里的 LF 会转成 CRLF。
func (d *Desktop) Paste(text string) error {
	if err := writeClipboardText(toCRLF(text)); err != nil {
		return err
	}
	if err := chord(vkLControl, vkA); err != nil {
		return err
	}
	time.Sleep(selectDelay)
	if err := chord(vkLControl, vkV); err != nil {
		return err
	}
	d.pasted = true
	return nil
}

// PressEnter 在粘贴落地之后补一次回车。
func (d *Desktop) PressEnter() error {
	time.Sleep(pasteDelay)
	return tap(vkReturn)
}

// Restore 还原 Capture 时备份的剪贴板。没有备份时什么都不做。
func (d *Desktop) Restore() error {
	if !d.backed {
		return nil
	}
	if d.pasted {
		time.Sleep(restoreDelay)
	}
	err := restoreClipboard(d.backup)
	d.backup = nil
	d.backed = false
	d.pasted = false
	return err
}
