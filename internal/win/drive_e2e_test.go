//go:build windows && uidrive

package win

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"transhelper/internal/config"
	"transhelper/internal/translate"
)

func TestE2E_OpenSettings(t *testing.T) {
	app := newDriveApp(t)
	cfg := loadDriveConfig(t, app)
	state := openDriveSettings(t, app)
	if state.SettingsCount != 1 {
		t.Fatalf("设置窗口数量 = %d", state.SettingsCount)
	}
	if got := state.Controls["url"].Text; got != cfg.BaseURL {
		t.Fatalf("地址 = %q，配置是 %q", got, cfg.BaseURL)
	}
	if got := state.Controls["model"].Text; got != cfg.Model {
		t.Fatalf("模型 = %q，配置是 %q", got, cfg.Model)
	}
	wantTimeout := strconv.Itoa(int(cfg.Timeout / time.Second))
	if got := state.Controls["timeout"].Text; got != wantTimeout {
		t.Fatalf("超时 = %q，配置是 %s 秒", got, wantTimeout)
	}
	if got := state.Controls["theme"].Value; got != cfg.Theme {
		t.Fatalf("主题 = %q，配置是 %q", got, cfg.Theme)
	}
	if state.Controls["key"].Text != "" || state.Controls["key"].TextLen != 0 {
		t.Fatalf("空密钥不应出现在状态里: %+v", state.Controls["key"])
	}
	shot := filepath.Join(t.TempDir(), "settings.png")
	if err := app.Shot(shot); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(shot)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() < 100 {
		t.Fatalf("截图只有 %d 字节", info.Size())
	}

	hwnd := state.SettingsHWND
	if err := app.Menu("settings"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		again, err := app.State()
		if err != nil {
			t.Fatal(err)
		}
		if again.SettingsCount != 1 || again.SettingsHWND != hwnd {
			t.Fatalf("再次打开设置出现了另一个窗口: %+v", again)
		}
		time.Sleep(pollGap)
	}
}

func TestE2E_Apply(t *testing.T) {
	app := newDriveApp(t)
	openDriveSettings(t, app)
	mustDrive(t, app.Set("url", "http://localhost:8001/v1"))
	mustDrive(t, app.Set("model", "test-model"))
	mustDrive(t, app.Set("timeout", "15"))
	mustDrive(t, app.Click("apply"))
	if _, err := app.WaitSettings(false); err != nil {
		dialog, _ := app.Dialog()
		t.Fatalf("设置窗口没有关闭，弹窗 %q", dialog.Text)
	}
	if err := app.WaitLog("通知：应用设置成功"); err != nil {
		t.Fatal(err)
	}
	cfg := loadDriveConfig(t, app)
	if cfg.BaseURL != "http://localhost:8001/v1" || cfg.Model != "test-model" || cfg.Timeout != 15*time.Second {
		t.Fatalf("写回的配置不对: %+v", cfg)
	}
}

func TestE2E_EnterApplies(t *testing.T) {
	app := newDriveApp(t)
	openDriveSettings(t, app)
	mustDrive(t, app.Set("timeout", "11"))
	mustDrive(t, app.Key("enter"))
	if _, err := app.WaitSettings(false); err != nil {
		t.Fatal(err)
	}
	cfg := loadDriveConfig(t, app)
	if cfg.Timeout != 11*time.Second {
		t.Fatalf("回车没有写回超时: %s", cfg.Timeout)
	}
}

func TestE2E_Cancel(t *testing.T) {
	app := newDriveApp(t)
	openDriveSettings(t, app)
	before := driveConfigText(t, app)
	mustDrive(t, app.Set("url", "http://localhost:8001/v1"))
	mustDrive(t, app.Click("cancel"))
	if _, err := app.WaitSettings(false); err != nil {
		t.Fatal(err)
	}
	if got := driveConfigText(t, app); got != before {
		t.Fatal("取消后配置文件变了")
	}
}

func TestE2E_Escape(t *testing.T) {
	app := newDriveApp(t)
	openDriveSettings(t, app)
	before := driveConfigText(t, app)
	mustDrive(t, app.Set("url", "http://localhost:8001/v1"))
	mustDrive(t, app.Key("esc"))
	if _, err := app.WaitSettings(false); err != nil {
		t.Fatal(err)
	}
	if got := driveConfigText(t, app); got != before {
		t.Fatal("按 Esc 后配置文件变了")
	}
}

