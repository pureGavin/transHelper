//go:build windows && uidrive

package main

import (
	"fmt"
	"os"
	"strings"
)

// instanceName 在测试构建里允许换一把锁，这样用户正在用的 transHelper 不会挡住测试实例。
// TRANSHELPER_INSTANCE 只是锁名，会放在 Local\ 下面。正式构建没有这个文件。
func instanceName() (string, error) {
	name := strings.TrimSpace(os.Getenv("TRANSHELPER_INSTANCE"))
	if name == "" {
		return `Local\transHelper`, nil
	}
	if strings.ContainsAny(name, `\/`) {
		return "", fmt.Errorf("TRANSHELPER_INSTANCE 只能是锁名，不要写路径")
	}
	return `Local\` + name, nil
}
