//go:build windows

package win

import (
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	clipboardAttempts  = 10
	clipboardRetryWait = 20 * time.Millisecond
	// maxClipboardBytes 避免某个异常格式把内存撑爆。超过的格式不备份。
	maxClipboardBytes = 64 << 20
)

type clipItem struct {
	format uint32
	data   []byte
}

func callOK(proc *windows.LazyProc, args ...uintptr) error {
	r, _, err := proc.Call(args...)
	if r != 0 {
		return nil
	}
	if err == nil || errors.Is(err, windows.ERROR_SUCCESS) {
		return fmt.Errorf("%s 失败", proc.Name)
	}
	return err
}

func openClipboard() error {
	var last error
	for i := 0; i < clipboardAttempts; i++ {
		r, _, err := procOpenClipboard.Call(0)
		// 剪贴板查看器会在 OpenClipboard 内部重入，把剪贴板关掉，函数却仍可能返回成功。
		// 不确认当前线程还持有它，后面的读写会报“没有打开剪贴板”。
		if r != 0 && ownsClipboard() {
			return nil
		}
		last = err
		procCloseClipboard.Call()
		time.Sleep(clipboardRetryWait)
	}
	if last == nil || errors.Is(last, windows.ERROR_SUCCESS) {
		last = errors.New("剪贴板被其他程序占用")
	}
	return fmt.Errorf("打开剪贴板: %w", last)
}

func ownsClipboard() bool {
	procSetLastError.Call(0)
	r, _, err := procEnumClipboardFormats.Call(0)
	if r != 0 {
		return true
	}
	return !errors.Is(err, windows.ERROR_CLIPBOARD_NOT_OPEN)
}

func closeClipboard() {
	procCloseClipboard.Call()
}

func withClipboard(fn func() error) error {
	// 剪贴板按线程打开。goroutine 可能在打开和读写之间换到另一条系统线程。
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// 没有消息队列时，查看器同步发来的消息没法处理，剪贴板状态会乱。
	ensureMessageQueue()
	if err := openClipboard(); err != nil {
		return err
	}
	defer closeClipboard()
	return fn()
}

// copyFromMemory 把 GlobalLock 得到的地址拷进 Go 内存。
// 这块内存不属于 Go 堆，不能直接把 uintptr 转成指针，否则 vet 会报警，GC 也不认识它。
func copyFromMemory(addr uintptr, n int) []byte {
	if addr == 0 || n <= 0 {
		return nil
	}
	var view []byte
	header := (*reflect.SliceHeader)(unsafe.Pointer(&view))
	header.Data = addr
	header.Len = n
	header.Cap = n
	out := make([]byte, n)
	copy(out, view)
	return out
}

func copyToMemory(addr uintptr, data []byte) {
	if addr == 0 || len(data) == 0 {
		return
	}
	var view []byte
	header := (*reflect.SliceHeader)(unsafe.Pointer(&view))
	header.Data = addr
	header.Len = len(data)
	header.Cap = len(data)
	copy(view, data)
}

func skipClipboardFormat(format uint32) bool {
	switch format {
	case cfBitmap, cfMetafilePict, cfPalette, cfEnhMetafile, cfOwnerDisplay, cfDSPBitmap, cfDSPMetafile, cfDSPEnhMetafile:
		// 这些格式存的是 GDI 句柄，不能按字节拷贝。位图数据仍在 CF_DIB 里，系统会自己合成 CF_BITMAP。
		return true
	default:
		return false
	}
}