func TestE2E_Close(t *testing.T) {
	app := newDriveApp(t)
	openDriveSettings(t, app)
	before := driveConfigText(t, app)
	mustDrive(t, app.Set("model", "test-model"))
	mustDrive(t, app.Close())
	if _, err := app.WaitSettings(false); err != nil {
		t.Fatal(err)
	}
	if got := driveConfigText(t, app); got != before {
		t.Fatal("关闭窗口后配置文件变了")
	}
}

func TestE2E_Reject(t *testing.T) {
	app := newDriveApp(t)
	openDriveSettings(t, app)
	before := driveConfigText(t, app)

	mustDrive(t, app.Set("timeout", "0"))
	mustDrive(t, app.Click("apply"))
	dialog, err := app.WaitDialog()
	if err != nil {
		t.Fatal(err)
	}
	if dialog.Text != "超时必须是 1 到 120 的整数" {
		t.Fatalf("超时提示 = %q", dialog.Text)
	}
	state, err := app.State()
	if err != nil {
		t.Fatal(err)
	}
	if state.Page != "general" {
		t.Fatalf("超时错误后停在 %s", state.Page)
	}
	mustDrive(t, app.DialogOK())
	// 超时还是 0 时，后面的地址错误不会被检查到。
	mustDrive(t, app.Set("timeout", "20"))

	mustDrive(t, app.Set("url", ""))
	mustDrive(t, app.Click("apply"))
	dialog, err = app.WaitDialog()
	if err != nil {
		t.Fatal(err)
	}
	if dialog.Text != "请填写模型地址" {
		t.Fatalf("地址提示 = %q", dialog.Text)
	}
	state, err = app.State()
	if err != nil {
		t.Fatal(err)
	}
	if state.Page != "general" {
		t.Fatalf("地址错误后停在 %s", state.Page)
	}
	mustDrive(t, app.DialogOK())
	if _, err := app.WaitSettings(true); err != nil {
		t.Fatal(err)
	}
	if got := driveConfigText(t, app); got != before {
		t.Fatal("校验失败后配置文件变了")
	}
}

func TestE2E_Network(t *testing.T) {
	app := newDriveApp(t)
	openDriveSettings(t, app)
	mustDrive(t, app.Tab("network"))
	if err := waitDrivePage(t, app, "network"); err != nil {
		t.Fatal(err)
	}
	state, err := app.State()
	if err != nil {
		t.Fatal(err)
	}
	if state.Controls["proxyHost"].Enabled || state.Controls["proxyUser"].Enabled {
		t.Fatalf("没开代理时地址或用户名仍可用: %+v", state.Controls)
	}

	mustDrive(t, app.Click("proxyOn"))
	if _, err := app.WaitControl("proxyHost", func(ctrl ControlState) bool { return ctrl.Enabled }); err != nil {
		t.Fatal(err)
	}
	state, err = app.State()
	if err != nil {
		t.Fatal(err)
	}
	if state.Controls["proxyUser"].Enabled {
		t.Fatal("还没要求身份验证，用户名就已经可用")
	}

	mustDrive(t, app.Click("proxyAuth"))
	if _, err := app.WaitControl("proxyUser", func(ctrl ControlState) bool { return ctrl.Enabled }); err != nil {
		t.Fatal(err)
	}
	if _, err := app.WaitControl("proxyPass", func(ctrl ControlState) bool { return ctrl.Enabled }); err != nil {
		t.Fatal(err)
	}

	mustDrive(t, app.Click("apply"))
	dialog, err := app.WaitDialog()
	if err != nil {
		t.Fatal(err)
	}
	if dialog.Text != "请填写代理地址" {
		t.Fatalf("代理提示 = %q", dialog.Text)
	}
	state, err = app.State()
	if err != nil {
		t.Fatal(err)
	}
	if state.Page != "network" {
		t.Fatalf("代理错误后停在 %s", state.Page)
	}
	mustDrive(t, app.DialogOK())

	mustDrive(t, app.Set("proxyHost", "localhost"))
	mustDrive(t, app.Set("proxyPort", "8080"))
	mustDrive(t, app.Set("proxyUser", "proxy-user"))
	mustDrive(t, app.Set("proxyPass", "proxy-pass"))
	mustDrive(t, app.Select("proxyType", "socks5"))
	mustDrive(t, app.Click("proxyModel"))
	if _, err := app.WaitControl("proxyModel", func(ctrl ControlState) bool { return ctrl.Checked }); err != nil {
		t.Fatal(err)
	}
	mustDrive(t, app.Click("apply"))
	if _, err := app.WaitSettings(false); err != nil {
		t.Fatal(err)
	}
	cfg := loadDriveConfig(t, app)
	if !cfg.Proxy.Enabled || cfg.Proxy.Type != "socks5" || cfg.Proxy.Host != "localhost" || cfg.Proxy.Port != 8080 {
		t.Fatalf("代理没有写回: %+v", cfg.Proxy)
	}
	if !cfg.Proxy.Auth || cfg.Proxy.Username != "proxy-user" || cfg.Proxy.Password != "proxy-pass" || !cfg.Proxy.ForModel {
		t.Fatalf("代理身份验证没有写回: %+v", cfg.Proxy)
	}

	openDriveSettings(t, app)
	mustDrive(t, app.Tab("network"))
	if err := waitDrivePage(t, app, "network"); err != nil {
		t.Fatal(err)
	}
	mustDrive(t, app.Click("proxyOn"))
	if _, err := app.WaitControl("proxyHost", func(ctrl ControlState) bool { return !ctrl.Enabled }); err != nil {
		t.Fatal(err)
	}
	mustDrive(t, app.Click("apply"))
	if _, err := app.WaitSettings(false); err != nil {
		t.Fatal(err)
	}
	cfg = loadDriveConfig(t, app)
	if cfg.Proxy.Enabled || cfg.Proxy.Host != "localhost" || cfg.Proxy.Username != "proxy-user" || cfg.Proxy.Password != "proxy-pass" {
		t.Fatalf("关掉代理后已填内容没有保留: %+v", cfg.Proxy)
	}

	openDriveSettings(t, app)
	mustDrive(t, app.Tab("network"))
	host, err := app.WaitControl("proxyHost", func(ctrl ControlState) bool { return ctrl.Text == "localhost" })
	if err != nil {
		t.Fatal(err)
	}
	if host.Text != "localhost" {
		t.Fatalf("重新打开后地址 = %q", host.Text)
	}
	// 密码框不把正文交给别的进程，状态里必须是空的。是否写上由上面的配置文件确认。
	pass, err := app.WaitControl("proxyPass", func(ctrl ControlState) bool { return ctrl.Text == "" })
	if err != nil {
		t.Fatal(err)
	}
	if pass.Text != "" {
		t.Fatal("状态里出现了代理密码")
	}
}

