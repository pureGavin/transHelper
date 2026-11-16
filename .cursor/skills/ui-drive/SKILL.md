---
name: ui-drive
description: >
  用窗口消息驱动 transHelper 的托盘和设置窗口。
  验证设置项、托盘菜单、通知文字、弹窗或窗口滚动时使用。
  不要用 Computer Use 去点这些控件。
---

# 驱动设置窗口

托盘菜单和设置窗口按控件 ID 发消息，不靠截图坐标。正式版 exe 没有这套入口以外的测试代码；`uidrive` 构建标签只用于测试。

先加载工具链。终端里还没有执行过时：

```powershell
. .\scripts\env.ps1
```

## 何时用

- 改了设置窗口、托盘菜单、通知或配置写回，要看真窗口时，跑端到端用例：

```powershell
go test -tags uidrive ./internal/win -run E2E -count=1 -v
```

- 要自己逐步操作时，用 `scripts\uidrive.ps1`。它会编译驱动，再转发参数。
- 颜色、自绘、滚动条外观用 `shot` 截设置窗口，然后看图。不要为了看界面去点屏幕。

Computer Use 的坐标点击不用在托盘图标、设置窗口和它的弹窗上。

## 命令

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\uidrive.ps1 start
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\uidrive.ps1 menu settings
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\uidrive.ps1 state
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\uidrive.ps1 stop
```

| 命令 | 作用 |
| --- | --- |
| `start` | 启动测试实例。锁名和正在使用的 transHelper 分开 |
| `attach <pid>` | 接到已经在运行的进程。配置按 exe 同目录找 |
| `menu settings` | 打开设置。还有 `check-update`、`auto-check` |
| `state` | 打印窗口和每个控件的 JSON |
| `set <控件> <文字>` | 写输入框 |
| `click <控件>` | 点按钮或复选框 |
| `select theme dark` | 选下拉项。`theme` 是 `system`、`light`、`dark`；`proxyType` 是 `http`、`socks5` |
| `tab network` | 切到 `general`、`network` 或 `about` |
| `key esc` | `esc` 关闭不保存，`enter` 在非按钮上等于应用 |
| `dialog` | 读提示框文字。`dialog ok` 点确定 |
| `close` | 关闭设置窗口，不保存 |
| `resize 200` | 改高度。省略高度时缩到最小 |
| `wheel 4` | 滚动。正数向下 |
| `shot a.png` | 只截设置窗口 |
| `balloon-click` | 点当前托盘通知 |
| `log` | 读测试实例的日志 |
| `stop` | 退出测试实例 |

控件名：`apply`、`cancel`、`url`、`model`、`key`、`showKey`、`timeout`、`theme`、`tabs`、`link`、`proxyOn`、`proxyType`、`proxyHost`、`proxyPort`、`proxyAuth`、`proxyUser`、`proxyPass`、`proxyModel`。

`state` 里密码框没有正文。Windows 会拦住别的进程读取密码框，所以 `textLen` 也经常是 0，不能靠它判断写进去没有。勾选「显示」只把 `passwordChar` 变成 0。要确认密钥，点「应用」后读测试配置，不要把密钥打到对话里。

会话记在 `.tools/tmp/uidrive/session.json`。`start` 之后的命令都作用在这个进程上。

通知文字写在日志里，形如 `通知：应用设置成功`。日志不记要翻译的消息。

## 做不到的事

- 不能靠投递消息模拟 Ctrl+Tab。切换标签用 `tab`。
- 不能跨进程发送 `WM_NOTIFY`，系统不会拷贝里面的指针。
- `shot` 只负责截图，不能从图片反推该点哪里。
