package translate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"unicode/utf8"
)

const (
	defaultMaxTokens = 4096
	maxBodyBytes     = 1 << 20
	maxSnippetRunes  = 512
)

// Kind 区分翻译失败的原因，调用方据此决定提示文案。
type Kind int

const (
	// KindTimeout 表示请求超时。
	KindTimeout Kind = iota + 1
	// KindCanceled 表示调用方取消了请求，例如程序正在退出。
	KindCanceled
	// KindHTTP 表示连不上模型，或服务返回了失败状态。
	KindHTTP
	// KindDecode 表示响应不是合法的 JSON。
	KindDecode
	// KindEmpty 表示没有可用的译文。
	KindEmpty
	// KindTruncated 表示译文被 max_tokens 截断，不能当作完整结果粘贴。
	KindTruncated
)

// Error 是翻译客户端的错误。Body 只用于排查，Error 文本不包含它，避免把原文写进日志。
type Error struct {
	Kind   Kind
	Status int
	Body   string
	Err    error
}

func (e *Error) Error() string {
	switch e.Kind {
	case KindTimeout:
		return "翻译超时"
	case KindCanceled:
		return "翻译已取消"
	case KindHTTP:
		if e.Status == 0 {
			return "连不上模型"
		}
		return fmt.Sprintf("模型返回 HTTP %d", e.Status)
	case KindDecode:
		return "模型返回无法解析"
	case KindEmpty:
		return "模型返回空结果"
	case KindTruncated:
		return "译文过长被截断"
	default:
		return "翻译失败"
	}
}

func (e *Error) Unwrap() error { return e.Err }

// UserMessage 返回给用户看的短提示。失败时原文保持不动。
func UserMessage(err error) string {
	var te *Error
	if !errors.As(err, &te) {
		return "翻译失败，原文未改动"
	}
	switch te.Kind {
	case KindTimeout:
		return "翻译超时，原文未改动"
	case KindCanceled:
		return "操作已取消，原文未改动"
	case KindHTTP:
		if te.Status == 0 {
			return "连不上模型，原文未改动"
		}
		return fmt.Sprintf("模型返回 HTTP %d，原文未改动", te.Status)
	case KindDecode:
		return "模型返回无法解析，原文未改动"
	case KindEmpty:
		return "模型返回空结果，原文未改动"
	case KindTruncated:
		return "译文过长被截断，原文未改动"
	default:
		return "翻译失败，原文未改动"
	}
}

// Client 调用 OpenAI 兼容的 /chat/completions。
type Client struct {
	// BaseURL 例如 http://127.0.0.1:20000/v1，不要带末尾斜杠。
	BaseURL string
	// APIKey 为空时不发送 Authorization 头。
	APIKey string
	// Model 是请求体里的模型名。
	Model string
	// Temperature 原样传给模型。
	Temperature float64
	// Prompt 为空时使用 DefaultPrompt。
	Prompt string
	// MaxTokens 为 0 时使用 4096，避免服务端默认值把长消息截断。
	MaxTokens int
	// HTTP 为空时使用 http.DefaultClient。超时由 ctx 控制。
	HTTP *http.Client
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	Stream      bool          `json:"stream"`
	MaxTokens   int           `json:"max_tokens"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Translate 把 text 译成英文。text 里的换行使用 LF。
func (c *Client) Translate(ctx context.Context, text string) (string, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return "", fmt.Errorf("模型地址为空")
	}
	if strings.TrimSpace(c.Model) == "" {
		return "", fmt.Errorf("模型名为空")
	}
	if err := ctx.Err(); err != nil {
		return "", classifyTransport(ctx, err)
	}

	payload := chatRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{Role: "system", Content: c.prompt()},
			{Role: "user", Content: text},
		},
		Temperature: c.Temperature,
		Stream:      false,
		MaxTokens:   c.maxTokens(),
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(c.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", classifyTransport(ctx, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return "", classifyTransport(ctx, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &Error{Kind: KindHTTP, Status: resp.StatusCode, Body: snippet(string(body))}
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", &Error{Kind: KindDecode, Status: resp.StatusCode, Body: snippet(string(body)), Err: err}
	}
	if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
		return "", &Error{Kind: KindHTTP, Status: resp.StatusCode, Body: snippet(parsed.Error.Message)}
	}
	if len(parsed.Choices) == 0 {
		return "", &Error{Kind: KindEmpty, Status: resp.StatusCode}
	}
	choice := parsed.Choices[0]
	if choice.FinishReason == "length" {
		return "", &Error{Kind: KindTruncated, Status: resp.StatusCode}
	}
	cleaned := Clean(text, choice.Message.Content)
	if strings.TrimSpace(cleaned) == "" {
		return "", &Error{Kind: KindEmpty, Status: resp.StatusCode}
	}
	return cleaned, nil
}

func (c *Client) prompt() string {
	if strings.TrimSpace(c.Prompt) != "" {
		return c.Prompt
	}
	return DefaultPrompt
}

func (c *Client) maxTokens() int {
	if c.MaxTokens > 0 {
		return c.MaxTokens
	}
	return defaultMaxTokens
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func classifyTransport(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.Canceled) && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &Error{Kind: KindCanceled, Err: err}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &Error{Kind: KindTimeout, Err: err}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &Error{Kind: KindTimeout, Err: err}
	}
	return &Error{Kind: KindHTTP, Err: err, Body: snippet(err.Error())}
}

func snippet(s string) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= maxSnippetRunes {
		return s
	}
	end := 0
	n := 0
	for end < len(s) && n < maxSnippetRunes {
		_, size := utf8.DecodeRuneInString(s[end:])
		end += size
		n++
	}
	return s[:end] + "..."
}
