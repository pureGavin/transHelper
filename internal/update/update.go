// Package update 查询 GitHub Release，并下载、校验、替换程序。它不调用 Win32。
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// LatestURL 是本仓库最新正式版的 API 地址。
	LatestURL = "https://api.github.com/repos/pureGavin/transHelper/releases/latest"
	// RepoURL 是本仓库的 GitHub 页面。
	RepoURL = "https://github.com/pureGavin/transHelper"

	maxBodyBytes     = 1 << 20
	defaultTimeout   = 10 * time.Second
	defaultUserAgent = "transHelper"
)

// FileSHA256 计算 path 的 SHA256，返回小写十六进制。
func FileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// ErrNoRelease 表示仓库还没有任何正式版。
var ErrNoRelease = errors.New("还没有正式版本")

const (
	// MaxZipBytes 是可下载安装包的上限。
	MaxZipBytes = 64 << 20
	// MaxExeBytes 是解压出的 exe 的上限。
	MaxExeBytes = 128 << 20
)

// Asset 是 Release 上可用于自动更新的 zip。零值表示没有合格附件。
type Asset struct {
	Name   string
	URL    string
	Size   int64
	SHA256 string
}

// Available 报告附件是否可以下载安装。
func (a Asset) Available() bool {
	return a.Name != "" && a.URL != "" && a.Size > 0 && a.SHA256 != ""
}

// Release 是 GitHub 上的一个正式版本。
type Release struct {
	// Version 不带前导 v，例如 2026.11.1。
	Version string
	// URL 是这个版本的 Release 页面。
	URL string
	// Asset 是自动更新用的 zip。没有合格附件时为零值。
	Asset Asset
}

// Action 是点击更新通知后要做的事。
type Action int

const (
	// ActionNone 表示通知不能点击。
	ActionNone Action = iota
	// ActionOpen 表示打开 Release 页面。
	ActionOpen
	// ActionInstall 表示下载并替换当前程序。
	ActionInstall
)

type version struct {
	year  int
	month int
	patch int
}

// IsRelease 报告 v 是不是正式发布的版本号。
// dev 和末尾为 0 的本机构建都不是。
func IsRelease(v string) bool {
	parsed, err := parseVersion(v)
	return err == nil && parsed.patch != 0
}

// Compare 比较两个 YYYY.MM.x 版本。a 更新时返回 1，相同返回 0，a 更旧返回 -1。
// 可以带前导 v。任一边无法解析时返回错误。
func Compare(a, b string) (int, error) {
	left, err := parseVersion(a)
	if err != nil {
		return 0, fmt.Errorf("版本 %q: %w", a, err)
	}
	right, err := parseVersion(b)
	if err != nil {
		return 0, fmt.Errorf("版本 %q: %w", b, err)
	}
	switch {
	case left.year != right.year:
		return cmpInt(left.year, right.year), nil
	case left.month != right.month:
		return cmpInt(left.month, right.month), nil
	default:
		return cmpInt(left.patch, right.patch), nil
	}
}

func cmpInt(a, b int) int {
	switch {
	case a > b:
		return 1
	case a < b:
		return -1
	default:
		return 0
	}
}

func parseVersion(v string) (version, error) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return version{}, fmt.Errorf("版本号必须是 YYYY.MM.x")
	}
	year, err := strconv.Atoi(parts[0])
	if err != nil || year < 1000 || year > 9999 {
		return version{}, fmt.Errorf("版本号必须是 YYYY.MM.x")
	}
	month, err := strconv.Atoi(parts[1])
	if err != nil || month < 1 || month > 12 || len(parts[1]) != 2 {
		return version{}, fmt.Errorf("版本号必须是 YYYY.MM.x")
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil || patch < 0 || strings.HasPrefix(parts[2], "+") {
		return version{}, fmt.Errorf("版本号必须是 YYYY.MM.x")
	}
	return version{year: year, month: month, patch: patch}, nil
}

