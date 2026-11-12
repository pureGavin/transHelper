// Package app 把一次热键触发编排成：读取、翻译、写回、失败则保持原样。
// 它不调用 Win32，桌面操作都通过 Desktop 完成，方便用假实现单测。
package app

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"transhelper/internal/translate"
)

const defaultTimeout = 20 * time.Second

// Action 是热键对应的动作。
type Action int

const (
	// ActionTranslateOnly 只把译文写回输入框。
	ActionTranslateOnly Action = iota
	// ActionTranslateAndSend 写回译文后再按回车。
	ActionTranslateAndSend
)

var (
	// ErrModifiersHeld 表示修饰键还按着，此时不能注入 Ctrl 组合键。
	ErrModifiersHeld = errors.New("修饰键未松开")
	// ErrForegroundChanged 表示读取前前台窗口已经换过，调用方不应再模拟按键。
	ErrForegroundChanged = errors.New("前台窗口已切换")
)

// Desktop 是对当前输入框和剪贴板的操作。
// Capture 负责备份剪贴板并读出输入框文本，文本使用 LF 换行。
// Restore 在任何路径上都会被调用，没有备份时应什么都不做。
type Desktop interface {
	Capture() (text string, foreground uintptr, err error)
	ForegroundChanged(foreground uintptr) bool
	Paste(text string) error
	PressEnter() error
	Restore() error
}

// Translator 把一段文本译成英文。
type Translator interface {
	Translate(ctx context.Context, text string) (string, error)
}

// Notifier 向用户显示一条短提示。
type Notifier interface {
	Notify(message string)
}

// Runner 执行一次热键触发的完整流程。
type Runner struct {
	Desktop    Desktop
	Translator Translator
	Notifier   Notifier
	// Timeout 是翻译步骤的超时。小于等于 0 时按 20 秒处理。
	Timeout time.Duration
	// Log 为空时不写日志。日志只记错误、耗时和字符数，不记消息内容。
	Log *log.Logger
}

// Run 处理一次热键。失败时不粘贴、不按回车，并尽量还原剪贴板。
func (r *Runner) Run(ctx context.Context, action Action) {
	defer func() {
		if err := r.Desktop.Restore(); err != nil {
			r.logf("还原剪贴板失败: %v", err)
			r.notify("剪贴板可能没有还原")
		}
	}()
	if ctx.Err() != nil {
		return
	}

	text, foreground, err := r.Desktop.Capture()
	if err != nil {
		r.logf("读取输入框失败: %v", err)
		r.notify(captureMessage(err))
		return
	}
	if strings.TrimSpace(text) == "" {
		r.logf("输入框为空")
		r.notify("输入框为空，未翻译")
		return
	}

	tctx, cancel := context.WithTimeout(ctx, r.timeout())
	start := time.Now()
	translated, err := r.Translator.Translate(tctx, text)
	cancel()
	if err != nil {
		r.logf("翻译失败: %v", err)
		r.notify(translate.UserMessage(err))
		return
	}
	r.logf("翻译完成，%d 个字符，耗时 %s", utf8.RuneCountInString(text), time.Since(start).Round(time.Millisecond))

	if r.Desktop.ForegroundChanged(foreground) {
		r.logf("前台窗口已变化，取消写回")
		r.notify("窗口已切换，没有写回")
		return
	}
	if err := r.Desktop.Paste(translated); err != nil {
		r.logf("写回失败: %v", err)
		r.notify("写回输入框失败，请检查原文")
		return
	}
	if action != ActionTranslateAndSend {
		return
	}
	if err := r.Desktop.PressEnter(); err != nil {
		r.logf("发送失败: %v", err)
		r.notify("译文已写回，但回车没有发出")
	}
}

func (r *Runner) timeout() time.Duration {
	if r.Timeout <= 0 {
		return defaultTimeout
	}
	return r.Timeout
}

func (r *Runner) logf(format string, args ...any) {
	if r.Log != nil {
		r.Log.Printf(format, args...)
	}
}

func (r *Runner) notify(message string) {
	if r.Notifier != nil && message != "" {
		r.Notifier.Notify(message)
	}
}

func captureMessage(err error) string {
	switch {
	case errors.Is(err, ErrModifiersHeld):
		return "修饰键未松开，已取消"
	case errors.Is(err, ErrForegroundChanged):
		return "窗口已切换，已取消"
	default:
		return "读取输入框失败，原文未改动"
	}
}
