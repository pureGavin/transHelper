package update

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"2026.11.10", "2026.11.9", 1},
		{"2027.01.1", "2026.12.9", 1},
		{"v2026.11.1", "2026.11.1", 0},
		{"2026.11.1", "2026.12.1", -1},
		{"2026.11.0", "2026.11.1", -1},
	}
	for _, tt := range tests {
		got, err := Compare(tt.a, tt.b)
		if err != nil {
			t.Fatalf("Compare(%s, %s): %v", tt.a, tt.b, err)
		}
		if got != tt.want {
			t.Fatalf("Compare(%s, %s) = %d，期望 %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCompareRejectsBadVersion(t *testing.T) {
	if _, err := Compare("dev", "2026.11.1"); err == nil {
		t.Fatal("dev 应该无法比较")
	}
	if _, err := Compare("2026.11.1", "2026.13.1"); err == nil {
		t.Fatal("非法月份应该无法比较")
	}
}

func TestIsRelease(t *testing.T) {
	if IsRelease("dev") || IsRelease("2026.11.0") || IsRelease("") {
		t.Fatal("开发构建和末尾为 0 的版本不是正式版")
	}
	if !IsRelease("2026.11.1") || !IsRelease("v2027.01.1") {
		t.Fatal("正式版应该被认出来")
	}
}

func TestAllowedPage(t *testing.T) {
	if !AllowedPage("https://github.com/pureGavin/transHelper/releases/tag/v2026.11.1") {
		t.Fatal("本仓库的 Release 页面应该允许打开")
	}
	for _, raw := range []string{
		"http://github.com/pureGavin/transHelper/releases/tag/v2026.11.1",
		"https://example.com/pureGavin/transHelper/releases/latest",
		"https://github.com/other/transHelper/releases/latest",
		"https://user:pass@github.com/pureGavin/transHelper/releases/latest",
	} {
		if AllowedPage(raw) {
			t.Fatalf("不应该允许 %s", raw)
		}
	}
}

func TestLatest(t *testing.T) {
	var gotAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAgent = r.Header.Get("User-Agent")
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		switch r.URL.Path {
		case "/ok":
			io.WriteString(w, `{"tag_name":"v2026.11.1","html_url":"https://github.com/pureGavin/transHelper/releases/tag/v2026.11.1"}`)
		case "/missing":
			w.WriteHeader(http.StatusNotFound)
		case "/limit":
			w.WriteHeader(http.StatusForbidden)
		case "/bad-tag":
			io.WriteString(w, `{"tag_name":"v1.2.3","html_url":"https://github.com/pureGavin/transHelper/releases/tag/v1.2.3"}`)
		case "/bad-url":
			io.WriteString(w, `{"tag_name":"v2026.11.1","html_url":"https://example.com/download"}`)
		case "/huge":
			w.Write(bytesOf('x', maxBodyBytes+1))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := server.Client()
	ctx := context.Background()

	rel, err := Latest(ctx, client, server.URL+"/ok", "transHelper/2026.11.1")
	if err != nil {
		t.Fatal(err)
	}
	if rel.Version != "2026.11.1" {
		t.Fatalf("Version = %q", rel.Version)
	}
	if !strings.HasSuffix(rel.URL, "/releases/tag/v2026.11.1") {
		t.Fatalf("URL = %q", rel.URL)
	}
	if gotAgent != "transHelper/2026.11.1" {
		t.Fatalf("User-Agent = %q", gotAgent)
	}

	_, err = Latest(ctx, client, server.URL+"/missing", "transHelper")
	if !errors.Is(err, ErrNoRelease) {
		t.Fatalf("404 错误 = %v", err)
	}
	_, err = Latest(ctx, client, server.URL+"/limit", "transHelper")
	if err == nil || !strings.Contains(err.Error(), "限制") {
		t.Fatalf("403 错误 = %v", err)
	}
	if _, err := Latest(ctx, client, server.URL+"/bad-tag", "transHelper"); err == nil {
		t.Fatal("非法 tag 应该失败")
	}
	if _, err := Latest(ctx, client, server.URL+"/bad-url", "transHelper"); err == nil {
		t.Fatal("站外页面应该失败")
	}
	if _, err := Latest(ctx, client, server.URL+"/huge", "transHelper"); err == nil {
		t.Fatal("过大的响应应该失败")
	}
}

func TestDescribe(t *testing.T) {
	page := "https://github.com/pureGavin/transHelper/releases/tag/v2026.11.2"
	rel := Release{Version: "2026.11.2", URL: page}
	withAsset := rel
	withAsset.Asset = Asset{
		Name:   "transHelper-2026.11.2-windows-amd64.zip",
		URL:    "https://github.com/pureGavin/transHelper/releases/download/v2026.11.2/transHelper-2026.11.2-windows-amd64.zip",
		Size:   100,
		SHA256: strings.Repeat("a", 64),
	}

	message, action, show := Describe("2026.11.2", true, "", rel, nil)
	if !show || message != "已是最新版本 2026.11.2" || action != ActionNone {
		t.Fatalf("已是最新: %q %v %v", message, action, show)
	}

	message, action, show = Describe("2026.11.1", true, "", withAsset, nil)
	if !show || message != "发现新版本 2026.11.2，点击这里更新" || action != ActionInstall {
		t.Fatalf("可安装的新版本: %q %v %v", message, action, show)
	}
	message, action, show = Describe("2026.11.1", true, "", rel, nil)
	if !show || message != "发现新版本 2026.11.2，点击打开下载页" || action != ActionOpen {
		t.Fatalf("没有安装包时: %q %v %v", message, action, show)
	}

	_, action, show = Describe("2026.11.1", false, "", withAsset, nil)
	if !show || action != ActionInstall {
		t.Fatal("自动检查发现可安装的新版本时应该通知")
	}
	_, _, show = Describe("2026.11.1", false, "2026.11.2", withAsset, nil)
	if show {
		t.Fatal("同一个新版本不应该重复自动通知")
	}
	_, _, show = Describe("2026.11.2", false, "", rel, nil)
	if show {
		t.Fatal("自动检查发现已是最新时不应该打扰")
	}

	message, action, show = Describe("dev", true, "", withAsset, nil)
	if !show || action != ActionNone || !strings.Contains(message, "本机构建 dev") || !strings.Contains(message, "2026.11.2") {
		t.Fatalf("本机构建: %q %v %v", message, action, show)
	}
	_, _, show = Describe("2026.11.0", false, "", withAsset, nil)
	if show {
		t.Fatal("本机构建的自动检查不应该通知")
	}

	message, action, show = Describe("2026.11.1", true, "", Release{}, ErrNoRelease)
	if !show || action != ActionNone || message != "还没有已发布的版本" {
		t.Fatalf("没有正式版: %q %v %v", message, action, show)
	}
	_, _, show = Describe("2026.11.1", false, "", Release{}, ErrNoRelease)
	if show {
		t.Fatal("自动检查没有正式版时不应该打扰")
	}
	message, action, show = Describe("2026.11.1", true, "", Release{}, errors.New("连接失败"))
	if !show || action != ActionNone || message != "检查更新失败，请检查网络" {
		t.Fatalf("失败: %q %v %v", message, action, show)
	}
}

func bytesOf(b byte, n int) []byte {
	return []byte(strings.Repeat(string(b), n))
}

func TestMain(m *testing.M) {
	// 改名测试会把本测试程序复制出去再启动。那个进程只需要占住文件。
	if os.Getenv("TRANSHELPER_HOLD") == "1" {
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestLatestSelectsInstallableAsset(t *testing.T) {
	sum := strings.Repeat("ab", 32)
	page := "https://github.com/pureGavin/transHelper/releases/tag/v2026.11.1"
	good := "https://github.com/pureGavin/transHelper/releases/download/v2026.11.1/transHelper-2026.11.1-windows-amd64.zip"
	body := `{
		"tag_name":"v2026.11.1",
		"html_url":"` + page + `",
		"assets":[
			{"name":"notes.txt","size":10,"browser_download_url":"` + good + `","digest":"sha256:` + sum + `"},
			{"name":"transHelper-2026.11.1-windows-amd64.zip","size":10,"browser_download_url":"http://github.com/pureGavin/transHelper/releases/download/v2026.11.1/transHelper-2026.11.1-windows-amd64.zip","digest":"sha256:` + sum + `"},
			{"name":"transHelper-2026.11.1-windows-amd64.zip","size":10,"browser_download_url":"` + good + `","digest":"sha256:zz"},
			{"name":"transHelper-2026.11.1-windows-amd64.zip","size":99999999999,"browser_download_url":"` + good + `","digest":"sha256:` + sum + `"},
			{"name":"transHelper-2026.11.1-windows-amd64.zip","size":1234,"browser_download_url":"` + good + `","digest":"sha256:` + sum + `"}
		]
	}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	defer server.Close()

	rel, err := Latest(context.Background(), server.Client(), server.URL, "transHelper")
	if err != nil {
		t.Fatal(err)
	}
	if !rel.Asset.Available() || rel.Asset.Size != 1234 || rel.Asset.SHA256 != sum || rel.Asset.URL != good {
		t.Fatalf("选中的附件不对: %+v", rel.Asset)
	}
}

func TestLatestIgnoresUnusableAssets(t *testing.T) {
	body := `{
		"tag_name":"v2026.11.1",
		"html_url":"https://github.com/pureGavin/transHelper/releases/tag/v2026.11.1",
		"assets":[
			{"name":"transHelper-2026.11.1-windows-amd64.zip","size":10,"browser_download_url":"https://example.com/transHelper-2026.11.1-windows-amd64.zip","digest":"sha256:` + strings.Repeat("a", 64) + `"}
		]
	}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, body)
	}))
	defer server.Close()
	rel, err := Latest(context.Background(), server.Client(), server.URL, "transHelper")
	if err != nil {
		t.Fatal(err)
	}
	if rel.Asset.Available() {
		t.Fatalf("站外附件不该被选中: %+v", rel.Asset)
	}
}
