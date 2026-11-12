//go:build windows

package win

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func currentForeground() uintptr {
	hwnd, _, _ := procGetForegroundWindow.Call()
	return hwnd
}

// foregroundProcessName 返回前台窗口所属进程的文件名。
// 打不开进程时返回空字符串，调用方会把它当成不在白名单里。
func foregroundProcessName() string {
	hwnd := currentForeground()
	if hwnd == 0 {
		return ""
	}
	var pid uint32
	windows.GetWindowThreadProcessId(windows.HWND(hwnd), &pid)
	if pid == 0 {
		return ""
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(handle)

	buf := make([]uint16, 32768)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &size); err != nil || size == 0 || size > uint32(len(buf)) {
		return ""
	}
	return filepath.Base(windows.UTF16ToString(buf[:size]))
}
