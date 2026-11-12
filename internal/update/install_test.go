package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadExtractsExecutable(t *testing.T) {
	exe := testExecutable(t)
	payload := makeZip(t, exeName, exe)
	dir := t.TempDir()
	server := serveZip(t, payload, false)
	defer server.Close()

	got, err := Download(context.Background(), server.Client(), assetFor(server.URL, payload), dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != NewExePath(dir) {
		t.Fatalf("路径 = %s", got)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, exe) {
		t.Fatal("解压出的程序和原文件不一致")
	}
	if _, err := os.Stat(filepath.Join(dir, zipTempName)); !os.IsNotExist(err) {
		t.Fatal("zip 临时文件应该被删掉")
	}
}

func TestDownloadRejectsBadPackage(t *testing.T) {
	exe := []byte("not a pe")
	cases := []struct {
		name    string
		zipName string
		body    []byte
		asset   func(url string, body []byte) Asset
		want    string
	}{
		{
			name:    "校验失败",
			zipName: exeName,
			body:    makeZip(t, exeName, exe),
			asset: func(raw string, body []byte) Asset {
				asset := assetFor(raw, body)
				asset.SHA256 = stringsRepeatA()
				return asset
			},
			want: "校验失败",
		},
		{
			name:    "没有程序",
			zipName: "bin/" + exeName,
			body:    makeZip(t, "bin/"+exeName, exe),
			asset:   assetFor,
			want:    "没有 transhelper.exe",
		},
		{
			name:    "过大",
			zipName: exeName,
			body:    makeZip(t, exeName, exe),
			asset: func(raw string, body []byte) Asset {
				asset := assetFor(raw, body)
				asset.Size = 8
				return asset
			},
			want: "过大",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			server := serveZip(t, tt.body, false)
			defer server.Close()
			_, err := Download(context.Background(), server.Client(), tt.asset(server.URL, tt.body), dir)
			if err == nil || !bytes.Contains([]byte(err.Error()), []byte(tt.want)) {
				t.Fatalf("错误 = %v，期望包含 %s", err, tt.want)
			}
			assertEmpty(t, dir)
		})
	}
}

func TestDownloadRejectsHTTPRedirect(t *testing.T) {
	dir := t.TempDir()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.com/transhelper.zip", http.StatusFound)
	}))
	defer server.Close()
	asset := Asset{
		Name:   "transHelper-2026.11.1-windows-amd64.zip",
		URL:    server.URL,
		Size:   4,
		SHA256: stringsRepeatA(),
	}
	_, err := Download(context.Background(), server.Client(), asset, dir)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("https")) {
		t.Fatalf("错误 = %v", err)
	}
	assertEmpty(t, dir)
}

func TestSwapAndRestore(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, exeName)
	if err := os.WriteFile(exe, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing.new")
	if err := Swap(exe, missing); err == nil {
		t.Fatal("新文件不存在时应该失败")
	}
	data, err := os.ReadFile(exe)
	if err != nil || string(data) != "old" {
		t.Fatalf("失败后原文件 = %q, %v", data, err)
	}
	if _, err := os.Stat(OldExePath(exe)); !os.IsNotExist(err) {
		t.Fatal("回滚后不应该留下 .old")
	}

	newExe := NewExePath(dir)
	if err := os.WriteFile(newExe, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Swap(exe, newExe); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(exe)
	if err != nil || string(data) != "new" {
		t.Fatalf("替换后 = %q, %v", data, err)
	}
	old, err := os.ReadFile(OldExePath(exe))
	if err != nil || string(old) != "old" {
		t.Fatalf("旧文件 = %q, %v", old, err)
	}

	if err := Restore(exe); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(exe)
	if err != nil || string(data) != "old" {
		t.Fatalf("恢复后 = %q, %v", data, err)
	}
	if _, err := os.Stat(OldExePath(exe)); !os.IsNotExist(err) {
		t.Fatal("恢复后不应该留下 .old")
	}
}

func TestRemoveOld(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, exeName)
	old := OldExePath(exe)
	if err := os.WriteFile(old, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	RemoveOld(exe)
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("旧程序应该被删除")
	}
	RemoveOld(exe)
}

func testExecutable(t *testing.T) []byte {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func makeZip(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	file, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func serveZip(t *testing.T, payload []byte, fail bool) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "no", http.StatusInternalServerError)
			return
		}
		w.Write(payload)
	}))
}

func assetFor(raw string, body []byte) Asset {
	sum := sha256.Sum256(body)
	return Asset{
		Name:   "transHelper-2026.11.1-windows-amd64.zip",
		URL:    raw,
		Size:   int64(len(body)),
		SHA256: hex.EncodeToString(sum[:]),
	}
}

func stringsRepeatA() string {
	return "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
}

func assertEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("目录里还有文件: %v", names)
	}
}