func backupClipboard() ([]clipItem, error) {
	var items []clipItem
	err := withClipboard(func() error {
		var format uintptr
		for {
			next, _, _ := procEnumClipboardFormats.Call(format)
			if next == 0 {
				return nil
			}
			format = next
			if skipClipboardFormat(uint32(format)) {
				continue
			}
			handle, _, _ := procGetClipboardData.Call(format)
			if handle == 0 {
				continue
			}
			size, _, _ := procGlobalSize.Call(handle)
			if size == 0 || size > maxClipboardBytes {
				continue
			}
			ptr, _, _ := procGlobalLock.Call(handle)
			if ptr == 0 {
				continue
			}
			buf := copyFromMemory(ptr, int(size))
			procGlobalUnlock.Call(handle)
			items = append(items, clipItem{format: uint32(format), data: buf})
		}
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

func setClipboardBytes(format uint32, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	handle, _, err := procGlobalAlloc.Call(gmemMoveable, uintptr(len(data)))
	if handle == 0 {
		if err == nil || errors.Is(err, windows.ERROR_SUCCESS) {
			return errors.New("GlobalAlloc 失败")
		}
		return err
	}
	ptr, _, err := procGlobalLock.Call(handle)
	if ptr == 0 {
		procGlobalFree.Call(handle)
		if err == nil || errors.Is(err, windows.ERROR_SUCCESS) {
			return errors.New("GlobalLock 失败")
		}
		return err
	}
	copyToMemory(ptr, data)
	procGlobalUnlock.Call(handle)
	if err := callOK(procSetClipboardData, uintptr(format), handle); err != nil {
		// 失败时所有权还在我们这边，必须释放。成功则系统接管这块内存。
		procGlobalFree.Call(handle)
		return err
	}
	return nil
}

var (
	fmtHistory uint32
	fmtMonitor uint32
)

func clipboardExclusionFormats() (uint32, uint32) {
	if fmtHistory == 0 {
		fmtHistory = registerClipboardFormat("CanIncludeInClipboardHistory")
	}
	if fmtMonitor == 0 {
		fmtMonitor = registerClipboardFormat("ExcludeClipboardContentFromMonitorProcessing")
	}
	return fmtHistory, fmtMonitor
}

func registerClipboardFormat(name string) uint32 {
	u := utf16String(name)
	r, _, _ := procRegisterClipboardFormat.Call(uintptr(unsafe.Pointer(&u[0])))
	runtime.KeepAlive(u)
	return uint32(r)
}

// setClipboardExclusions 让这次写入不进 Win+V 历史，也不进剪贴板云同步。
// 调用时剪贴板必须已经打开。值为 0 表示排除。
func setClipboardExclusions() error {
	history, monitor := clipboardExclusionFormats()
	zero := []byte{0, 0, 0, 0}
	if history != 0 {
		if err := setClipboardBytes(history, zero); err != nil {
			return err
		}
	}
	if monitor != 0 {
		if err := setClipboardBytes(monitor, zero); err != nil {
			return err
		}
	}
	return nil
}

func writeClipboardText(text string) error {
	encoded, err := windows.UTF16FromString(text)
	if err != nil {
		return err
	}
	buf := make([]byte, len(encoded)*2)
	for i, unit := range encoded {
		binary.LittleEndian.PutUint16(buf[i*2:], unit)
	}
	return withClipboard(func() error {
		if err := callOK(procEmptyClipboard); err != nil {
			return err
		}
		if err := setClipboardBytes(cfUnicodeText, buf); err != nil {
			return err
		}
		return setClipboardExclusions()
	})
}

func readClipboardText() (string, error) {
	var text string
	err := withClipboard(func() error {
		handle, _, _ := procGetClipboardData.Call(cfUnicodeText)
		if handle == 0 {
			return nil
		}
		ptr, _, _ := procGlobalLock.Call(handle)
		if ptr == 0 {
			return nil
		}
		defer procGlobalUnlock.Call(handle)
		size, _, _ := procGlobalSize.Call(handle)
		text = utf16FromMemory(ptr, int(size))
		return nil
	})
	return text, err
}

func utf16FromMemory(addr uintptr, nbytes int) string {
	buf := copyFromMemory(addr, nbytes)
	if len(buf) < 2 {
		return ""
	}
	units := make([]uint16, len(buf)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(buf[i*2:])
	}
	return windows.UTF16ToString(units)
}

func restoreClipboard(items []clipItem) error {
	return withClipboard(func() error {
		if err := callOK(procEmptyClipboard); err != nil {
			return err
		}
		for _, item := range items {
			if err := setClipboardBytes(item.format, item.data); err != nil {
				return err
			}
		}
		return setClipboardExclusions()
	})
}

// copySelection 清空剪贴板后发送 Ctrl+A、Ctrl+C，再用序列号确认复制确实发生了。
// 序列号要在清空之后取样：清空本身就会让它加一。
func copySelection() (string, error) {
	if err := withClipboard(func() error {
		return callOK(procEmptyClipboard)
	}); err != nil {
		return "", err
	}
	before, _, _ := procGetClipboardSequenceNumber.Call()
	if err := chord(vkLControl, vkA); err != nil {
		return "", err
	}
	time.Sleep(selectDelay)
	if err := chord(vkLControl, vkC); err != nil {
		return "", err
	}
	deadline := time.Now().Add(copyWait)
	copied := false
	for {
		seq, _, _ := procGetClipboardSequenceNumber.Call()
		if seq != before {
			copied = true
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(clipboardRetryWait)
	}
	if !copied {
		// 没有选中文字时，Ctrl+C 不会改剪贴板。这和复制失败对用户来说都是“没有文本”。
		return "", nil
	}
	text, err := readClipboardText()
	if err != nil {
		return "", err
	}
	return toLF(text), nil
}
