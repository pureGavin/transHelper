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

## 目录

- `scripts/`：安装 Go 和设置当前会话环境的脚本。
- `.tools/`：Go 编译器和缓存，不入库。
- `AGENTS.md`：给 AI 助手的项目约定。
- `TODO.md`：计划和测试方法。
