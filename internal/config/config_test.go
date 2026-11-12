package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestParseExampleFile(t *testing.T) {
	path := filepath.Join("..", "..", "transhelper.example.toml")
	data, err := os.ReadFile(path)
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
		t.Fatal("示例配置不应该带密钥")
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
