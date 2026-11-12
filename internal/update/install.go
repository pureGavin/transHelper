package update

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"debug/pe"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	exeName         = "transhelper.exe"
	newExeName      = "transhelper.exe.new"
	zipTempName     = ".transhelper-update.zip.tmp"
	downloadTimeout = 5 * time.Minute
)

// NewExePath 返回下载后、替换前的新程序路径。
func NewExePath(dir string) string {
	return filepath.Join(dir, newExeName)
}

// OldExePath 返回被换下来的旧程序路径。
func OldExePath(exe string) string {
	return exe + ".old"
}

// Download 把 asset 下载到 dir，校验后解压出 transhelper.exe.new。
// client 为空时使用 5 分钟超时的客户端。初始地址和每次重定向都必须是 https。
func Download(ctx context.Context, client *http.Client, asset Asset, dir string) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("缺少上下文")
	}
	if !asset.Available() {
		return "", fmt.Errorf("没有可安装的安装包")
	}
	if err := requireHTTPS(asset.URL); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	zipPath := filepath.Join(dir, zipTempName)
	newPath := NewExePath(dir)
	cleanup := func() {
		os.Remove(zipPath)
		os.Remove(newPath)
	}
	if err := downloadFile(ctx, client, asset, zipPath); err != nil {
		cleanup()
		return "", err
	}
	if err := extractExe(zipPath, newPath); err != nil {
		cleanup()
		return "", err
	}
	os.Remove(zipPath)
	if err := requirePE64(newPath); err != nil {
		os.Remove(newPath)
		return "", err
	}
	return newPath, nil
}

func downloadFile(ctx context.Context, client *http.Client, asset Asset, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	resp, err := httpsClient(client).Do(req)
	if err != nil {
		return fmt.Errorf("下载安装包: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载安装包 HTTP %d", resp.StatusCode)
	}

	file, err := os.Create(dest)
	if err != nil {
		return err
	}
	hash := sha256.New()
	// 多读 1 字节，用来发现比声明更大的包。
	n, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, asset.Size+1))
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("下载安装包: %w", copyErr)
	}
	if closeErr != nil {
		return closeErr
	}
	if n > asset.Size || n > MaxZipBytes {
		return fmt.Errorf("安装包过大")
	}
	if n != asset.Size {
		return fmt.Errorf("安装包大小不符")
	}
	sum := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(sum, asset.SHA256) {
		return fmt.Errorf("安装包校验失败")
	}
	return nil
}

func httpsClient(base *http.Client) *http.Client {
	client := &http.Client{Timeout: downloadTimeout}
	if base != nil {
		copied := *base
		client = &copied
		if client.Timeout == 0 {
			client.Timeout = downloadTimeout
		}
	}
	previous := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := requireHTTPS(req.URL.String()); err != nil {
			return err
		}
		if previous != nil {
			return previous(req, via)
		}
		if len(via) >= 10 {
			return errors.New("重定向次数过多")
		}
		return nil
	}
	return client
}

func requireHTTPS(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("下载地址必须是 https")
	}
	return nil
}

func extractExe(zipPath, dest string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("无法打开安装包: %w", err)
	}
	defer reader.Close()

	var found *zip.File
	for _, file := range reader.File {
		if filepath.ToSlash(file.Name) == exeName {
			found = file
			break
		}
	}
	if found == nil {
		return fmt.Errorf("安装包里没有 transhelper.exe")
	}
	if found.UncompressedSize64 > MaxExeBytes {
		return fmt.Errorf("程序文件过大")
	}
	source, err := found.Open()
	if err != nil {
		return err
	}
	defer source.Close()

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(out, io.LimitReader(source, MaxExeBytes+1))
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n > MaxExeBytes {
		return fmt.Errorf("程序文件过大")
	}
	return nil
}

func requirePE64(path string) error {
	file, err := pe.Open(path)
	if err != nil {
		return fmt.Errorf("不是有效的程序文件")
	}
	defer file.Close()
	if _, ok := file.OptionalHeader.(*pe.OptionalHeader64); !ok {
		return fmt.Errorf("不是 64 位程序")
	}
	return nil
}

// Swap 把正在使用的 exe 改名为 .old，再把 newExe 放到原来的路径。
// 第二步失败时会把旧文件换回去。
func Swap(exe, newExe string) error {
	old := OldExePath(exe)
	if err := os.Remove(old); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("无法删除上一份旧程序: %w", err)
	}
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("无法挪开当前程序: %w", err)
	}
	if err := os.Rename(newExe, exe); err != nil {
		if back := os.Rename(old, exe); back != nil {
			return fmt.Errorf("替换失败（%v），且无法恢复旧程序: %w", err, back)
		}
		return fmt.Errorf("替换程序失败: %w", err)
	}
	return nil
}

// Restore 在新程序没能启动时，用 .old 换回原来的 exe。
func Restore(exe string) error {
	old := OldExePath(exe)
	if _, err := os.Stat(old); err != nil {
		return fmt.Errorf("没有可恢复的旧程序: %w", err)
	}
	if err := os.Remove(exe); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("无法移除未能启动的程序: %w", err)
	}
	if err := os.Rename(old, exe); err != nil {
		return fmt.Errorf("无法恢复旧程序: %w", err)
	}
	return nil
}

// RemoveOld 删除被换下来的旧程序。文件可能仍被退出中的进程占用，所以会重试几次。
func RemoveOld(exe string) {
	old := OldExePath(exe)
	for i := 0; i < 20; i++ {
		err := os.Remove(old)
		if err == nil || os.IsNotExist(err) {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}