func TestE2E_ShowKey(t *testing.T) {
	app := newDriveApp(t)
	openDriveSettings(t, app)
	mustDrive(t, app.Set("key", "abc"))
	hidden, err := app.WaitControl("key", func(ctrl ControlState) bool {
		return ctrl.PasswordChar != nil && *ctrl.PasswordChar != 0 && ctrl.Text == ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if hidden.Text != "" {
		t.Fatal("状态里出现了密钥")
	}
	mustDrive(t, app.Click("showKey"))
	if _, err := app.WaitControl("key", func(ctrl ControlState) bool {
		return ctrl.PasswordChar != nil && *ctrl.PasswordChar == 0 && ctrl.Text == ""
	}); err != nil {
		t.Fatal(err)
	}
	mustDrive(t, app.Click("apply"))
	if _, err := app.WaitSettings(false); err != nil {
		t.Fatal(err)
	}
	cfg := loadDriveConfig(t, app)
	if cfg.APIKey != "abc" {
		t.Fatalf("密钥没有写回")
	}
}

func TestE2E_Theme(t *testing.T) {
	app := newDriveApp(t)
	openDriveSettings(t, app)
	mustDrive(t, app.Select("theme", "dark"))
	if _, err := app.WaitControl("theme", func(ctrl ControlState) bool { return ctrl.Value == "dark" }); err != nil {
		t.Fatal(err)
	}
	mustDrive(t, app.Click("apply"))
	if _, err := app.WaitSettings(false); err != nil {
		t.Fatal(err)
	}
	cfg := loadDriveConfig(t, app)
	if cfg.Theme != config.ThemeDark {
		t.Fatalf("主题 = %q", cfg.Theme)
	}
}

func TestE2E_Scroll(t *testing.T) {
	app := newDriveApp(t)
	openDriveSettings(t, app)
	mustDrive(t, app.Tab("network"))
	if err := waitDrivePage(t, app, "network"); err != nil {
		t.Fatal(err)
	}
	mustDrive(t, app.Resize(1))
	var before int
	if err := poll(driveTimeout, func() bool {
		state, err := app.State()
		if err != nil || !state.CanScroll {
			return false
		}
		before = state.ScrollPos
		return true
	}); err != nil {
		state, _ := app.State()
		t.Fatalf("窗口调矮后不能滚动: %+v", state)
	}
	mustDrive(t, app.Wheel(4))
	if err := poll(driveTimeout, func() bool {
		state, err := app.State()
		return err == nil && state.ScrollPos > before
	}); err != nil {
		state, _ := app.State()
		t.Fatalf("滚轮没有改变滚动位置 %d: %+v", before, state)
	}
}

func TestE2E_Processes(t *testing.T) {
	app := newDriveApp(t)
	openDriveSettings(t, app)
	mustDrive(t, app.Tab("processes"))
	if err := waitDrivePage(t, app, "processes"); err != nil {
		t.Fatal(err)
	}
	state, err := app.State()
	if err != nil {
		t.Fatal(err)
	}
	enabled := state.Controls["procEnabled"].Items
	if !containsFold(enabled, "ms-teams.exe") || !containsFold(enabled, "Teams.exe") {
		t.Fatalf("默认启用 = %#v", enabled)
	}
	exe, err := processExe(app.PID)
	if err != nil {
		t.Fatal(err)
	}
	own := filepath.Base(exe)
	if containsFold(state.Controls["procApps"].Items, own) {
		t.Fatalf("左边出现了测试实例 %s: %#v", own, state.Controls["procApps"].Items)
	}
	if state.Controls["procAdd"].Enabled || state.Controls["procRemove"].Enabled {
		t.Fatalf("没选中时箭头不该可用: %+v %+v", state.Controls["procAdd"], state.Controls["procRemove"])
	}

	for _, name := range []string{"ms-teams.exe", "Teams.exe"} {
		mustDrive(t, app.Pick("procEnabled", name))
		if _, err := app.WaitControl("procRemove", func(ctrl ControlState) bool { return ctrl.Enabled }); err != nil {
			t.Fatal(name, err)
		}
		mustDrive(t, app.Click("procRemove"))
		if err := waitDriveList(t, app, "procEnabled", name, false); err != nil {
			t.Fatal(err)
		}
	}
	mustDrive(t, app.Click("apply"))
	if _, err := app.WaitSettings(false); err != nil {
		dialog, _ := app.Dialog()
		t.Fatalf("设置窗口没有关闭，弹窗 %q", dialog.Text)
	}
	cfg := loadDriveConfig(t, app)
	if len(cfg.Processes) != 0 {
		t.Fatalf("清空后 Processes = %#v", cfg.Processes)
	}
	if !strings.Contains(driveConfigText(t, app), "processes = []") {
		t.Fatalf("配置没有写成空列表:\n%s", driveConfigText(t, app))
	}

	openDriveSettings(t, app)
	mustDrive(t, app.Tab("processes"))
	if err := waitDrivePage(t, app, "processes"); err != nil {
		t.Fatal(err)
	}
	if err := waitDriveListLen(t, app, "procEnabled", 0); err != nil {
		t.Fatal(err)
	}
	state, err = app.State()
	if err != nil {
		t.Fatal(err)
	}
	left := state.Controls["procApps"].Items
	if len(left) == 0 {
		t.Fatal("左边没有正在运行的程序")
	}
	first := left[0]
	mustDrive(t, app.Pick("procApps", first))
	if _, err := app.WaitControl("procAdd", func(ctrl ControlState) bool { return ctrl.Enabled }); err != nil {
		t.Fatal(err)
	}
	mustDrive(t, app.Click("procAdd"))
	if err := waitDriveList(t, app, "procEnabled", first, true); err != nil {
		t.Fatal(err)
	}
	mustDrive(t, app.Click("apply"))
	if _, err := app.WaitSettings(false); err != nil {
		t.Fatal(err)
	}
	cfg = loadDriveConfig(t, app)
	if len(cfg.Processes) != 1 || !strings.EqualFold(cfg.Processes[0], first) {
		t.Fatalf("写回 = %#v，期望 %s", cfg.Processes, first)
	}
}

func TestE2E_StartNotice(t *testing.T) {
	app := newDriveApp(t)
	if err := app.WaitLog("通知：已启动"); err != nil {
		t.Fatal(err)
	}
}

func TestE2E_CheckUpdateQuiet(t *testing.T) {
	app := newDriveApp(t)
	mustDrive(t, app.Menu("check-update"))
	// 上一次还没结束时再点，也不该弹出「正在检查更新」。
	mustDrive(t, app.Menu("check-update"))
	var body string
	err := poll(20*time.Second, func() bool {
		text, readErr := app.Log()
		if readErr != nil {
			return false
		}
		body = text
		return updateResultLogged(text)
	})
	if err != nil {
		t.Fatalf("没有等到检查结果:\n%s", body)
	}
	if strings.Contains(body, "正在检查更新") {
		t.Fatalf("不应出现正在检查更新:\n%s", body)
	}
}

func updateResultLogged(text string) bool {
	for _, part := range []string{
		"通知：已是最新版本",
		"通知：发现新版本",
		"通知：检查更新失败",
		"通知：还没有已发布的版本",
		"通知：当前是本机构建",
	} {
		if strings.Contains(text, part) {
			return true
		}
	}
	return false
}

func TestE2E_ProcessWheel(t *testing.T) {
	app := newDriveApp(t)
	writeDriveProcesses(t, app, numberedProcesses(40))
	openDriveSettings(t, app)
	mustDrive(t, app.Tab("processes"))
	if err := waitDrivePage(t, app, "processes"); err != nil {
		t.Fatal(err)
	}
	if err := waitDriveListLen(t, app, "procEnabled", 40); err != nil {
		t.Fatal(err)
	}
	before, err := app.State()
	if err != nil {
		t.Fatal(err)
	}
	mustDrive(t, app.WheelOn("procEnabled", 4))
	if err := poll(driveTimeout, func() bool {
		state, readErr := app.State()
		if readErr != nil {
			return false
		}
		return state.Controls["procEnabled"].Top > before.Controls["procEnabled"].Top &&
			state.ScrollPos == before.ScrollPos
	}); err != nil {
		state, _ := app.State()
		t.Fatalf("列表没有滚动: before top %d scroll %d, now %+v", before.Controls["procEnabled"].Top, before.ScrollPos, state)
	}

	mustDrive(t, app.Close())
	if _, err := app.WaitSettings(false); err != nil {
		t.Fatal(err)
	}
	writeDriveProcesses(t, app, []string{"ms-teams.exe", "Teams.exe"})
	openDriveSettings(t, app)
	mustDrive(t, app.Tab("processes"))
	if err := waitDrivePage(t, app, "processes"); err != nil {
		t.Fatal(err)
	}
	if err := waitDriveListLen(t, app, "procEnabled", 2); err != nil {
		t.Fatal(err)
	}
	mustDrive(t, app.Resize(1))
	var pageBefore int
	if err := poll(driveTimeout, func() bool {
		state, readErr := app.State()
		if readErr != nil || !state.CanScroll {
			return false
		}
		pageBefore = state.ScrollPos
		return true
	}); err != nil {
		state, _ := app.State()
		t.Fatalf("窗口调矮后不能滚动: %+v", state)
	}
	mustDrive(t, app.WheelOn("procEnabled", 4))
	if err := poll(driveTimeout, func() bool {
		state, readErr := app.State()
		return readErr == nil && state.ScrollPos > pageBefore
	}); err != nil {
		state, _ := app.State()
		t.Fatalf("放得下的列表没有把滚轮交给整页，位置 %d: %+v", pageBefore, state)
	}
}

func numberedProcesses(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("app-%02d.exe", i+1)
	}
	return out
}

func writeDriveProcesses(t *testing.T, app *App, names []string) {
	t.Helper()
	cfg := loadDriveConfig(t, app)
	cfg.Processes = names
	if err := config.Save(app.ConfigPath, cfg); err != nil {
		t.Fatal(err)
	}
}

func TestE2E_ModelTest(t *testing.T) {
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		t.Setenv(key, "")
	}
	t.Setenv("NO_PROXY", "*")
	t.Setenv("no_proxy", "*")

	var sawSample atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if strings.Contains(string(body), translate.CheckSample) {
			sawSample.Store(true)
		}
		if r.Header.Get("Authorization") != "Bearer good-key" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":{"message":"do-not-show-this"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"Hello"}}]}`)
	}))
	defer srv.Close()

	app := newDriveApp(t)
	before := driveConfigText(t, app)
	openDriveSettings(t, app)
	shotDrive(t, app, "model-test-light.png")
	mustDrive(t, app.Select("theme", "dark"))
	if _, err := app.WaitControl("theme", func(ctrl ControlState) bool { return ctrl.Value == "dark" }); err != nil {
		t.Fatal(err)
	}
	shotDrive(t, app, "model-test-dark.png")
	mustDrive(t, app.Select("theme", "light"))

	mustDrive(t, app.Set("url", srv.URL+"/v1"))
	mustDrive(t, app.Set("model", "demo"))
	mustDrive(t, app.Set("key", "good-key"))
	mustDrive(t, app.Click("test"))
	status, err := app.WaitControl("testStatus", func(ctrl ControlState) bool {
		return strings.Contains(ctrl.Text, "可用") && strings.Contains(ctrl.Text, "Hello")
	})
	if err != nil {
		t.Fatalf("测试结果 = %q", status.Text)
	}
	if _, err := app.WaitControl("test", func(ctrl ControlState) bool { return ctrl.Enabled }); err != nil {
		t.Fatal(err)
	}
	logText, err := app.Log()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logText, "模型测试成功") || strings.Contains(logText, "Hello") {
		t.Fatalf("日志不应包含译文:\n%s", logText)
	}
	if !sawSample.Load() {
		t.Fatal("模型没有收到固定样句")
	}
	shotDrive(t, app, "model-test-result.png")

	mustDrive(t, app.Set("model", "other-model"))
	if _, err := app.WaitControl("testStatus", func(ctrl ControlState) bool { return ctrl.Text == "" }); err != nil {
		t.Fatal(err)
	}
	mustDrive(t, app.Set("model", "demo"))
	mustDrive(t, app.Set("key", "bad-key"))
	mustDrive(t, app.Click("test"))
	status, err = app.WaitControl("testStatus", func(ctrl ControlState) bool {
		return strings.Contains(ctrl.Text, "HTTP 401") && strings.Contains(ctrl.Text, "API Key")
	})
	if err != nil {
		t.Fatalf("失败结果 = %q", status.Text)
	}
	if strings.Contains(status.Text, "do-not-show-this") {
		t.Fatalf("结果带了响应正文: %s", status.Text)
	}
	mustDrive(t, app.Click("cancel"))
	if _, err := app.WaitSettings(false); err != nil {
		t.Fatal(err)
	}
	if got := driveConfigText(t, app); got != before {
		t.Fatal("测试或取消后配置文件变了")
	}
}

