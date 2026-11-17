//go:build windows

package win

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ERROR_DYNAMIC_CODE_BLOCKED。禁止动态代码后，申请可执行页会返回这个错误。
const dynamicCodeBlocked = windows.Errno(1655)

var procGetProcessMitigationPolicy = kernel32.NewProc("GetProcessMitigationPolicy")

func TestHarden(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestHardenChild$")
	cmd.Env = append(os.Environ(), "TRANSHELPER_HARDEN_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("子进程: %v\n%s", err, out)
	}
}

func TestHardenChild(t *testing.T) {
	if os.Getenv("TRANSHELPER_HARDEN_CHILD") != "1" {
		t.Skip("只在 Harden 的子进程里运行")
	}
	if err := Harden(); err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name   string
		policy uintptr
		want   uint32
	}{
		{"映像加载", processImageLoadPolicy, noRemoteImages | noLowMandatoryLabelImages | preferSystem32Images},
		{"签名", processSignaturePolicy, microsoftSignedOnly},
		{"动态代码", processDynamicCodePolicy, prohibitDynamicCode},
	}
	for _, check := range checks {
		got, err := mitigationFlags(check.policy)
		if err != nil {
			t.Fatalf("%s: %v", check.name, err)
		}
		if got&check.want != check.want {
			t.Fatalf("%s: 标志 %#x，期望包含 %#x", check.name, got, check.want)
		}
	}
	addr, err := windows.VirtualAlloc(0, 4096, windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_EXECUTE_READWRITE)
	if addr != 0 {
		_ = windows.VirtualFree(addr, 0, windows.MEM_RELEASE)
		t.Fatal("禁止动态代码后仍能申请可执行内存")
	}
	if !errors.Is(err, dynamicCodeBlocked) {
		t.Fatalf("申请可执行内存: %v", err)
	}
}

func mitigationFlags(policy uintptr) (uint32, error) {
	var flags uint32
	r, _, err := procGetProcessMitigationPolicy.Call(
		uintptr(windows.CurrentProcess()),
		policy,
		uintptr(unsafe.Pointer(&flags)),
		unsafe.Sizeof(flags),
	)
	runtime.KeepAlive(&flags)
	if r == 0 {
		return 0, err
	}
	return flags, nil
}
