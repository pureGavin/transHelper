package app

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
	"time"

	"transhelper/internal/translate"
)

type fakeDesktop struct {
	text       string
	foreground uintptr
	captureErr error
	changed    bool
	pasteErr   error
	enterErr   error
	restoreErr error
	pasted     string
	calls      []string
}

func (f *fakeDesktop) Capture() (string, uintptr, error) {
	f.calls = append(f.calls, "capture")
	return f.text, f.foreground, f.captureErr
}

func (f *fakeDesktop) ForegroundChanged(uintptr) bool {
	f.calls = append(f.calls, "foreground")
	return f.changed
}

func (f *fakeDesktop) Paste(text string) error {
	f.calls = append(f.calls, "paste")
	f.pasted = text
	return f.pasteErr
}

func (f *fakeDesktop) PressEnter() error {
	f.calls = append(f.calls, "enter")
	return f.enterErr
}

func (f *fakeDesktop) Restore() error {
	f.calls = append(f.calls, "restore")
	return f.restoreErr
}

type fakeTranslator struct {
	out         string
	err         error
	calls       int
	got         string
	hadDeadline bool
}

func (f *fakeTranslator) Translate(ctx context.Context, text string) (string, error) {
	f.calls++
	f.got = text
	_, f.hadDeadline = ctx.Deadline()
	return f.out, f.err
}

type fakeNotifier struct {
	msgs []string
}

func (f *fakeNotifier) Notify(message string) {
	f.msgs = append(f.msgs, message)
}

