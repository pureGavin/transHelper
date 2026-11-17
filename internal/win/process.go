//go:build windows

package win

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// gwlExStyle 是 GWL_EXSTYLE，数值为 -20。工具窗口不代表用户正在用的程序。
const gwlExStyle = ^uintptr(19)

var (
	windowedCB   = windows.NewCallback(enumWindowed)
	windowedMu   sync.Mutex
	windowedPIDs []uint32
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
	return processBaseName(pid)
}

// windowedProcessNames 返回当前有独立可见窗口的进程文件名。
// 跳过有所有者的窗口和工具窗口，也跳过 transHelper 自己。同名进程只保留一个。
func windowedProcessNames() []string {
	pids := collectWindowedPIDs()
	self := os.Getpid()
	seen := make(map[string]struct{}, len(pids))
	var names []string
	for _, pid := range pids {
		if pid == 0 || int(pid) == self {
			continue
		}
		name := processBaseName(pid)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		names = append(names, name)
	}
	return names
}

func collectWindowedPIDs() []uint32 {
	windowedMu.Lock()
	defer windowedMu.Unlock()
	windowedPIDs = nil
	procEnumWindows.Call(windowedCB, 0)
	return append([]uint32(nil), windowedPIDs...)
}

func enumWindowed(hwnd, _ uintptr) uintptr {
	if !windowVisible(hwnd) {
		return 1
	}
	owner, _, _ := procGetWindow.Call(hwnd, gwOwner)
	if owner != 0 {
		return 1
	}
	ex, _, _ := procGetWindowLongPtr.Call(hwnd, gwlExStyle)
	if ex&wsExToolWindow != 0 {
		return 1
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid != 0 {
		windowedPIDs = append(windowedPIDs, pid)
	}
	return 1
}

func windowVisible(hwnd uintptr) bool {
	r, _, _ := procIsWindowVisible.Call(hwnd)
	return r != 0
}

func processBaseName(pid uint32) string {
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
