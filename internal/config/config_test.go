package config

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"transhelper/internal/hotkey"
)

func TestParseDefaults(t *testing.T) {
	cfg, err := Parse([]byte(`
[model]
base_url = "http://127.0.0.1:9/v1/"
model = " demo "
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "http://127.0.0.1:9/v1" {
		t.Fatalf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.Model != "demo" {
		t.Fatalf("Model = %q", cfg.Model)
	}
	if cfg.APIKey != "" {
		t.Fatalf("APIKey = %q，默认应为空", cfg.APIKey)
	}
	if cfg.Timeout != 20*time.Second {
		t.Fatalf("Timeout = %s", cfg.Timeout)
	}
	if cfg.Temperature != 0.2 {
		t.Fatalf("Temperature = %v", cfg.Temperature)
	}
	if cfg.SystemPrompt != "" {
		t.Fatalf("SystemPrompt = %q", cfg.SystemPrompt)
	}
	if cfg.TranslateAndSend.String() != "Alt+Enter" || cfg.TranslateOnly.String() != "Alt+T" {
		t.Fatalf("热键 = %s / %s", cfg.TranslateAndSend, cfg.TranslateOnly)
	}
	if len(cfg.Processes) != 2 || cfg.Processes[0] != "ms-teams.exe" || cfg.Processes[1] != "Teams.exe" {
		t.Fatalf("Processes = %#v", cfg.Processes)
	}
	if !cfg.AllowsProcess(`C:\Program Files\Teams\ms-teams.exe`) || !cfg.AllowsProcess("TEAMS.EXE") {
		t.Fatal("默认白名单应该匹配两个 Teams 进程名，且不区分大小写")
	}
	if cfg.AllowsProcess("notepad.exe") {
		t.Fatal("记事本不应该在默认白名单里")
	}
	if cfg.AutoCheckUpdate {
		t.Fatal("未写 update.auto_check 时应该关闭自动检查")
	}
	if cfg.Proxy.Enabled || cfg.Proxy.Type != "http" || cfg.Proxy.Host != "" || cfg.Proxy.Port != 0 || cfg.Proxy.Auth || cfg.Proxy.ForModel {
		t.Fatalf("未写 proxy 时 = %+v", cfg.Proxy)
	}
}

func TestParseExplicitZeroTemperature(t *testing.T) {
	cfg, err := Parse([]byte(`
[model]
base_url = "http://127.0.0.1:9/v1"
model = "demo"
temperature = 0
timeout_seconds = 1
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Temperature != 0 {
		t.Fatalf("Temperature = %v，显式写 0 时不该被默认值覆盖", cfg.Temperature)
	}
	if cfg.Timeout != time.Second {
		t.Fatalf("Timeout = %s", cfg.Timeout)
	}
}

func TestParseBOM(t *testing.T) {
	body := []byte{0xEF, 0xBB, 0xBF}
	body = append(body, []byte(`
[model]
base_url = "https://example.com/v1"
model = "demo"
`)...)
	if _, err := Parse(body); err != nil {
		t.Fatal(err)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		toml string
		want string
	}{
		{
			name: "缺少地址",
			toml: `
[model]
model = "demo"
`,
			want: "base_url",
		},
		{
			name: "缺少模型名",
			toml: `
[model]
base_url = "http://127.0.0.1:9/v1"
`,
			want: "model.model",
		},
		{
			name: "非法地址",
			toml: `
[model]
base_url = "ftp://127.0.0.1/v1"
model = "demo"
`,
			want: "base_url",
		},
		{
			name: "超时太小",
			toml: `
[model]
base_url = "http://127.0.0.1:9/v1"
model = "demo"
timeout_seconds = 0
`,
			want: "timeout_seconds",
		},
		{
			name: "超时太大",
			toml: `
[model]
base_url = "http://127.0.0.1:9/v1"
model = "demo"
timeout_seconds = 121
`,
			want: "timeout_seconds",
		},
		{
			name: "温度超出范围",
			toml: `
[model]
base_url = "http://127.0.0.1:9/v1"
model = "demo"
temperature = 2.5
`,
			want: "temperature",
		},
		{
			name: "非法热键",
			toml: `
[model]
base_url = "http://127.0.0.1:9/v1"
model = "demo"

[hotkeys]
translate_only = "Shift+A"
`,
			want: "translate_only",
		},
		{
			name: "热键重复",
			toml: `
[model]
base_url = "http://127.0.0.1:9/v1"
model = "demo"

[hotkeys]
translate_and_send = "Alt+Enter"
translate_only = "Alt+Return"
`,
			want: "不能相同",
		},
		{
			name: "白名单为空",
			toml: `
[model]
base_url = "http://127.0.0.1:9/v1"
model = "demo"

[target]
processes = []
`,
			want: "processes",
		},
		{
			name: "进程名重复",
			toml: `
[model]
base_url = "http://127.0.0.1:9/v1"
model = "demo"

[target]
processes = ["notepad.exe", "Notepad.exe"]
`,
			want: "重复",
		},
		{
			name: "代理类型不对",
			toml: `
[model]
base_url = "http://127.0.0.1:9/v1"
model = "demo"

[proxy]
type = "https"
`,
			want: "proxy.type",
		},
		{
			name: "代理端口太大",
			toml: `
[model]
base_url = "http://127.0.0.1:9/v1"
model = "demo"

[proxy]
port = 65536
`,
			want: "proxy.port",
		},
		{
			name: "启用时代理地址为空",
			toml: `
[model]
base_url = "http://127.0.0.1:9/v1"
model = "demo"

[proxy]
enabled = true
port = 8080
`,
			want: "proxy.host",
		},
		{
			name: "启用时代理端口为 0",
			toml: `
[model]
base_url = "http://127.0.0.1:9/v1"
model = "demo"

[proxy]
enabled = true
host = "localhost"
port = 0
`,
			want: "proxy.port",
		},
		{
			name: "启用身份验证但没有用户名",
			toml: `
[model]
base_url = "http://127.0.0.1:9/v1"
model = "demo"

[proxy]
enabled = true
host = "localhost"
port = 1080
auth = true
`,
			want: "proxy.username",
		},
		{
			name: "代理地址带协议",
			toml: `
[model]
base_url = "http://127.0.0.1:9/v1"
model = "demo"

[proxy]
host = "http://localhost"
`,
			want: "proxy.host",
		},
		{
			name: "未知字段",
			toml: `
[model]
base_url = "http://127.0.0.1:9/v1"
model = "demo"
timeout = 5
`,
			want: "timeout",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.toml))
			if err == nil {
				t.Fatal("期望解析失败")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("错误 %q 里没有 %q", err.Error(), tt.want)
			}
		})
	}
}

