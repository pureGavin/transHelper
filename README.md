# transHelper

在 Teams 里用中文输入，按热键后以英文发出。翻译用自己部署的大模型，不经过第三方翻译服务。

## 状态

开发中。热键、剪贴板和托盘已经接上，Teams 里的端到端测试还没做。进度见 [TODO.md](TODO.md)。

## 功能

- 两个热键：`Alt+Enter` 翻译并发送，`Alt+T` 只翻译并替换输入框内容。组合可在配置里改。
- 只在白名单进程处于前台时注册热键，默认是新版和经典版 Teams。
- 模型走 OpenAI 兼容接口，地址和模型名放在配置文件里。
- 超时、报错或空文本时不粘贴、不发送，保留原文，托盘冒泡并响一声。
- 纯 Go 静态编译，一个 exe 加一个配置文件即可分发。托盘图标目前用系统自带图标。

## 运行

先准备好 `transhelper.toml`，然后：

```powershell
. .\scripts\env.ps1
go run ./cmd/transhelper -config transhelper.toml
```

不写 `-config` 时，程序读取 exe 同目录的 `transhelper.toml`。正式打包是阶段 4 的事，开发时用上面的命令即可。

程序没有主窗口，任务栏托盘里有图标，右键选择“退出”。同一时间只能运行一个实例。日志写在 exe 旁边的 `transhelper.log`，开发时也会打到终端。日志只记错误、耗时和字数，不记消息内容。

## 开发环境

工具链放在项目内的 `.tools/`，不安装到系统目录。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\setup.ps1
. .\scripts\env.ps1
```

终端执行策略受限时，先执行 `Set-ExecutionPolicy -Scope Process Bypass`，只影响当前会话。

## 配置

复制 [transhelper.example.toml](transhelper.example.toml) 为 `transhelper.toml`，按自己的环境修改。`transhelper.toml` 含有密钥，已被 Git 忽略。

- `model.base_url`、`model.model`：必填。地址是 OpenAI 兼容接口，末尾保留 `/v1`。
- `model.api_key`：可选，不需要密钥时留空。
- `model.timeout_seconds`：默认 20，范围 1 到 120。超时后不改动输入框。
- `model.temperature`：默认 0.2，显式写 0 也会保留。
- `model.system_prompt`：留空则使用程序内置的提示词。
- `hotkeys.translate_and_send`：默认 `Alt+Enter`，翻译后替换输入框内容并回车发送。
- `hotkeys.translate_only`：默认 `Alt+T`，只替换输入框内容，不发送。
- `target.processes`：前台进程白名单，不区分大小写。默认是 `ms-teams.exe`（新版 Teams）和 `Teams.exe`（经典版）。

热键至少要带 Ctrl、Alt、Win 中的一个，两个热键不能相同。配置里出现拼错的字段会直接报错。

## 检查译文

不注册热键，直接用配置里的模型翻译样例句：

```powershell
. .\scripts\env.ps1
go run ./cmd/trytranslate -config transhelper.toml
```

样例句在 `testdata/samples.txt`，条目之间用单独一行的 `---` 分隔。

## 目录

- `scripts/`：安装 Go 和设置当前会话环境的脚本。
- `cmd/transhelper/`：正式程序。加载配置、保证单实例、写日志、跑消息循环。
- `cmd/trytranslate/`：用真实模型翻译样例句。
- `internal/app/`：一次热键的流程编排，不调用 Win32。
- `internal/win/`：窗口、热键、剪贴板、托盘等全部 Win32 调用。
- `internal/hotkey/`：把热键字符串解析成修饰键和虚拟键码。
- `internal/config/`：读取 TOML 配置、填默认值并校验。
- `internal/translate/`：OpenAI 兼容的翻译客户端和输出清洗。
- `testdata/samples.txt`：人工检查用的样例句。
- `transhelper.example.toml`：配置示例。
- `.tools/`：Go 编译器和缓存，不入库。
- `AGENTS.md`：给 AI 助手的项目约定。
- `TODO.md`：计划和测试方法。
