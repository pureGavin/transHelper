// Package config 读取 TOML 配置并填上默认值。
package config

import (
	"bytes"
	_ "embed"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/BurntSushi/toml"

	"transhelper/internal/hotkey"
)

//go:embed template.toml
var configTemplate string

var configTmpl = template.Must(template.New("config").Funcs(template.FuncMap{
	"toml":    tomlString,
	"seconds": func(d time.Duration) int { return int(d / time.Second) },
	"temp":    func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) },
}).Parse(configTemplate))

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
	// AutoCheckUpdate 为 true 时定期检查 GitHub Release。默认 false。
	AutoCheckUpdate bool
	// Proxy 是检查更新和可选的翻译请求使用的代理。未启用时沿用环境变量。
	Proxy Proxy
}

// Proxy 是 HTTP 或 SOCKS5 代理。密码保留首尾空格。
type Proxy struct {
	// Enabled 为 false 时不使用下面的地址，请求沿用 HTTPS_PROXY 等环境变量。
	Enabled bool
	// Type 是 http 或 socks5。
	Type string
	// Host 是代理主机名或 IP，不含协议和端口。
	Host string
	// Port 是代理端口。未启用时可以是 0。
	Port int
	// Auth 为 true 时向代理提供用户名和密码。
	Auth bool
	// Username 是代理用户名。
	Username string
	// Password 是代理密码，不做 TrimSpace。
	Password string
	// ForModel 为 true 时翻译请求也走这个代理。
	ForModel bool
}

// URL 返回代理地址。未启用时返回 nil。开启身份验证时带上用户名和密码。
func (p Proxy) URL() *url.URL {
	if !p.Enabled {
		return nil
	}
	u := &url.URL{
		Scheme: p.Type,
		Host:   net.JoinHostPort(p.Host, strconv.Itoa(p.Port)),
	}
	if p.Auth {
		u.User = url.UserPassword(p.Username, p.Password)
	}
	return u
}

// Client 返回走该代理的 HTTP 客户端。未启用时返回 nil，调用方继续用默认客户端。
func (p Proxy) Client() *http.Client {
	u := p.URL()
	if u == nil {
		return nil
	}
	var transport *http.Transport
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		transport = base.Clone()
	} else {
		transport = &http.Transport{}
	}
	transport.Proxy = http.ProxyURL(u)
	return &http.Client{Transport: transport}
}

// ModelClient 仅在启用代理且翻译也走代理时返回客户端，否则返回 nil。
func (p Proxy) ModelClient() *http.Client {
	if !p.ForModel {
		return nil
	}
	return p.Client()
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
	Update  updateSection `toml:"update"`
	Proxy   proxySection  `toml:"proxy"`
}

type proxySection struct {
	Enabled     bool   `toml:"enabled"`
	Type        string `toml:"type"`
	Host        string `toml:"host"`
	Port        int    `toml:"port"`
	Auth        bool   `toml:"auth"`
	Username    string `toml:"username"`
	Password    string `toml:"password"`
	UseForModel bool   `toml:"use_for_model"`
}

type updateSection struct {
	AutoCheck bool `toml:"auto_check"`
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
	cfg.AutoCheckUpdate = raw.Update.AutoCheck
	proxy, err := parseProxy(raw.Proxy, md)
	if err != nil {
		return Config{}, err
	}
	cfg.Proxy = proxy
	return cfg, nil
}