func TestDefaultTemplate(t *testing.T) {
	data, err := Render(Default())
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "http://localhost:8000/v1" {
		t.Fatalf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.Model != "Qwen/Qwen3.6-35B-A3B" {
		t.Fatalf("Model = %q", cfg.Model)
	}
	if cfg.APIKey != "" {
		t.Fatal("默认配置不应该带密钥")
	}
	if cfg.Timeout != 20*time.Second || cfg.Temperature != 0.2 {
		t.Fatalf("Timeout=%s Temperature=%v", cfg.Timeout, cfg.Temperature)
	}
	if cfg.TranslateAndSend.String() != "Alt+Enter" || cfg.TranslateOnly.String() != "Alt+T" {
		t.Fatalf("热键 = %s / %s", cfg.TranslateAndSend, cfg.TranslateOnly)
	}
	if !cfg.AllowsProcess("ms-teams.exe") || !cfg.AllowsProcess("Teams.exe") {
		t.Fatalf("Processes = %#v", cfg.Processes)
	}
	if cfg.AutoCheckUpdate {
		t.Fatal("默认不应该自动检查更新")
	}
	if cfg.Proxy.Enabled || cfg.Proxy.Type != "http" || cfg.Proxy.Auth || cfg.Proxy.ForModel {
		t.Fatalf("默认代理 = %+v", cfg.Proxy)
	}
	if strings.Contains(string(data), "192.168.") || strings.Contains(cfg.BaseURL, "192.168.") {
		t.Fatal("默认配置不应该写内网地址")
	}
}

func TestRenderRoundTrip(t *testing.T) {
	cfg := Default()
	cfg.APIKey = "key \"quoted\" \\ slash"
	cfg.SystemPrompt = "第一行\n第二行 \"引号\" \\ 反斜杠 \t制表"
	cfg.Model = "org/name"
	cfg.BaseURL = "https://localhost:8000/v1"
	cfg.Timeout = 15 * time.Second
	cfg.Temperature = 0
	cfg.AutoCheckUpdate = true
	cfg.Processes = []string{"ms-teams.exe", "Teams.exe"}
	cfg.Proxy = Proxy{
		Enabled:  true,
		Type:     "socks5",
		Host:     "localhost",
		Port:     1080,
		Auth:     true,
		Username: "user",
		Password: "p@ss \"word\" \\ 空格 ",
		ForModel: true,
	}
	only, err := hotkey.Parse("Ctrl+Alt+T")
	if err != nil {
		t.Fatal(err)
	}
	cfg.TranslateOnly = only

	data, err := Render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("解析渲染结果: %v\n%s", err, data)
	}
	if got.APIKey != cfg.APIKey || got.SystemPrompt != cfg.SystemPrompt {
		t.Fatalf("字符串没有原样回来: key=%q prompt=%q", got.APIKey, got.SystemPrompt)
	}
	if got.Model != cfg.Model || got.BaseURL != cfg.BaseURL {
		t.Fatalf("地址或模型变化: %q %q", got.BaseURL, got.Model)
	}
	if got.Timeout != cfg.Timeout || got.Temperature != cfg.Temperature {
		t.Fatalf("Timeout=%s Temperature=%v", got.Timeout, got.Temperature)
	}
	if !got.AutoCheckUpdate {
		t.Fatal("auto_check 应该为 true")
	}
	if got.TranslateOnly.String() != "Ctrl+Alt+T" || got.TranslateAndSend.String() != "Alt+Enter" {
		t.Fatalf("热键 = %s / %s", got.TranslateAndSend, got.TranslateOnly)
	}
	if len(got.Processes) != 2 || got.Processes[0] != "ms-teams.exe" || got.Processes[1] != "Teams.exe" {
		t.Fatalf("Processes = %#v", got.Processes)
	}
	if got.Proxy != cfg.Proxy {
		t.Fatalf("代理没有原样回来: %+v", got.Proxy)
	}
}

