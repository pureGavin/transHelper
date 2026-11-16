//go:build windows && uidrive

package win

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"
)

const pwRenderFull = 2

var (
	procPrintWindow            = user32.NewProc("PrintWindow")
	procGetWindowDC            = user32.NewProc("GetWindowDC")
	procReleaseDC              = user32.NewProc("ReleaseDC")
	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procGetDIBits              = gdi32.NewProc("GetDIBits")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
)

// bitmapInfoHeader 对应 BITMAPINFOHEADER。后面不再跟调色板，因为用的是 32 位色。
type bitmapInfoHeader struct {
	Size         uint32
	Width        int32
	Height       int32
	Planes       uint16
	BitCount     uint16
	Compression  uint32
	SizeImage    uint32
	Xppm         int32
	Yppm         int32
	ClrUsed      uint32
	ClrImportant uint32
}

// Shot 把设置窗口画进 PNG。只用来看外观，不靠它定位控件。
func (a *App) Shot(path string) error {
	hwnd, err := a.requireSettings()
	if err != nil {
		return err
	}
	var rc rect
	ok, _, callErr := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	runtime.KeepAlive(&rc)
	if ok == 0 {
		return fmt.Errorf("读取窗口大小: %v", callErr)
	}
	width := int(rc.Right - rc.Left)
	height := int(rc.Bottom - rc.Top)
	if width < 1 || height < 1 {
		return fmt.Errorf("窗口大小无效")
	}
	pixels, err := captureWindow(hwnd, width, height)
	if err != nil {
		return err
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := 0; i+3 < len(pixels) && i+3 < len(img.Pix); i += 4 {
		img.Pix[i] = pixels[i+2]
		img.Pix[i+1] = pixels[i+1]
		img.Pix[i+2] = pixels[i]
		img.Pix[i+3] = 255
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	encodeErr := png.Encode(file, img)
	closeErr := file.Close()
	if encodeErr != nil {
		return encodeErr
	}
	return closeErr
}

func captureWindow(hwnd uintptr, width, height int) ([]byte, error) {
	hdc, _, _ := procGetWindowDC.Call(hwnd)
	if hdc == 0 {
		return nil, fmt.Errorf("GetWindowDC 失败")
	}
	defer procReleaseDC.Call(hwnd, hdc)

	mem, _, _ := procCreateCompatibleDC.Call(hdc)
	if mem == 0 {
		return nil, fmt.Errorf("CreateCompatibleDC 失败")
	}
	defer procDeleteDC.Call(mem)

	bmp, _, _ := procCreateCompatibleBitmap.Call(hdc, uintptr(width), uintptr(height))
	if bmp == 0 {
		return nil, fmt.Errorf("CreateCompatibleBitmap 失败")
	}
	defer procDeleteObject.Call(bmp)

	old, _, _ := procSelectObject.Call(mem, bmp)
	printed, _, printErr := procPrintWindow.Call(hwnd, mem, pwRenderFull)
	if old != 0 {
		procSelectObject.Call(mem, old)
	}
	if printed == 0 {
		return nil, fmt.Errorf("PrintWindow: %v", printErr)
	}

	header := bitmapInfoHeader{
		Size:     uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:    int32(width),
		Height:   -int32(height),
		Planes:   1,
		BitCount: 32,
	}
	pixels := make([]byte, width*height*4)
	got, _, bitErr := procGetDIBits.Call(
		hdc,
		bmp,
		0,
		uintptr(height),
		uintptr(unsafe.Pointer(&pixels[0])),
		uintptr(unsafe.Pointer(&header)),
		0,
	)
	runtime.KeepAlive(&header)
	runtime.KeepAlive(pixels)
	if got == 0 {
		return nil, fmt.Errorf("GetDIBits: %v", bitErr)
	}
	return pixels, nil
}