func TestRunTranslateOnly(t *testing.T) {
	desktop := &fakeDesktop{text: "你好", foreground: 7}
	translator := &fakeTranslator{out: "Hello"}
	notifier := &fakeNotifier{}
	var logs bytes.Buffer
	runner := &Runner{
		Desktop:    desktop,
		Translator: translator,
		Notifier:   notifier,
		Timeout:    time.Second,
		Log:        log.New(&logs, "", 0),
	}

	runner.Run(context.Background(), ActionTranslateOnly)

	if translator.calls != 1 || translator.got != "你好" || !translator.hadDeadline {
		t.Fatalf("翻译调用 = %d %q deadline=%v", translator.calls, translator.got, translator.hadDeadline)
	}
	if desktop.pasted != "Hello" {
		t.Fatalf("粘贴内容 = %q", desktop.pasted)
	}
	if strings.Join(desktop.calls, ",") != "capture,foreground,paste,restore" {
		t.Fatalf("调用顺序 = %v", desktop.calls)
	}
	if len(notifier.msgs) != 0 {
		t.Fatalf("成功时不应提示: %v", notifier.msgs)
	}
	if strings.Contains(logs.String(), "你好") || strings.Contains(logs.String(), "Hello") {
		t.Fatalf("日志包含了消息内容: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "2 个字符") {
		t.Fatalf("日志 = %s", logs.String())
	}
}

func TestRunTranslateAndSendPressesEnterAfterPaste(t *testing.T) {
	desktop := &fakeDesktop{text: "你好", foreground: 7}
	translator := &fakeTranslator{out: "Hello"}
	runner := &Runner{Desktop: desktop, Translator: translator, Timeout: time.Second}

	runner.Run(context.Background(), ActionTranslateAndSend)

	if strings.Join(desktop.calls, ",") != "capture,foreground,paste,enter,restore" {
		t.Fatalf("调用顺序 = %v", desktop.calls)
	}
	if desktop.pasted != "Hello" {
		t.Fatalf("粘贴内容 = %q", desktop.pasted)
	}
}

func TestRunTranslateErrorDoesNotPasteOrSend(t *testing.T) {
	desktop := &fakeDesktop{text: "你好", foreground: 7}
	translator := &fakeTranslator{err: &translate.Error{Kind: translate.KindHTTP, Status: 503}}
	notifier := &fakeNotifier{}
	runner := &Runner{Desktop: desktop, Translator: translator, Notifier: notifier, Timeout: time.Second}

	runner.Run(context.Background(), ActionTranslateAndSend)

	if strings.Join(desktop.calls, ",") != "capture,restore" {
		t.Fatalf("调用顺序 = %v", desktop.calls)
	}
	if desktop.pasted != "" {
		t.Fatalf("失败时不应粘贴，实际 %q", desktop.pasted)
	}
	if len(notifier.msgs) != 1 || notifier.msgs[0] != "模型返回 HTTP 503，原文未改动" {
		t.Fatalf("提示 = %v", notifier.msgs)
	}
}

func TestRunEmptyInputSkipsModel(t *testing.T) {
	desktop := &fakeDesktop{text: " \r\n\t "}
	translator := &fakeTranslator{out: "Hello"}
	notifier := &fakeNotifier{}
	runner := &Runner{Desktop: desktop, Translator: translator, Notifier: notifier}

	runner.Run(context.Background(), ActionTranslateAndSend)

	if translator.calls != 0 {
		t.Fatalf("空输入不应调用模型，调用了 %d 次", translator.calls)
	}
	if strings.Join(desktop.calls, ",") != "capture,restore" {
		t.Fatalf("调用顺序 = %v", desktop.calls)
	}
	if len(notifier.msgs) != 1 || !strings.Contains(notifier.msgs[0], "输入框为空") {
		t.Fatalf("提示 = %v", notifier.msgs)
	}
}

func TestRunForegroundChangedDoesNotPaste(t *testing.T) {
	desktop := &fakeDesktop{text: "你好", foreground: 7, changed: true}
	translator := &fakeTranslator{out: "Hello"}
	notifier := &fakeNotifier{}
	runner := &Runner{Desktop: desktop, Translator: translator, Notifier: notifier, Timeout: time.Second}

	runner.Run(context.Background(), ActionTranslateAndSend)

	if translator.calls != 1 {
		t.Fatal("窗口切换前应该已经翻译")
	}
	if strings.Join(desktop.calls, ",") != "capture,foreground,restore" {
		t.Fatalf("调用顺序 = %v", desktop.calls)
	}
	if len(notifier.msgs) != 1 || notifier.msgs[0] != "窗口已切换，没有写回" {
		t.Fatalf("提示 = %v", notifier.msgs)
	}
}

func TestRunCaptureErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "修饰键", err: ErrModifiersHeld, want: "修饰键未松开，已取消"},
		{name: "窗口切换", err: ErrForegroundChanged, want: "窗口已切换，已取消"},
		{name: "其他", err: errRead, want: "读取输入框失败，原文未改动"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desktop := &fakeDesktop{captureErr: tt.err}
			translator := &fakeTranslator{out: "Hello"}
			notifier := &fakeNotifier{}
			runner := &Runner{Desktop: desktop, Translator: translator, Notifier: notifier}
			runner.Run(context.Background(), ActionTranslateAndSend)
			if translator.calls != 0 {
				t.Fatal("读取失败时不应翻译")
			}
			if strings.Join(desktop.calls, ",") != "capture,restore" {
				t.Fatalf("调用顺序 = %v", desktop.calls)
			}
			if len(notifier.msgs) != 1 || notifier.msgs[0] != tt.want {
				t.Fatalf("提示 = %v", notifier.msgs)
			}
		})
	}
}

var errRead = errString("read failed")

type errString string

func (e errString) Error() string { return string(e) }

func TestRunRestoreErrorStillReported(t *testing.T) {
	desktop := &fakeDesktop{text: "你好", restoreErr: errRead}
	translator := &fakeTranslator{err: &translate.Error{Kind: translate.KindTimeout}}
	notifier := &fakeNotifier{}
	runner := &Runner{Desktop: desktop, Translator: translator, Notifier: notifier, Timeout: time.Second}

	runner.Run(context.Background(), ActionTranslateOnly)

	if len(notifier.msgs) != 2 {
		t.Fatalf("提示 = %v", notifier.msgs)
	}
	if notifier.msgs[0] != "翻译超时，原文未改动" || notifier.msgs[1] != "剪贴板可能没有还原" {
		t.Fatalf("提示 = %v", notifier.msgs)
	}
}

func TestRunCanceledBeforeCapture(t *testing.T) {
	desktop := &fakeDesktop{text: "你好"}
	translator := &fakeTranslator{out: "Hello"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := &Runner{Desktop: desktop, Translator: translator}

	runner.Run(ctx, ActionTranslateOnly)

	if translator.calls != 0 {
		t.Fatal("已取消时不应翻译")
	}
	if strings.Join(desktop.calls, ",") != "restore" {
		t.Fatalf("调用顺序 = %v", desktop.calls)
	}
}