func TestParseProxy(t *testing.T) {
	cfg, err := Parse([]byte(`
[model]
base_url = "http://127.0.0.1:9/v1"
model = "demo"

[proxy]
enabled = true
type = "SOCKS5"
host = "localhost"
port = 1080
auth = true
username = " user "
password = " keep spaces "
use_for_model = true
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Proxy.Type != "socks5" || cfg.Proxy.Host != "localhost" || cfg.Proxy.Port != 1080 {
		t.Fatalf("地址 = %+v", cfg.Proxy)
	}
	if cfg.Proxy.Username != "user" || cfg.Proxy.Password != " keep spaces " || !cfg.Proxy.ForModel {
		t.Fatalf("身份 = %+v", cfg.Proxy)
	}

	off, err := Parse([]byte(`
[model]
base_url = "http://127.0.0.1:9/v1"
model = "demo"

[proxy]
enabled = false
auth = true
password = "x"
`))
	if err != nil {
		t.Fatal(err)
	}
	if off.Proxy.Enabled || !off.Proxy.Auth || off.Proxy.Username != "" || off.Proxy.Password != "x" {
		t.Fatalf("关闭代理时仍应保留已填内容: %+v", off.Proxy)
	}
}

func TestProxyURLAndClient(t *testing.T) {
	off := Proxy{Type: "http", Host: "127.0.0.1", Port: 8888, Auth: true, Username: "u", Password: "p"}
	if off.URL() != nil || off.Client() != nil || off.ModelClient() != nil {
		t.Fatal("未启用时不应该产生代理")
	}

	on := Proxy{Enabled: true, Type: "http", Host: "127.0.0.1", Port: 8888}
	if on.URL().String() != "http://127.0.0.1:8888" {
		t.Fatalf("URL = %s", on.URL())
	}
	if on.ModelClient() != nil {
		t.Fatal("未勾选时模型不走代理")
	}
	req, err := http.NewRequest(http.MethodGet, "https://example.com/v1", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := on.Client().Transport.(*http.Transport).Proxy(req)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "http://127.0.0.1:8888" {
		t.Fatalf("Proxy = %s", got)
	}

	socks := Proxy{
		Enabled: true, Type: "socks5", Host: "2001:db8::1", Port: 1080,
		Auth: true, Username: "user", Password: "p@ss word", ForModel: true,
	}
	u := socks.URL()
	if u.Scheme != "socks5" || u.Host != "[2001:db8::1]:1080" {
		t.Fatalf("URL = %s", u)
	}
	if u.User.Username() != "user" {
		t.Fatalf("username = %s", u.User.Username())
	}
	pass, ok := u.User.Password()
	if !ok || pass != "p@ss word" {
		t.Fatalf("password = %q", pass)
	}
	model := socks.ModelClient()
	if model == nil {
		t.Fatal("模型客户端应该存在")
	}
	req2, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:9/v1/chat/completions", nil)
	if err != nil {
		t.Fatal(err)
	}
	got2, err := model.Transport.(*http.Transport).Proxy(req2)
	if err != nil {
		t.Fatal(err)
	}
	if got2.Scheme != "socks5" || got2.Host != "[2001:db8::1]:1080" {
		t.Fatalf("model proxy = %s", got2)
	}
}

func TestCreateDefaultDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := CreateDefault(path)
	if err == nil {
		t.Fatal("已有文件时不应该覆盖")
	}
	if !os.IsExist(err) {
		t.Fatalf("错误 = %v，期望已存在", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep" {
		t.Fatalf("文件被改成了 %q", got)
	}
}

func TestSaveKeepsOriginalWhenInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Default()
	cfg.BaseURL = ""
	if err := Save(path, cfg); err == nil {
		t.Fatal("空地址应该保存失败")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep" {
		t.Fatalf("失败时文件被改成了 %q", got)
	}
}

func TestSaveOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if err := CreateDefault(path); err != nil {
		t.Fatal(err)
	}
	cfg := Default()
	cfg.Model = "replaced"
	cfg.AutoCheckUpdate = true
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "replaced" || !got.AutoCheckUpdate {
		t.Fatalf("Model=%q AutoCheck=%v", got.Model, got.AutoCheckUpdate)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".transhelper-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("临时文件还在: %v", matches)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err == nil {
		t.Fatal("缺少文件时应该失败")
	}
}

func TestDefaultPath(t *testing.T) {
	got := DefaultPath(`D:\apps\transhelper.exe`)
	want := filepath.Join(`D:\apps`, FileName)
	if got != want {
		t.Fatalf("DefaultPath = %q, 期望 %q", got, want)
	}
}
