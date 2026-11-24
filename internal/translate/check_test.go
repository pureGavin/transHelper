package translate

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDescribeCheckSuccess(t *testing.T) {
	got := DescribeCheck("Hello\nthere", 800*time.Millisecond, nil)
	if got != "可用，耗时 0.8 秒；译文：Hello there" {
		t.Fatalf("成功 = %q", got)
	}
	fast := DescribeCheck("Hi", 12*time.Millisecond, nil)
	if fast != "可用，耗时 12 毫秒；译文：Hi" {
		t.Fatalf("很快的成功 = %q", fast)
	}
	long := DescribeCheck(strings.Repeat("字", checkMaxRunes+5), time.Second, nil)
	if !strings.HasSuffix(long, "…") || strings.Count(long, "字") != checkMaxRunes {
		t.Fatalf("过长译文 = %q", long)
	}
}

func TestCheckReports(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
		kind   string
		sample bool
	}{
		{
			name:   "可用",
			status: http.StatusOK,
			body:   `{"choices":[{"finish_reason":"stop","message":{"content":"Hello"}}]}`,
			want:   "译文：Hello",
			sample: true,
		},
		{
			name:   "密钥不对",
			status: http.StatusUnauthorized,
			body:   `{"error":{"message":"bad key"}}`,
			want:   "模型返回 HTTP 401，请检查 API Key",
			kind:   "HTTP 401",
		},
		{
			name:   "找不到",
			status: http.StatusNotFound,
			body:   `missing`,
			want:   "模型返回 HTTP 404，请检查地址和模型名称",
			kind:   "HTTP 404",
		},
		{
			name:   "服务器错误",
			status: http.StatusInternalServerError,
			body:   `boom`,
			want:   "模型返回 HTTP 500",
			kind:   "HTTP 500",
		},
		{
			name:   "不是接口",
			status: http.StatusOK,
			body:   `not-json`,
			want:   "模型返回无法解析，请确认是 OpenAI 兼容接口",
			kind:   "无法解析",
		},
		{
			name:   "空结果",
			status: http.StatusOK,
			body:   `{"choices":[]}`,
			want:   "模型返回空结果",
			kind:   "空结果",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sawSample bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				sawSample = strings.Contains(string(body), CheckSample)
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer srv.Close()

			client := &Client{BaseURL: srv.URL + "/v1", Model: "demo", HTTP: srv.Client()}
			msg, err := Check(context.Background(), client)
			if tt.kind == "" && err != nil {
				t.Fatal(err)
			}
			if tt.kind != "" && err == nil {
				t.Fatal("失败时应该返回错误")
			}
			if !strings.Contains(msg, tt.want) {
				t.Fatalf("结果 = %q，期望包含 %q", msg, tt.want)
			}
			if strings.Contains(msg, tt.body) && tt.body != "" && tt.status != http.StatusOK {
				t.Fatalf("结果不该带上响应正文 %q：%s", tt.body, msg)
			}
			if CheckKind(err) != tt.kind {
				t.Fatalf("类别 = %q，期望 %q", CheckKind(err), tt.kind)
			}
			if tt.sample && !sawSample {
				t.Fatal("没有把固定样句发给模型")
			}
		})
	}
}

func TestCheckTimeout(t *testing.T) {
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
	msg, err := Check(ctx, client)
	if msg != "测试超时，请加大超时或检查网络" || CheckKind(err) != "超时" {
		t.Fatalf("超时 = %q %v", msg, err)
	}
}

func TestCheckUnreachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client := &Client{
		BaseURL: "http://" + addr,
		Model:   "demo",
		HTTP:    &http.Client{Transport: &http.Transport{Proxy: nil}},
	}
	msg, err := Check(ctx, client)
	if msg != "连不上模型，请检查地址、网络和代理" || CheckKind(err) != "连不上" {
		t.Fatalf("连不上 = %q %v", msg, err)
	}
}
