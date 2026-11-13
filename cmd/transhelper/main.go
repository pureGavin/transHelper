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
	"os/exec"
	"runtime"

	"golang.org/x/sys/windows"

	"transhelper/internal/app"
	"transhelper/internal/config"
	"transhelper/internal/translate"
	"transhelper/internal/update"
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
	// 新进程用环境变量得知自己是刚更新完的。清掉它，避免以后再启动时重复提示。
	justUpdated := os.Getenv("TRANSHELPER_UPDATED") == "1"
	_ = os.Unsetenv("TRANSHELPER_UPDATED")
	if justUpdated {
		if exe, err := os.Executable(); err == nil {
			go update.RemoveOld(exe)
		}
	}

	release, already, err := acquireInstance()
	if err != nil {
		fatal(err)
	}
	if already {
		messageBox("transHelper 已在运行。")
		return
	}
	var closed bool
	closeInstance := func() {
		if closed {
			return
		}
		closed = true
		release()
	}
	defer closeInstance()

	restart, err := win.Run(context.Background(), win.Options{
		ConfigPath:    path,
		Config:        cfg,
		Version:       version,
		FirstRun:      firstRun,
		Updated:       justUpdated,
		Logger:        logger,
		NewTranslator: newTranslator,
	})
	if err != nil {
		logger.Printf("退出: %v", err)
		fatal(err)
	}
	if !restart {
		return
	}
	// 新进程要拿到单实例锁，所以先放开，再启动替换后的 exe。
	closeInstance()
	if err := relaunch(path); err != nil {
		logger.Printf("启动新版本失败: %v", err)
		if exe, exeErr := os.Executable(); exeErr == nil {
			if rerr := update.Restore(exe); rerr != nil {
				logger.Printf("恢复旧版本失败: %v", rerr)
				messageBox("新版本没有启动，恢复旧版本也失败。请从发布页重新下载。")
				os.Exit(1)
			}
		}
		messageBox("新版本没有启动，已恢复旧版本，请重新打开")
		os.Exit(1)
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
		HTTP:        cfg.Proxy.ModelClient(),
	}
}

// relaunch 启动已经替换好的 exe。环境变量告诉新进程这次是更新完成。
func relaunch(configPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "-config", configPath)
	cmd.Env = append(os.Environ(), "TRANSHELPER_UPDATED=1")
	return cmd.Start()
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
