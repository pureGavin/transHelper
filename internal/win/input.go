//go:build windows

package win

import (
	"errors"
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"transhelper/internal/app"
)

const (
	// modifierWait 是等用户松开修饰键的最长时间。
	// 没松开就注入 Ctrl+A，会和手上的 Alt 组成另一个快捷键。
	modifierWait = 3 * time.Second
	// copyWait 是等目标程序处理 Ctrl+C 的时间。
	copyWait = time.Second
	// selectDelay 给目标程序一点时间完成全选，再复制或粘贴。
	selectDelay = 80 * time.Millisecond
	// pasteDelay 让目标程序先处理完 Ctrl+V，再按回车。
	pasteDelay = 100 * time.Millisecond
	// restoreDelay 必须晚于粘贴。目标程序是在处理 Ctrl+V 时才去读剪贴板的。
	restoreDelay = 300 * time.Millisecond
	keyPoll      = 10 * time.Millisecond
)

func sendInputs(events []keyboardEvent) error {
	if len(events) == 0 {
		return nil
	}
	r, _, err := procSendInput.Call(
		uintptr(len(events)),
		uintptr(unsafe.Pointer(&events[0])),
		unsafe.Sizeof(events[0]),
	)
	runtime.KeepAlive(&events[0])
	if r == uintptr(len(events)) {
		return nil
	}
	if err == nil || errors.Is(err, windows.ERROR_SUCCESS) {
		return fmt.Errorf("SendInput 只注入了 %d/%d 个事件", r, len(events))
	}
	return fmt.Errorf("SendInput: %w", err)
}

func keyEvent(vk uint16, flags uint32) keyboardEvent {
	return keyboardEvent{
		Type: inputKeyboard,
		Ki: keybdInput{
			Vk:    vk,
			Flags: flags,
		},
	}
}

func tap(vk uint16) error {
	return sendInputs([]keyboardEvent{
		keyEvent(vk, 0),
		keyEvent(vk, keyeventfKeyup),
	})
}

func chord(mod, key uint16) error {
	return sendInputs([]keyboardEvent{
		keyEvent(mod, 0),
		keyEvent(key, 0),
		keyEvent(key, keyeventfKeyup),
		keyEvent(mod, keyeventfKeyup),
	})
}

func keyDown(vk uint16) bool {
	r, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
	return r&0x8000 != 0
}

func altOrWinDown() bool {
	return keyDown(vkMenu) || keyDown(vkLMenu) || keyDown(vkRMenu) || keyDown(vkLWin) || keyDown(vkRWin)
}

func modifiersDown() bool {
	keys := []uint16{vkControl, vkLControl, vkRControl, vkMenu, vkLMenu, vkRMenu, vkShift, vkLShift, vkRShift, vkLWin, vkRWin}
	for _, vk := range keys {
		if keyDown(vk) {
			return true
		}
	}
	return false
}

// maskAltWin 在热键消息刚到达时调用。见 vkE8 的说明。
func maskAltWin() {
	if altOrWinDown() {
		_ = tap(vkE8)
	}
}

func waitModifiersReleased(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if !modifiersDown() {
			return nil
		}
		if time.Now().After(deadline) {
			return app.ErrModifiersHeld
		}
		time.Sleep(keyPoll)
	}
}

// ensureMessageQueue 让当前线程拥有消息队列。
// 线程第一次调用 user32 才会创建队列，没有队列时 OpenClipboard 偶尔会失败。
func ensureMessageQueue() {
	var m msg
	procPeekMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmNoRemove)
	runtime.KeepAlive(&m)
}
