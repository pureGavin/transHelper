//go:build windows

package win

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"
)

const (
	// 数值来自 PROCESS_MITIGATION_POLICY。这三个结构体都是一个 DWORD 的 Flags 联合，
	// 所以缓冲区就是一个 uint32。
	processDynamicCodePolicy = 2
	processSignaturePolicy   = 8
	processImageLoadPolicy   = 10

	prohibitDynamicCode       = 0x1
	microsoftSignedOnly       = 0x1
	noRemoteImages            = 0x1
	noLowMandatoryLabelImages = 0x2
	preferSystem32Images      = 0x4
)

// Harden 打开进程缓解策略：映像只能是微软签名的，优先从 System32 加载，
// 拒绝远程路径和低完整性映像，禁止运行时生成或修改代码。
// 策略对本进程不可撤销，不传给子进程。某项失败不影响其他项，错误合并返回。
func Harden() error {
	var errs []error
	// 先限制后面可能发生的加载，再关掉动态代码。
	// Go 的窗口回调用 exe 里的固定跳板，运行时不申请可执行页，所以可以禁止动态代码。
	if err := setMitigation(processImageLoadPolicy, noRemoteImages|noLowMandatoryLabelImages|preferSystem32Images); err != nil {
		errs = append(errs, fmt.Errorf("限制映像加载: %w", err))
	}
	if err := setMitigation(processSignaturePolicy, microsoftSignedOnly); err != nil {
		errs = append(errs, fmt.Errorf("只允许微软签名的映像: %w", err))
	}
	if err := setMitigation(processDynamicCodePolicy, prohibitDynamicCode); err != nil {
		errs = append(errs, fmt.Errorf("禁止动态代码: %w", err))
	}
	return errors.Join(errs...)
}

func setMitigation(policy uintptr, flags uint32) error {
	r, _, err := procSetProcessMitigationPolicy.Call(
		policy,
		uintptr(unsafe.Pointer(&flags)),
		unsafe.Sizeof(flags),
	)
	runtime.KeepAlive(&flags)
	if r == 0 {
		return err
	}
	return nil
}
