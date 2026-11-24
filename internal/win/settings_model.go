//go:build windows

package win

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"transhelper/internal/config"
	"transhelper/internal/translate"
)

// testOutcome 是一次模型测试的结果，回到界面线程后再显示。
type testOutcome struct {
	gen     uint64
	text    string
	elapsed time.Duration
	err     error
}

// collect 读取四个标签上当前填的值。bad 为 true 时，page 和 focus 指出该改哪里。
// 不读配置文件，也不保存。
func (s *settingsWindow) collect() (config.Config, int, uintptr, string, bool) {
	seconds, err := strconv.Atoi(strings.TrimSpace(windowText(s.general.timeout)))
	if err != nil || seconds < 1 || seconds > 120 {
		return config.Config{}, pageGeneral, s.general.timeout, "超时必须是 1 到 120 的整数", true
	}
	cfg := s.base
	cfg.BaseURL = strings.TrimSpace(windowText(s.general.url))
	cfg.Model = strings.TrimSpace(windowText(s.general.model))
	cfg.APIKey = strings.TrimSpace(windowText(s.general.key))
	cfg.Timeout = time.Duration(seconds) * time.Second
	cfg.Theme = selectedTheme(s.general.theme)
	// 窗口开着时，菜单可能已经改过自动检查。以内存里的为准，避免应用时把它改回去。
	if h := globalHost; h != nil {
		cfg.AutoCheckUpdate = h.cfg.AutoCheckUpdate
	}
	if cfg.BaseURL == "" {
		return cfg, pageGeneral, s.general.url, "请填写模型地址", true
	}
	if !acceptableModelURL(cfg.BaseURL) {
		return cfg, pageGeneral, s.general.url, "模型地址必须是 http 或 https", true
	}
	if cfg.Model == "" {
		return cfg, pageGeneral, s.general.model, "请填写模型名称", true
	}
	proxy, focus, message, bad := s.network.read()
	if bad {
		return cfg, pageNetwork, focus, message, true
	}
	cfg.Proxy = proxy
	cfg.Processes = append([]string(nil), s.process.enabledNames...)
	return cfg, 0, 0, "", false
}

func acceptableModelURL(raw string) bool {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

// testModel 用输入框里的值试译一句。失败或成功都只改按钮旁边的文字，不保存。
func (s *settingsWindow) testModel() {
	h := globalHost
	if s == nil || s.hwnd == 0 || h == nil || s.testBusy {
		return
	}
	cfg, page, focus, message, bad := s.collect()
	if bad {
		s.reject(page, focus, message)
		return
	}
	if h.newTranslator == nil {
		s.alert("翻译失败")
		return
	}
	translator := h.newTranslator(cfg)
	if translator == nil {
		s.alert("翻译失败")
		return
	}
	s.stopModelTest()
	s.testGen++
	gen := s.testGen
	ctx, cancel := context.WithTimeout(h.ctx, cfg.Timeout)
	s.testCancel = cancel
	s.testBusy = true
	enableWindow(s.general.test, false)
	setWindowText(s.general.status, "正在测试…")
	go func() {
		start := time.Now()
		text, err := translator.Translate(ctx, translate.CheckSample)
		s.pushTest(testOutcome{gen: gen, text: text, elapsed: time.Since(start), err: err})
	}()
}

// noteModelFieldChanged 在地址、模型、密钥或超时被改动时清掉上次的结果。
func (s *settingsWindow) noteModelFieldChanged() {
	if s == nil {
		return
	}
	s.cancelModelTest()
	enableWindow(s.general.test, true)
	setWindowText(s.general.status, "")
}

func (s *settingsWindow) cancelModelTest() {
	if s == nil {
		return
	}
	s.testGen++
	s.stopModelTest()
	s.testBusy = false
}

func (s *settingsWindow) stopModelTest() {
	if s.testCancel != nil {
		s.testCancel()
		s.testCancel = nil
	}
}

func (s *settingsWindow) pushTest(item testOutcome) {
	h := globalHost
	if h == nil || h.hwnd == 0 {
		return
	}
	s.testMu.Lock()
	s.testQueue = append(s.testQueue, item)
	s.testMu.Unlock()
	procPostMessage.Call(h.hwnd, wmTestResult, 0, 0)
}

func (s *settingsWindow) onTestResult() {
	s.testMu.Lock()
	items := s.testQueue
	s.testQueue = nil
	s.testMu.Unlock()
	for _, item := range items {
		if item.gen != s.testGen || s.hwnd == 0 {
			continue
		}
		s.showTestResult(item)
	}
}

func (s *settingsWindow) showTestResult(item testOutcome) {
	s.testBusy = false
	s.stopModelTest()
	enableWindow(s.general.test, true)
	setWindowText(s.general.status, translate.DescribeCheck(item.text, item.elapsed, item.err))
	h := globalHost
	if h == nil || h.logger == nil {
		return
	}
	elapsed := item.elapsed.Round(time.Millisecond)
	if item.err != nil {
		// 只记类别和耗时。响应正文可能含密钥或原文，不写日志。
		h.logger.Printf("模型测试失败: %s，耗时 %s", translate.CheckKind(item.err), elapsed)
		return
	}
	h.logger.Printf("模型测试成功，耗时 %s", elapsed)
}
