# transHelper

在 Teams 里用中文输入，按热键后以英文发出。翻译用自己部署的大模型，不经过第三方翻译服务。

## 状态

开发中。进度和测试方法见 [TODO.md](TODO.md)。

## 计划功能

- 两个热键：一个翻译并发送，一个只翻译并替换输入框内容。
- 只在 Teams 窗口生效，进程白名单可配置。
- 模型走 OpenAI 兼容接口，地址和模型名放在配置文件里。
- 超时、报错或空文本时不发送，保留原文并提示。
- 纯 Go 静态编译，一个 exe 加一个配置文件即可分发。

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

## 目录

- `scripts/`：安装 Go 和设置当前会话环境的脚本。
- `internal/hotkey/`：把热键字符串解析成修饰键和虚拟键码。
- `internal/config/`：读取 TOML 配置、填默认值并校验。
- `transhelper.example.toml`：配置示例。
- `.tools/`：Go 编译器和缓存，不入库。
- `AGENTS.md`：给 AI 助手的项目约定。
- `TODO.md`：计划和测试方法。
