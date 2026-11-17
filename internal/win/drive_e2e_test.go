//go:build windows && uidrive

package win

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"transhelper/internal/config"
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