// Describe 决定这次检查要不要通知用户，以及点击通知后做什么。
// manual 为 false 时，只有发现更新的正式版才通知。
// already 是本轮运行里自动检查已经提示过的版本，同一个版本不再重复提示。
// 有合格安装包时动作为安装；没有时退回打开下载页。本机构建只说明，不带动作。
func Describe(current string, manual bool, already string, rel Release, err error) (message string, action Action, show bool) {
	if err != nil {
		if !manual {
			return "", ActionNone, false
		}
		if errors.Is(err, ErrNoRelease) {
			return "还没有已发布的版本", ActionNone, true
		}
		return "检查更新失败，请检查网络", ActionNone, true
	}
	if !IsRelease(current) {
		if !manual {
			return "", ActionNone, false
		}
		return fmt.Sprintf("当前是本机构建 %s，最新正式版是 %s", current, rel.Version), ActionNone, true
	}
	cmp, cmpErr := Compare(rel.Version, current)
	if cmpErr != nil {
		if !manual {
			return "", ActionNone, false
		}
		return "检查更新失败，请检查网络", ActionNone, true
	}
	if cmp <= 0 {
		if !manual {
			return "", ActionNone, false
		}
		return "已是最新版本 " + current, ActionNone, true
	}
	if !manual && already == rel.Version {
		return "", ActionNone, false
	}
	if rel.Asset.Available() {
		return "发现新版本 " + rel.Version + "，点击这里更新", ActionInstall, true
	}
	return "发现新版本 " + rel.Version + "，点击打开下载页", ActionOpen, true
}

// AllowedPage 报告 raw 是不是本仓库的 GitHub Release 页面。
func AllowedPage(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, "github.com") {
		return false
	}
	if parsed.User != nil {
		return false
	}
	return strings.HasPrefix(parsed.Path, "/pureGavin/transHelper/releases/")
}

type releasePayload struct {
	TagName string         `json:"tag_name"`
	HTMLURL string         `json:"html_url"`
	Assets  []assetPayload `json:"assets"`
}

type assetPayload struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

// Latest 请求最新正式版。client 为空时使用 10 秒超时的客户端。
// rawURL 为空时使用 LatestURL。userAgent 为空时使用 transHelper。
func Latest(ctx context.Context, client *http.Client, rawURL, userAgent string) (Release, error) {
	if ctx == nil {
		return Release{}, fmt.Errorf("缺少上下文")
	}
	if strings.TrimSpace(rawURL) == "" {
		rawURL = LatestURL
	}
	if strings.TrimSpace(userAgent) == "" {
		userAgent = defaultUserAgent
	}
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("连接 GitHub: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return Release{}, fmt.Errorf("读取 GitHub 响应: %w", err)
	}
	if len(body) > maxBodyBytes {
		return Release{}, fmt.Errorf("GitHub 响应过大")
	}
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return Release{}, ErrNoRelease
	case http.StatusForbidden, http.StatusTooManyRequests:
		return Release{}, fmt.Errorf("GitHub 限制了请求次数")
	default:
		return Release{}, fmt.Errorf("GitHub 返回 HTTP %d", resp.StatusCode)
	}

	var payload releasePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return Release{}, fmt.Errorf("GitHub 响应无法解析: %w", err)
	}
	parsed, err := parseVersion(payload.TagName)
	if err != nil {
		return Release{}, fmt.Errorf("tag %q: %w", payload.TagName, err)
	}
	if !AllowedPage(payload.HTMLURL) {
		return Release{}, fmt.Errorf("Release 页面地址不可用")
	}
	versionText := fmt.Sprintf("%04d.%02d.%d", parsed.year, parsed.month, parsed.patch)
	return Release{
		Version: versionText,
		URL:     payload.HTMLURL,
		Asset:   selectAsset(versionText, payload.Assets),
	}, nil
}

// selectAsset 挑出可以自动安装的 zip。不合格的附件会被跳过。
func selectAsset(version string, assets []assetPayload) Asset {
	wantName := "transHelper-" + version + "-windows-amd64.zip"
	prefix := "https://github.com/pureGavin/transHelper/releases/download/v" + version + "/"
	for _, item := range assets {
		if item.Name != wantName || !strings.HasPrefix(item.BrowserDownloadURL, prefix) {
			continue
		}
		sum, ok := parseDigest(item.Digest)
		if !ok || item.Size <= 0 || item.Size > MaxZipBytes {
			continue
		}
		return Asset{
			Name:   item.Name,
			URL:    item.BrowserDownloadURL,
			Size:   item.Size,
			SHA256: sum,
		}
	}
	return Asset{}
}

func parseDigest(digest string) (string, bool) {
	const prefix = "sha256:"
	if !strings.HasPrefix(digest, prefix) {
		return "", false
	}
	sum := strings.ToLower(digest[len(prefix):])
	if len(sum) != 64 {
		return "", false
	}
	for _, c := range sum {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", false
		}
	}
	return sum, true
}
