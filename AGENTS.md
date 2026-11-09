# transHelper 项目约定

## 项目是什么

- Windows 本地小工具：在 Teams 输入框里按热键，用本地大模型把中文译成英文再发送。
- 纯 Go，`CGO_ENABLED=0`，最终是一个 exe 加一个配置文件，下载即用。
- 两个热键：翻译并发送；只翻译并替换输入框内容、不发送。热键组合可配置。
- 只在 Teams 窗口生效，进程白名单可配置。
- 模型走 OpenAI 兼容接口；地址、模型名、热键、进程白名单都放配置文件。
- 超时、报错、空文本时什么都不发送，保留原文并提示。

## 工具链与环境（最重要）

- 所有第三方下载物只放项目内的 `.tools/`：Go 编译器、模块缓存、构建缓存、临时文件。
- 不得安装到 C 盘或系统目录。不用 MSI、winget、`setx`、`go env -w`，不改全局 Git 配置。
- 终端里运行任何 Go 命令前，先在项目根目录执行 `. .\scripts\env.ps1`。它只影响当前会话，可重复执行。
- `.tools/` 丢失或刚克隆仓库后，运行 `powershell -NoProfile -ExecutionPolicy Bypass -File scripts\setup.ps1`。
- 不 vendor 依赖，版本由 `go.mod` 和 `go.sum` 锁定。

## Git

- 只做本地提交，不配置远端。提交身份已写在本仓库的本地配置里，不要改全局。
- 提交用显式路径 `git add <路径>`，不用 `git add -A`。每个里程碑提交一次，方便 `git revert` 或 `git reset` 回滚。
- `.gitignore` 是黑名单：出现新的杂项文件，先补忽略规则，再提交。

## 文件约定

- `scripts/*.ps1` 只用 ASCII，Windows PowerShell 5.1 读无 BOM 的 UTF-8 会乱码。
- 全仓库行尾统一 LF，见 `.gitattributes`。
- 构建产物输出到 `dist/`，该目录已被忽略。