func shotDrive(t *testing.T, app *App, name string) {
	t.Helper()
	root, err := ModuleRoot()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".tools", "tmp", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := app.Shot(path); err != nil {
		t.Fatal(err)
	}
	t.Log(path)
}

func TestE2E_Exit(t *testing.T) {
	app := newDriveApp(t)
	if err := app.Stop(); err != nil {
		t.Fatal(err)
	}
	if processAlive(app.PID) {
		t.Fatalf("进程 %d 还在运行", app.PID)
	}
}

func newDriveApp(t *testing.T) *App {
	t.Helper()
	app, err := Start()
	if err != nil {
		t.Fatalf("启动: %v", err)
	}
	t.Cleanup(func() {
		if err := app.Stop(); err != nil {
			t.Errorf("退出: %v", err)
		}
	})
	return app
}

func openDriveSettings(t *testing.T, app *App) UIState {
	t.Helper()
	if err := app.Menu("settings"); err != nil {
		t.Fatal(err)
	}
	state, err := app.WaitSettings(true)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func loadDriveConfig(t *testing.T, app *App) config.Config {
	t.Helper()
	cfg, err := config.Load(app.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func driveConfigText(t *testing.T, app *App) string {
	t.Helper()
	data, err := os.ReadFile(app.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func containsFold(items []string, name string) bool {
	for _, item := range items {
		if strings.EqualFold(item, name) {
			return true
		}
	}
	return false
}

func waitDriveList(t *testing.T, app *App, control, name string, want bool) error {
	t.Helper()
	return poll(driveTimeout, func() bool {
		state, err := app.State()
		if err != nil {
			return false
		}
		return containsFold(state.Controls[control].Items, name) == want
	})
}

func waitDriveListLen(t *testing.T, app *App, control string, n int) error {
	t.Helper()
	return poll(driveTimeout, func() bool {
		state, err := app.State()
		if err != nil {
			return false
		}
		return len(state.Controls[control].Items) == n
	})
}

func waitDrivePage(t *testing.T, app *App, page string) error {
	t.Helper()
	return poll(driveTimeout, func() bool {
		state, err := app.State()
		return err == nil && state.Page == page
	})
}

func mustDrive(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
