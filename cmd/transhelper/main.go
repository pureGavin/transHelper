//go:build windows

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"runtime"

	"golang.org/x/sys/windows"

	"transhelper/internal/app"
	"transhelper/internal/config"
	"transhelper/internal/translate"
	"transhelper/internal/win"
)

// version 由构建脚本用 -X 写入。开发时保持 dev。
var version = "dev"

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
	cfg, firstRun, err := loadOrCreate(path)
	if err != nil {
		fatal(err)
	}

	logger := newLogger()
	logger.Printf("transHelper %s 已加载配置 %s，模型 %s", version, path, cfg.Model)

	release, already, err := acquireInstance()
	if err != nil {
		fatal(err)
	}
	if already {
		messageBox("transHelper 已在运行。")
		return
	}
	defer release()

	if err := win.Run(context.Background(), win.Options{
		ConfigPath:    path,
		Config:        cfg,
		Version:       version,
		FirstRun:      firstRun,
		Logger:        logger,
		NewTranslator: newTranslator,
	}); err != nil {
		logger.Printf("退出: %v", err)
		fatal(err)
	}
}

// loadOrCreate 读取配置。文件不存在时先创建默认配置，已有文件不会被覆盖。
func loadOrCreate(path string) (config.Config, bool, error) {
	cfg, err := config.Load(path)
	if err == nil {
		return cfg, false, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return config.Config{}, false, err
	}
	firstRun := false
	if createErr := config.CreateDefault(path); createErr != nil {
		if !errors.Is(createErr, fs.ErrExist) {
			return config.Config{}, false, createErr
		}
	} else {
		firstRun = true
	}
	cfg, err = config.Load(path)
	if err != nil {
		return config.Config{}, false, err
	}
	return cfg, firstRun, nil
}

func newTranslator(cfg config.Config) app.Translator {
	return &translate.Client{
		BaseURL:     cfg.BaseURL,
		APIKey:      cfg.APIKey,
		Model:       cfg.Model,
		Temperature: cfg.Temperature,
		Prompt:      cfg.SystemPrompt,
	}
}

func newLogger() *log.Logger {
	// 正式版是窗口程序，没有控制台，也不在 exe 旁边写日志文件。
	// 从 cmd 用 2> 重定向时，这个句柄仍然可用，日志会进那个文件。
	// go run 时 stderr 是终端，日志照常打出来。
	if _, err := os.Stderr.Stat(); err == nil {
		return log.New(os.Stderr, "", log.LstdFlags)
	}
	return log.New(io.Discard, "", 0)
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