func parseProxy(raw proxySection, md toml.MetaData) (Proxy, error) {
	kind := strings.ToLower(strings.TrimSpace(raw.Type))
	if kind == "" && !md.IsDefined("proxy", "type") {
		kind = "http"
	}
	if kind != "http" && kind != "socks5" {
		return Proxy{}, fmt.Errorf("proxy.type 只能是 http 或 socks5")
	}
	if raw.Port < 0 || raw.Port > 65535 {
		return Proxy{}, fmt.Errorf("proxy.port 必须在 0 到 65535 之间")
	}
	host := strings.TrimSpace(raw.Host)
	if host != "" && !validProxyHost(host) {
		return Proxy{}, fmt.Errorf("proxy.host 不能包含协议、路径或空格")
	}
	username := strings.TrimSpace(raw.Username)
	if raw.Enabled {
		if host == "" {
			return Proxy{}, fmt.Errorf("proxy.host 不能为空")
		}
		if raw.Port < 1 {
			return Proxy{}, fmt.Errorf("proxy.port 必须在 1 到 65535 之间")
		}
		if raw.Auth && username == "" {
			return Proxy{}, fmt.Errorf("proxy.username 不能为空")
		}
	}
	return Proxy{
		Enabled:  raw.Enabled,
		Type:     kind,
		Host:     host,
		Port:     raw.Port,
		Auth:     raw.Auth,
		Username: username,
		Password: raw.Password,
		ForModel: raw.UseForModel,
	}, nil
}

func validProxyHost(host string) bool {
	if strings.Contains(host, "://") || strings.Contains(host, "/") || strings.Contains(host, "@") {
		return false
	}
	return !strings.ContainsAny(host, "[] \t\r\n")
}

const (
	defaultBaseURL = "http://localhost:8000/v1"
	defaultModel   = "Qwen/Qwen3.6-35B-A3B"
)

// Default 返回第一次运行时写入的配置。地址是占位值，密钥为空。
func Default() Config {
	send, err := hotkey.Parse("Alt+Enter")
	if err != nil {
		panic(err)
	}
	only, err := hotkey.Parse("Alt+T")
	if err != nil {
		panic(err)
	}
	return Config{
		BaseURL:          defaultBaseURL,
		Model:            defaultModel,
		Timeout:          defaultTimeoutSeconds * time.Second,
		Temperature:      defaultTemperature,
		TranslateAndSend: send,
		TranslateOnly:    only,
		Processes:        append([]string(nil), DefaultProcesses...),
		Proxy:            Proxy{Type: "http"},
	}
}

// Render 按内置模板把 cfg 写成 TOML。字符串里的引号、反斜杠和换行会按 TOML 规则转义。
func Render(cfg Config) ([]byte, error) {
	var buf bytes.Buffer
	if err := configTmpl.Execute(&buf, cfg); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// CreateDefault 在 path 创建默认配置。文件已存在时返回错误，不会覆盖。
func CreateDefault(path string) error {
	data, err := Render(Default())
	if err != nil {
		return err
	}
	// O_EXCL 保证已有配置（可能含密钥）不会被默认内容换掉。
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		os.Remove(path)
		return writeErr
	}
	if closeErr != nil {
		os.Remove(path)
		return closeErr
	}
	return nil
}

// Save 校验 cfg 后原子写入 path。手写在文件里的注释会被模板注释替换。
func Save(path string, cfg Config) error {
	data, err := Render(cfg)
	if err != nil {
		return err
	}
	if _, err := Parse(data); err != nil {
		return fmt.Errorf("配置无效: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".transhelper-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil {
		os.Remove(tmpName)
		return writeErr
	}
	if closeErr != nil {
		os.Remove(tmpName)
		return closeErr
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// tomlString 把 s 编码成 TOML 基本字符串，包含两侧引号。
func tomlString(s string) string {
	var buf strings.Builder
	buf.Grow(len(s) + 2)
	buf.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			buf.WriteString(`\\`)
		case '"':
			buf.WriteString(`\"`)
		case '\b':
			buf.WriteString(`\b`)
		case '\t':
			buf.WriteString(`\t`)
		case '\n':
			buf.WriteString(`\n`)
		case '\f':
			buf.WriteString(`\f`)
		case '\r':
			buf.WriteString(`\r`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&buf, `\u%04X`, r)
			} else {
				buf.WriteRune(r)
			}
		}
	}
	buf.WriteByte('"')
	return buf.String()
}
