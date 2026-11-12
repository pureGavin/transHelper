# transHelper

在 Teams 里用中文输入，按热键后以英文发出。翻译用自己部署的大模型，不经过第三方翻译服务。

## 状态

推送到 `main` 后，GitHub Actions 会自动构建并发布。Teams 里的端到端测试已经通过。计划和问题见 [Issues](https://github.com/pureGavin/transHelper/issues)。

## 功能

- 两个热键：`Alt+Enter` 翻译并发送，`Alt+T` 只翻译并替换输入框内容。组合可在配置里改。
- 只在白名单进程处于前台时注册热键，默认是新版和经典版 Teams。
- 模型走 OpenAI 兼容接口，地址和模型名放在配置文件里。
- 超时、报错或空文本时不粘贴、不发送，保留原文，托盘冒泡并响一声。
- 纯 Go 静态编译，一个 exe 加一个配置文件即可分发。托盘图标是紫色对话气泡和白色“译”字。

## 下载使用

1. 打开 [Releases](https://github.com/pureGavin/transHelper/releases/latest)，下载 `transHelper-YYYY.MM.x-windows-amd64.zip`。
2. 解压。
3. 双击 `transhelper.exe`。没有黑色控制台窗口，托盘里出现图标。同目录没有 `transhelper.toml` 时会自动创建，并提示填写模型地址。已有配置不会被覆盖。
4. 在 Teams 输入框里按 `Alt+T` 或 `Alt+Enter`。
5. 退出：在托盘图标上右键，选择“退出”。

正式版不在 exe 旁边写日志。需要排查时，在 cmd 里运行 `transhelper.exe 2> transhelper.log`，日志会进这个文件，只记错误、耗时和字数，不记消息内容。同一时间只能运行一个实例。

未签名程序第一次运行时，Windows 和杀毒软件可能拦截，见“SmartScreen 与杀毒软件”。

## 配置

第一次运行会在 exe 同目录创建 `transhelper.toml`。已有文件不会被覆盖。`transhelper.toml` 含有密钥，已被 Git 忽略。新建文件里的地址是占位用的 `http://localhost:8000/v1`，要改成自己的模型服务。程序写回配置时会按内置模板重写整个文件，手写的注释不会保留。

- `model.base_url`、`model.model`：必填。地址是 OpenAI 兼容接口，末尾保留 `/v1`。
- `model.api_key`：可选，不需要密钥时留空。
- `model.timeout_seconds`：默认 20，范围 1 到 120。超时后不改动输入框。
- `model.temperature`：默认 0.2，显式写 0 也会保留。
- `model.system_prompt`：留空则使用程序内置的提示词。
- `hotkeys.translate_and_send`：默认 `Alt+Enter`，翻译后替换输入框内容并回车发送。
- `hotkeys.translate_only`：默认 `Alt+T`，只替换输入框内容，不发送。
- `target.processes`：前台进程白名单，不区分大小写。默认是 `ms-teams.exe`（新版 Teams）和 `Teams.exe`（经典版）。
- `update.auto_check`：是否自动检查更新。默认 `false`。托盘菜单里的「自动检查更新」会写回这个字段。

热键至少要带 Ctrl、Alt、Win 中的一个，两个热键不能相同。配置里出现拼错的字段会直接报错。

## 检查更新

右键托盘图标，选择「检查更新」。程序只查询 GitHub Release，不会自己下载或替换 exe。需要能访问 `api.github.com`。公司网络要走代理时，先设置环境变量 `HTTPS_PROXY` 再启动。

- 点击后右下角通知「正在检查更新」。已是最新、检查失败，也会在通知里说明。
- 有新版本时，通知里带版本号。点击通知打开对应的 Release 页面。
- 「自动检查更新」默认关闭。勾选后写入配置，大约 30 秒后检查一次，之后每 24 小时一次。只有发现新版本才通知，同一次运行里同一个版本只提示一次。检查失败不影响翻译。
- `go run` 的版本是 `dev`，本机 `build.ps1` 不传版本号时末尾是 `.0`。这两种都不是正式版：手动检查会说明当前是本机构建；自动检查不会在这种构建上运行。

## 问题与计划

到 [Issues](https://github.com/pureGavin/transHelper/issues) 查看和提交。写明遇到的问题、怎样复现、期望的结果。计划和还没做的验证也写在那里。

## 发布与版本号

推送到 `main` 后，GitHub Actions 会构建并发布一个 Release。版本号是 `YYYY.MM.x`，按北京时间计算。

- `YYYY` 是年份，`MM` 是月份。
- `x` 是这一年的第几次发布。月份变化时继续累加，只有年份变化时才从 1 重新计数。例如 `2026.11.5` 的下一次是 `2026.12.6`，到了 2027 年则是 `2027.01.1`。
- 每次成功发布都会留下 tag `vYYYY.MM.x`，下一次的 `x` 就是这些 tag 里的最大值加 1。
- 在本机构建时不传 `-Version`，版本号是当前年月加 `.0`，例如 `2026.11.0`，用来和正式发布区分。
- 提交说明里写上 `[skip ci]` 时，这次推送不会触发构建，也不会产生新的 Release。适合只改文档的提交。
- Release 说明按「更新」「修复」「删除」分组，来自提交说明里以这几个词开头的行。没有这些行时发布会失败，不会发出一份看不出改了什么的 Release。说明末尾有和上一版的对比链接，再附上 exe 和 zip 的 SHA256。

## 从源码构建

日常使用直接下载 Release，不必安装 Go。下面是自己改代码时的做法。

### 开发环境

工具链放在项目内的 `.tools/`，不安装到系统目录。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\setup.ps1
. .\scripts\env.ps1
```

终端执行策略受限时，先执行 `Set-ExecutionPolicy -Scope Process Bypass`，只影响当前会话。

### 运行

开发时带上 `-config`，指向项目里的 `transhelper.toml`。没有这个文件时会创建一份默认配置。不写 `-config` 时，程序读取 exe 同目录的配置；`go run` 的 exe 在临时目录，配置也会创建在那里，而不是项目根目录。

```powershell
. .\scripts\env.ps1
go run ./cmd/transhelper -config transhelper.toml
```

开发时如果有控制台，日志会打到终端。正式版没有控制台，也不写日志文件；需要时用 `transhelper.exe 2> transhelper.log` 把 stderr 重定向到文件。

### 构建发布包

在项目根目录执行。不传版本号时，用当前北京时间的年月，末尾固定为 `.0`，例如 `2026.11.0`。

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\build.ps1 -Version 2026.11.0
```

脚本会先跑 `gofmt`、`go vet` 和 `go test`，再打进图标和版本信息，用 `-H=windowsgui` 编出没有控制台的 exe。构建使用 `-buildmode=pie` 和 `-s -w`，编完后检查 PE 头。产物：

- `dist\transhelper\transhelper.exe`，旁边还有 README
- `dist\transHelper-<版本>-windows-amd64.zip`

zip 里只有 exe 和 README。即使把带密钥的 `transhelper.toml` 放进 `dist\transhelper\`，它也不会进 zip。脚本结束时打印 exe 和 zip 的 SHA256。不用 UPX 压缩，压缩过的 exe 更容易被杀毒软件误报。

`dist\transhelper\transhelper.exe` 正在运行时脚本会中止，先从托盘退出再构建。

正式版 exe 必须带这些保护，缺一项构建就会失败：

- ASLR（`DYNAMIC_BASE`）和 64 位高熵 ASLR（`HIGH_ENTROPY_VA`），由 `-buildmode=pie` 打开。
- DEP（`NX_COMPAT`），数据页不能当代码执行。
- 没有 COFF 符号表，也没有 `.debug` 段，由 `-s -w` 去掉。

Go 链接器不支持 MSVC 的栈 canary（`/GS`）和控制流保护（`/guard:cf`）。运行时的 pclntab 会保留函数名，这是 Go 做回溯要用的，去不掉。

### 检查译文

不注册热键，直接用配置里的模型翻译样例句：

```powershell
. .\scripts\env.ps1
go run ./cmd/trytranslate -config transhelper.toml
```

样例句在 `testdata/samples.txt`，条目之间用单独一行的 `---` 分隔。

## SmartScreen 与杀毒软件

exe 没有代码签名。第一次运行时，Windows 可能提示“Windows 已保护你的电脑”。点“更多信息”，再点“仍要运行”。也可以在解压前打开 zip 的属性，勾选“解除锁定”。

杀毒软件可能把程序标成可疑。它会模拟按键、读写剪贴板，用来读取和替换 Teams 输入框里的文字，但不会安装键盘钩子，也不会改系统设置。

被拦截时，先把文件的 SHA256 和 Release 说明里的值对一下。确认一致后再让杀毒软件信任它，或者自己从源码构建。也可以向杀毒厂商提交误报。

## 已知限制

- 拼音候选框还开着时，`Alt+Enter` 不会把消息发出去，但回车会被输入法吃掉，没上屏的拼音可能粘在译文后面。`Alt+T` 只替换已经上屏的文字。程序不会先按 Esc 取消组字：没有组字时，Esc 在 Teams 里可能关掉回复框。
- Teams 把输入框里的一个空行复制出来是四个换行。写回后显示仍是一个空行。
- 大约 390 字的消息，在 Teams 里翻译大约要 14 秒。默认超时是 20 秒，更长的消息可以在配置里把 `timeout_seconds` 调大。

## 目录

- `scripts/`：安装 Go、设置当前会话环境、`build.ps1` 构建发布包、`next-version.ps1` 计算下一个版本号、`github-issues.ps1` 提交和拉取 Issues，以及 `release-notes.ps1` 生成 Release 说明。
- `.github/workflows/`：推送到 main 后构建并发布 Release。
- `.cursor/skills/`：提交和拉取 GitHub Issues 的说明。
- `cmd/transhelper/`：正式程序。加载配置、保证单实例、跑消息循环。
- `cmd/pecheck/`：检查发布 exe 的 ASLR、DEP 和符号表。
- `cmd/transhelper/winres/`：图标、清单和版本信息资源。
- `cmd/trytranslate/`：用真实模型翻译样例句。
- `internal/app/`：一次热键的流程编排，不调用 Win32。
- `internal/win/`：窗口、热键、剪贴板、托盘等全部 Win32 调用。
- `internal/hotkey/`：把热键字符串解析成修饰键和虚拟键码。
- `internal/config/`：读取和写回 TOML 配置。默认内容在 `template.toml`。
- `internal/update/`：查询 GitHub Release 并比较版本号。
- `internal/translate/`：OpenAI 兼容的翻译客户端和输出清洗。
- `testdata/samples.txt`：人工检查用的样例句。
- `.tools/`：Go 编译器和缓存，不入库。
- `AGENTS.md`：给 AI 助手的项目约定。
