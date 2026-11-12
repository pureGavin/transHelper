//go:build windows

package win

import (
	"context"
	"runtime"
	"time"
	"unsafe"

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

func (h *host) checkUpdate(manual bool) {
	if !h.checking.CompareAndSwap(false, true) {
		if manual {
			h.info("正在检查更新", "")
		}
		return
	}
	if manual {
		h.info("正在检查更新", "")
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
	message, page, show := update.Describe(h.version, item.manual, h.notifiedVersion, item.release, item.err)
	if !show {
		return
	}
	if !item.manual && page != "" {
		h.notifiedVersion = item.release.Version
	}
	h.info(message, page)
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
	raw := h.balloonURL
	h.balloonURL = ""
	if !update.AllowedPage(raw) {
		return
	}
	openBrowser(raw)
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
