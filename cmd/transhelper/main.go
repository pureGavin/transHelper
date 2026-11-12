//go:build windows

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/sys/windows"

	"transhelper/internal/config"
	"transhelper/internal/translate"
	"transhelper/internal/win"
)

func main() {
	// 窗口、热键和剪贴板都绑在创建它们的线程上。
	runtime.LockOSThread()

	configPath := flag.String("config", "", "配置文件路径，默认是 exe 同目录的 transhelper.toml")
	flag.Parse()

	path := *configPath
	if path == "" {
		exe, err := os.Executable()
		if err != nil {
			fatal(err)
		}
		path = config.DefaultPath(exe)
	}
	cfg, err := config.Load(path)
	if err != nil {
		fatal(err)
	}

	logger := newLogger()
	logger.Printf("已加载配置 %s，模型 %s", path, cfg.Model)

	release, already, err := acquireInstance()
	if err != nil {
		fatal(err)
	}
	if already {
		messageBox("transHelper 已在运行。")
		return
	}
	defer release()

	client := &translate.Client{
		BaseURL:     cfg.BaseURL,
		APIKey:      cfg.APIKey,
		Model:       cfg.Model,
		Temperature: cfg.Temperature,
		Prompt:      cfg.SystemPrompt,
	}
	if err := win.Run(context.Background(), cfg, client, logger); err != nil {
		logger.Printf("退出: %v", err)
		fatal(err)
	}
}

func newLogger() *log.Logger {
	writers := []io.Writer{os.Stderr}
	exe, err := os.Executable()
	if err == nil {
		path := filepath.Join(filepath.Dir(exe), "transhelper.log")
		file, openErr := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if openErr == nil {
			writers = append(writers, file)
		}
	}
	return log.New(io.MultiWriter(writers...), "", log.LstdFlags)
}

var instanceMutex windows.Handle

func acquireInstance() (func(), bool, error) {
	name, err := windows.UTF16FromString(`Local\transHelper`)
	if err != nil {
		return nil, false, err
	}
	handle, err := windows.CreateMutex(nil, false, &name[0])
	runtime.KeepAlive(name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
		return nil, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	instanceMutex = handle
	return func() {
		if instanceMutex != 0 {
			windows.CloseHandle(instanceMutex)
			instanceMutex = 0
		}
	}, false, nil
}

func messageBox(text string) {
	body, err := windows.UTF16FromString(text)
	if err != nil {
		fmt.Fprintln(os.Stderr, text)
		return
	}
	title, err := windows.UTF16FromString("transHelper")
	if err != nil {
		fmt.Fprintln(os.Stderr, text)
		return
	}
	windows.MessageBox(0, &body[0], &title[0], windows.MB_OK|windows.MB_ICONINFORMATION)
	runtime.KeepAlive(body)
	runtime.KeepAlive(title)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	messageBox(err.Error())
	os.Exit(1)
}
