//go:build windows && uidrive

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"transhelper/internal/win"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(command string, args []string) error {
	switch command {
	case "help", "-h", "--help":
		usage()
		return nil
	case "start":
		if len(args) != 0 {
			return fmt.Errorf("start 不接受参数")
		}
		return cmdStart()
	case "attach":
		if len(args) != 1 {
			return fmt.Errorf("用法: uidrive attach <pid>")
		}
		pid, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("pid 无效")
		}
		return cmdAttach(pid)
	case "menu":
		if len(args) != 1 {
			return fmt.Errorf("用法: uidrive menu settings|check-update|auto-check")
		}
		return withApp(func(app *win.App) error { return app.Menu(args[0]) })
	case "state":
		if len(args) != 0 {
			return fmt.Errorf("state 不接受参数")
		}
		return withApp(func(app *win.App) error {
			state, err := app.State()
			if err != nil {
				return err
			}
			return printJSON(state)
		})
	case "set":
		if len(args) < 2 {
			return fmt.Errorf("用法: uidrive set <控件> <文字>")
		}
		text := strings.Join(args[1:], " ")
		return withApp(func(app *win.App) error { return app.Set(args[0], text) })
	case "click":
		if len(args) != 1 {
			return fmt.Errorf("用法: uidrive click <控件>")
		}
		return withApp(func(app *win.App) error { return app.Click(args[0]) })
	case "select":
		if len(args) != 2 {
			return fmt.Errorf("用法: uidrive select theme system|light|dark  或  select proxyType http|socks5")
		}
		return withApp(func(app *win.App) error { return app.Select(args[0], args[1]) })
	case "pick":
		if len(args) != 2 {
			return fmt.Errorf("用法: uidrive pick procApps|procEnabled <进程名>")
		}
		return withApp(func(app *win.App) error { return app.Pick(args[0], args[1]) })
	case "tab":
		if len(args) != 1 {
			return fmt.Errorf("用法: uidrive tab general|processes|network|about")
		}
		return withApp(func(app *win.App) error { return app.Tab(args[0]) })
	case "key":
		if len(args) != 1 {
			return fmt.Errorf("用法: uidrive key esc|enter")
		}
		return withApp(func(app *win.App) error { return app.Key(args[0]) })
	case "dialog":
		if len(args) == 0 {
			return withApp(func(app *win.App) error {
				dialog, err := app.Dialog()
				if err != nil {
					return err
				}
				return printJSON(dialog)
			})
		}
		if len(args) == 1 && args[0] == "ok" {
			return withApp(func(app *win.App) error { return app.DialogOK() })
		}
		return fmt.Errorf("用法: uidrive dialog [ok]")
	case "balloon-click":
		if len(args) != 0 {
			return fmt.Errorf("balloon-click 不接受参数")
		}
		return withApp(func(app *win.App) error { return app.BalloonClick() })
	case "resize":
		height := 1
		if len(args) == 1 {
			n, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("高度必须是整数")
			}
			height = n
		} else if len(args) != 0 {
			return fmt.Errorf("用法: uidrive resize [高度]")
		}
		return withApp(func(app *win.App) error { return app.Resize(height) })
	case "wheel":
		if len(args) != 1 {
			return fmt.Errorf("用法: uidrive wheel <格数>，正数向下")
		}
		n, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("格数必须是整数")
		}
		return withApp(func(app *win.App) error { return app.Wheel(n) })
	case "shot":
		if len(args) != 1 {
			return fmt.Errorf("用法: uidrive shot <png>")
		}
		return withApp(func(app *win.App) error { return app.Shot(args[0]) })
	case "log":
		if len(args) != 0 {
			return fmt.Errorf("log 不接受参数")
		}
		return withApp(func(app *win.App) error {
			text, err := app.Log()
			if err != nil {
				return err
			}
			fmt.Print(text)
			return nil
		})
	case "close":
		if len(args) != 0 {
			return fmt.Errorf("close 不接受参数")
		}
		return withApp(func(app *win.App) error { return app.Close() })
	case "stop":
		if len(args) != 0 {
			return fmt.Errorf("stop 不接受参数")
		}
		return withApp(func(app *win.App) error { return app.Stop() })
	default:
		usage()
		return fmt.Errorf("不认识的命令 %s", command)
	}
}

func cmdStart() error {
	root, err := win.ModuleRoot()
	if err != nil {
		return err
	}
	app, err := win.Start()
	if err != nil {
		return err
	}
	if err := win.SaveSession(root, app); err != nil {
		_ = app.Stop()
		return err
	}
	return printJSON(map[string]any{
		"pid":    app.PID,
		"config": app.ConfigPath,
		"log":    app.LogPath,
	})
}

func cmdAttach(pid int) error {
	root, err := win.ModuleRoot()
	if err != nil {
		return err
	}
	app, err := win.Attach(pid)
	if err != nil {
		return err
	}
	if err := win.SaveSession(root, app); err != nil {
		return err
	}
	return printJSON(map[string]any{
		"pid":    app.PID,
		"config": app.ConfigPath,
		"log":    app.LogPath,
	})
}

func withApp(fn func(*win.App) error) error {
	root, err := win.ModuleRoot()
	if err != nil {
		return err
	}
	app, err := win.OpenSession(root)
	if err != nil {
		return err
	}
	if err := fn(app); err != nil {
		return err
	}
	switch os.Args[1] {
	case "state", "dialog", "log", "start", "attach":
		return nil
	default:
		return printJSON(map[string]bool{"ok": true})
	}
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func usage() {
	fmt.Fprint(os.Stderr, `用法: uidrive <命令>

  start                          启动一个测试实例
  attach <pid>                   接到已经在运行的 transHelper
  menu settings|check-update|auto-check
  state                          打印设置窗口状态
  set <控件> <文字>
  click <控件>
  select theme system|light|dark
  select proxyType http|socks5
  pick procApps|procEnabled <进程名>
  tab general|processes|network|about
  key esc|enter
  dialog [ok]
  balloon-click
  resize [高度]
  wheel <格数>                   正数向下
  shot <png>
  log
  close                          关掉设置窗口，不保存
  stop                           退出测试实例

控件: apply cancel url model key showKey timeout theme tabs link
      proxyOn proxyType proxyHost proxyPort proxyAuth proxyUser proxyPass proxyModel
      procApps procEnabled procAdd procRemove

密码框只报告长度。先在项目根目录执行 . .\scripts\env.ps1。
`)
}
