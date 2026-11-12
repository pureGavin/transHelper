package translate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTranslateSuccess(t *testing.T) {
	var gotReq chatRequest
	var gotPath, gotAuth, gotType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotType = r.Header.Get("Content-Type")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("读取请求: %v", err)
		}
		if err := json.Unmarshal(body, &gotReq); err != nil {
			t.Errorf("解析请求: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"choices":[{
				"finish_reason":"stop",
				"message":{"role":"assistant","content":"<think>x</think>\n\"Hello\""}
			}]
		}`)
	}))
	defer srv.Close()

	client := &Client{
		BaseURL:     srv.URL + "/v1",
		APIKey:      "test-key",
		Model:       "demo",
		Temperature: 0,
		HTTP:        srv.Client(),
	}
	got, err := client.Translate(context.Background(), "你好")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Hello" {
		t.Fatalf("译文 = %q", got)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("路径 = %q", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if !strings.Contains(gotType, "application/json") {
		t.Fatalf("Content-Type = %q", gotType)
	}
	if gotReq.Model != "demo" || gotReq.Stream || gotReq.Temperature != 0 || gotReq.MaxTokens != defaultMaxTokens {
		t.Fatalf("请求体 = %+v", gotReq)
	}
	if len(gotReq.Messages) != 2 || gotReq.Messages[0].Role != "system" || gotReq.Messages[1].Content != "你好" {
		t.Fatalf("消息 = %+v", gotReq.Messages)
	}
	if gotReq.Messages[0].Content != DefaultPrompt {
		t.Fatal("未填写提示词时应使用内置提示词")
	}
}

func TestTranslateOmitsAuthorizationWhenKeyEmpty(t *testing.T) {
	var gotAuth string
	var hasAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, hasAuth = r.Header["Authorization"]
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"content":"Hi"}}]}`)
	}))
	defer srv.Close()

	client := &Client{BaseURL: srv.URL, Model: "demo", Prompt: "custom", HTTP: srv.Client()}
	if _, err := client.Translate(context.Background(), "你好"); err != nil {
		t.Fatal(err)
	}
	if hasAuth || gotAuth != "" {
		t.Fatalf("空密钥不应发送 Authorization，实际 %q", gotAuth)
	}
}

func TestTranslateFailures(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		kind   Kind
	}{
		{name: "未授权", status: http.StatusUnauthorized, body: `{"error":{"message":"bad key"}}`, kind: KindHTTP},
		{name: "服务器错误", status: http.StatusInternalServerError, body: `boom`, kind: KindHTTP},
		{name: "无法解析", status: http.StatusOK, body: `not-json`, kind: KindDecode},
		{name: "没有候选", status: http.StatusOK, body: `{"choices":[]}`, kind: KindEmpty},
		{name: "内容为空", status: http.StatusOK, body: `{"choices":[{"finish_reason":"stop","message":{"content":"  "}}]}`, kind: KindEmpty},
		{name: "被截断", status: http.StatusOK, body: `{"choices":[{"finish_reason":"length","message":{"content":"Hello"}}]}`, kind: KindTruncated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer srv.Close()

			client := &Client{BaseURL: srv.URL, Model: "demo", HTTP: srv.Client()}
			_, err := client.Translate(context.Background(), "你好")
			var te *Error
			if !errors.As(err, &te) {
				t.Fatalf("错误 = %v，期望 *Error", err)
			}
			if te.Kind != tt.kind {
				t.Fatalf("Kind = %d, 期望 %d (%v)", te.Kind, tt.kind, err)
			}
			if tt.status >= 400 && te.Status != tt.status {
				t.Fatalf("Status = %d, 期望 %d", te.Status, tt.status)
			}
			if tt.status >= 400 && te.Body == "" {
				t.Fatal("HTTP 错误应该保留截断后的响应体")
			}
			if msg := UserMessage(err); !strings.Contains(msg, "原文未改动") {
				t.Fatalf("用户提示 = %q", msg)
			}
		})
	}
}

func TestTranslateTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	client := &Client{BaseURL: srv.URL, Model: "demo", HTTP: srv.Client()}
	start := time.Now()
	_, err := client.Translate(ctx, "你好")
	if time.Since(start) > time.Second {
		t.Fatal("超时后没有及时返回")
	}
	var te *Error
	if !errors.As(err, &te) || te.Kind != KindTimeout {
		t.Fatalf("错误 = %v", err)
	}
	if UserMessage(err) != "翻译超时，原文未改动" {
		t.Fatalf("用户提示 = %q", UserMessage(err))
	}
}

func TestTranslateCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &Client{BaseURL: "http://127.0.0.1:9", Model: "demo"}
	_, err := client.Translate(ctx, "你好")
	var te *Error
	if !errors.As(err, &te) || te.Kind != KindCanceled {
		t.Fatalf("错误 = %v", err)
	}
}
