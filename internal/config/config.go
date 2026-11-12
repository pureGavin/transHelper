// Package config 读取 TOML 配置并填上默认值。
package config

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"transhelper/internal/hotkey"
)

const (
	// FileName 是默认配置文件名，放在 exe 同目录。
	FileName = "transhelper.toml"

	defaultTimeoutSeconds = 20
	minTimeoutSeconds     = 1
	maxTimeoutSeconds     = 120
	defaultTemperature    = 0.2
)

// DefaultProcesses 是未写进程白名单时使用的名单。
// ms-teams.exe 是新版 Teams，Teams.exe 是经典版。
var DefaultProcesses = []string{"ms-teams.exe", "Teams.exe"}

// Config 是校验并填好默认值之后的配置。
type Config struct {
	// BaseURL 是 OpenAI 兼容接口的地址，已去掉末尾的斜杠。
	BaseURL string
	// Model 是模型名。
	Model string
	// APIKey 是可选的接口密钥，为空时不发送 Authorization 头。
	APIKey string
	// Timeout 是单次翻译的超时时间。
	Timeout time.Duration
	// Temperature 传给模型。0 是合法值，表示未填写时才会用默认的 0.2。
	Temperature float64
	// SystemPrompt 为空时使用内置提示词。
	SystemPrompt string
	// TranslateAndSend 是翻译并发送的热键。
	TranslateAndSend hotkey.Combo
	// TranslateOnly 是只翻译不发送的热键。
	TranslateOnly hotkey.Combo
	// Processes 是前台进程白名单，只保留文件名，比较时不区分大小写。
	Processes []string
}

// DefaultPath 返回 exe 同目录下的配置文件路径。
func DefaultPath(exePath string) string {
	return filepath.Join(filepath.Dir(exePath), FileName)
}

// AllowsProcess 报告进程名是否在白名单里。name 可以是文件名或完整路径。
func (c Config) AllowsProcess(name string) bool {
	base := filepath.Base(strings.TrimSpace(name))
	for _, process := range c.Processes {
		if strings.EqualFold(process, base) {
			return true
		}
	}
	return false
}

// Load 从 path 读取配置文件。
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("读取配置文件 %s: %w", path, err)
	}
	cfg, err := Parse(data)
	if err != nil {
		return Config{}, fmt.Errorf("解析配置文件 %s: %w", path, err)
	}
	return cfg, nil
}

type fileConfig struct {
	Model   modelSection  `toml:"model"`
	Hotkeys hotkeySection `toml:"hotkeys"`
	Target  targetSection `toml:"target"`
}

type modelSection struct {
	BaseURL        string  `toml:"base_url"`
	Model          string  `toml:"model"`
	APIKey         string  `toml:"api_key"`
	TimeoutSeconds int     `toml:"timeout_seconds"`
	Temperature    float64 `toml:"temperature"`
	SystemPrompt   string  `toml:"system_prompt"`
}

type hotkeySection struct {
	TranslateAndSend string `toml:"translate_and_send"`
	TranslateOnly    string `toml:"translate_only"`
}

type targetSection struct {
	Processes []string `toml:"processes"`
}

// Parse 解析 TOML 内容。未填写的可选项使用默认值，未知字段会报错。
func Parse(data []byte) (Config, error) {
	// Windows 记事本和 PowerShell 5.1 常写出带 BOM 的 UTF-8，去掉后才能解析。
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	var raw fileConfig
	md, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&raw)
	if err != nil {
		return Config{}, err
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		names := make([]string, len(undecoded))
		for i, key := range undecoded {
			names[i] = key.String()
		}
		return Config{}, fmt.Errorf("配置里有无法识别的字段: %s", strings.Join(names, ", "))
	}

	cfg := Config{
		BaseURL:      strings.TrimRight(strings.TrimSpace(raw.Model.BaseURL), "/"),
		Model:        strings.TrimSpace(raw.Model.Model),
		APIKey:       strings.TrimSpace(raw.Model.APIKey),
		SystemPrompt: strings.TrimSpace(raw.Model.SystemPrompt),
	}
	if cfg.BaseURL == "" {
		return Config{}, fmt.Errorf("model.base_url 不能为空")
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return Config{}, fmt.Errorf("model.base_url 必须是 http 或 https 地址")
	}
	if cfg.Model == "" {
		return Config{}, fmt.Errorf("model.model 不能为空")
	}

	timeoutSeconds := raw.Model.TimeoutSeconds
	if !md.IsDefined("model", "timeout_seconds") {
		timeoutSeconds = defaultTimeoutSeconds
	}
	if timeoutSeconds < minTimeoutSeconds || timeoutSeconds > maxTimeoutSeconds {
		return Config{}, fmt.Errorf("model.timeout_seconds 必须在 %d 到 %d 之间", minTimeoutSeconds, maxTimeoutSeconds)
	}
	cfg.Timeout = time.Duration(timeoutSeconds) * time.Second

	cfg.Temperature = raw.Model.Temperature
	if !md.IsDefined("model", "temperature") {
		cfg.Temperature = defaultTemperature
	}
	if cfg.Temperature < 0 || cfg.Temperature > 2 {
		return Config{}, fmt.Errorf("model.temperature 必须在 0 到 2 之间")
	}

	sendSpec := strings.TrimSpace(raw.Hotkeys.TranslateAndSend)
	if !md.IsDefined("hotkeys", "translate_and_send") {
		sendSpec = "Alt+Enter"
	}
	onlySpec := strings.TrimSpace(raw.Hotkeys.TranslateOnly)
	if !md.IsDefined("hotkeys", "translate_only") {
		onlySpec = "Alt+T"
	}
	cfg.TranslateAndSend, err = hotkey.Parse(sendSpec)
	if err != nil {
		return Config{}, fmt.Errorf("hotkeys.translate_and_send: %w", err)
	}
	cfg.TranslateOnly, err = hotkey.Parse(onlySpec)
	if err != nil {
		return Config{}, fmt.Errorf("hotkeys.translate_only: %w", err)
	}
	if cfg.TranslateAndSend.Same(cfg.TranslateOnly) {
		return Config{}, fmt.Errorf("两个热键不能相同")
	}

	if !md.IsDefined("target", "processes") {
		cfg.Processes = append([]string(nil), DefaultProcesses...)
	} else {
		if len(raw.Target.Processes) == 0 {
			return Config{}, fmt.Errorf("target.processes 不能为空")
		}
		seen := make(map[string]struct{}, len(raw.Target.Processes))
		for _, process := range raw.Target.Processes {
			name := filepath.Base(strings.TrimSpace(process))
			if name == "" || name == "." || name == string(filepath.Separator) {
				return Config{}, fmt.Errorf("target.processes 里有空的进程名")
			}
			key := strings.ToLower(name)
			if _, ok := seen[key]; ok {
				return Config{}, fmt.Errorf("target.processes 里有重复的进程名 %s", name)
			}
			seen[key] = struct{}{}
			cfg.Processes = append(cfg.Processes, name)
		}
	}
	return cfg, nil
}
