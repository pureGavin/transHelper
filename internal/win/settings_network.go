//go:build windows

package win

import (
	"golang.org/x/sys/windows"

	"transhelper/internal/config"
)

// networkPage 是「网络」页。代理控件加在这个文件里。
// 先占住标签，避免以后插入时打乱「通用 / 网络 / 关于」的顺序。
type networkPage struct{}

func (n *networkPage) controls() []uintptr { return nil }

func (n *networkPage) show(bool) {}

func (n *networkPage) place(*settingsWindow) {}

func (n *networkPage) fill(config.Config) {}

func (n *networkPage) dropped() bool { return false }

func (n *networkPage) tips() []tipItem { return nil }

func (n *networkPage) onCommand(uintptr) {}

func (s *settingsWindow) createNetwork(windows.Handle) error { return nil }

func networkBody() int { return 0 }
