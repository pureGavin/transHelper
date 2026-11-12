//go:build windows

package win

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"transhelper/internal/config"
	"transhelper/internal/update"
)

const (
	// 启动后稍等再查，避免和创建托盘挤在一起。之后每天查一次。
	autoCheckInitial = 30 * time.Second
	autoCheckEvery   = 24 * time.Hour
)

type updateOutcome struct {
	manual  bool
	release update.Release
	err     error
}

// trayAction 是当前气泡被点击后要做的事。
type trayAction struct {
	open    bool
	install bool
	page    string
	rel     update.Release
}

type installOutcome struct {
	exe    string
	newExe string
	page   string
	err    error
}

func (h *host) checkUpdate(manual bool) {
	if !h.checking.CompareAndSwap(false, true) {
		if manual {
			h.info("正在检查更新")
		}
		return
	}
	if manual {
		h.info("正在检查更新")
	}
	go func() {
		defer h.checking.Store(false)
		ctx, cancel := context.WithTimeout(h.ctx, 10*time.Second)
		defer cancel()
		rel, err := update.Latest(ctx, nil, update.LatestURL, h.userAgent())
		h.pushUpdate(updateOutcome{manual: manual, release: rel, err: err})
	}()
}

func (h *host) userAgent() string {
	version := h.version
	if version == "" {
		version = "dev"
	}
	return "transHelper/" + version
}

func (h *host) pushUpdate(item updateOutcome) {
	h.updateMu.Lock()
	h.updateQueue = append(h.updateQueue, item)
	h.updateMu.Unlock()
	if h.hwnd != 0 {
		procPostMessage.Call(h.hwnd, wmUpdateResult, 0, 0)
	}
}

func (h *host) onUpdateResult() {
	h.updateMu.Lock()
	items := h.updateQueue
	h.updateQueue = nil
	h.updateMu.Unlock()
	for _, item := range items {
		h.showUpdate(item)
	}
}

func (h *host) showUpdate(item updateOutcome) {
	if item.err != nil {
		h.logger.Printf("检查更新失败: %v", item.err)
	}
	message, action, show := update.Describe(h.version, item.manual, h.notifiedVersion, item.release, item.err)
	if !show {
		return
	}
	if !item.manual && action != update.ActionNone {
		h.notifiedVersion = item.release.Version
	}
	switch action {
	case update.ActionInstall:
		h.infoAction(message, trayAction{install: true, rel: item.release, page: item.release.URL})
	case update.ActionOpen:
		h.infoAction(message, trayAction{open: true, page: item.release.URL})
	default:
		h.info(message)
	}
}

func (h *host) toggleAutoCheck() {
	cfg := h.cfg
	cfg.AutoCheckUpdate = !cfg.AutoCheckUpdate
	if err := config.Save(h.configPath, cfg); err != nil {
		h.logger.Printf("保存自动检查设置失败: %v", err)
		h.Notify("保存自动检查设置失败")
		return
	}
	h.cfg = cfg
	h.syncAutoCheck()
}

func (h *host) syncAutoCheck() {
	if h.autoCancel != nil {
		h.autoCancel()
		h.autoCancel = nil
	}
	if !h.cfg.AutoCheckUpdate || !update.IsRelease(h.version) {
		return
	}
	ctx, cancel := context.WithCancel(h.ctx)
	h.autoCancel = cancel
	go h.autoCheckLoop(ctx)
}

func (h *host) autoCheckLoop(ctx context.Context) {
	timer := time.NewTimer(autoCheckInitial)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if ctx.Err() != nil {
				return
			}
			h.checkUpdate(false)
			timer.Reset(autoCheckEvery)
		}
	}
}

func (h *host) openBalloon() {
	action := h.clickAction
	h.clickAction = trayAction{}
	switch {
	case action.install:
		h.startInstall(action.rel)
	case action.open && update.AllowedPage(action.page):
		openBrowser(action.page)
	}
}

func (h *host) startInstall(rel update.Release) {
	if !rel.Asset.Available() {
		if update.AllowedPage(rel.URL) {
			openBrowser(rel.URL)
		}
		return
	}
	if !h.updating.CompareAndSwap(false, true) {
		h.info("正在更新")
		return
	}
	h.info("正在下载更新")
	go h.downloadUpdate(rel)
}

func (h *host) downloadUpdate(rel update.Release) {
	ctx, cancel := context.WithTimeout(h.ctx, 5*time.Minute)
	defer cancel()
	exe, err := os.Executable()
	if err != nil {
		h.reportInstall(installOutcome{err: err, page: rel.URL})
		return
	}
	newExe, err := update.Download(ctx, nil, rel.Asset, filepath.Dir(exe))
	if err != nil {
		if h.ctx.Err() != nil {
			h.updating.Store(false)
			return
		}
		h.reportInstall(installOutcome{err: err, page: rel.URL})
		return
	}
	deadline := time.Now().Add(30 * time.Second)
	for h.busy.Load() && time.Now().Before(deadline) {
		if h.ctx.Err() != nil {
			os.Remove(newExe)
			h.updating.Store(false)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	if h.busy.Load() {
		os.Remove(newExe)
		h.reportInstall(installOutcome{err: fmt.Errorf("翻译尚未结束"), page: rel.URL})
		return
	}
	h.reportInstall(installOutcome{exe: exe, newExe: newExe, page: rel.URL})
}

func (h *host) reportInstall(item installOutcome) {
	if h.hwnd == 0 || h.ctx.Err() != nil {
		if item.newExe != "" {
			os.Remove(item.newExe)
		}
		h.updating.Store(false)
		return
	}
	h.installMu.Lock()
	h.installQueue = append(h.installQueue, item)
	h.installMu.Unlock()
	procPostMessage.Call(h.hwnd, wmInstallReady, 0, 0)
}

func (h *host) onInstallReady() {
	h.installMu.Lock()
	items := h.installQueue
	h.installQueue = nil
	h.installMu.Unlock()
	for _, item := range items {
		h.applyInstall(item)
		if h.restart {
			return
		}
	}
}

func (h *host) applyInstall(item installOutcome) {
	if item.err != nil {
		h.failInstall(item.err, item.page)
		return
	}
	if err := update.Swap(item.exe, item.newExe); err != nil {
		os.Remove(item.newExe)
		h.failInstall(err, item.page)
		return
	}
	// 旧进程先退出并放开单实例锁，再由 main 启动新程序。
	h.restart = true
	h.cancel()
	procPostQuitMessage.Call(0)
}

func (h *host) failInstall(err error, page string) {
	h.updating.Store(false)
	h.logger.Printf("更新失败: %v", err)
	h.infoAction("更新失败："+installReason(err)+"，点击打开下载页", trayAction{open: true, page: page})
}

func installReason(err error) string {
	if os.IsPermission(err) || errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return "程序目录不能写入"
	}
	return err.Error()
}

func openBrowser(raw string) {
	verb := utf16String("open")
	target := utf16String(raw)
	// 返回值小于等于 32 才是失败。打不开时用户仍能从菜单再检查一次。
	procShellExecute.Call(
		0,
		uintptr(unsafe.Pointer(&verb[0])),
		uintptr(unsafe.Pointer(&target[0])),
		0,
		0,
		swShowNormal,
	)
	runtime.KeepAlive(verb)
	runtime.KeepAlive(target)
}
