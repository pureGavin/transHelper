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
- `go-winres` 由 `go.mod` 的 `tool` 指令锁定，用 `go tool go-winres` 调用，不安装到系统目录。
- 发布包用 `scripts\build.ps1` 构建，产物在 `dist/`。构建使用 `-buildmode=pie` 和 `-s -w`，编完后用 `cmd/pecheck` 检查 ASLR、高熵 ASLR、DEP 和空符号表。推送到 main 后，GitHub Actions 用同一套脚本构建正式 Release，不把 `.tools/` 或 `dist/` 提交上去。

## 代码与注释

- 代码注释用中文。导出的类型和函数写中文文档注释，按 Go 惯例以标识符名开头，例如 `// Translate 把中文译成英文`。
- Win32 调用、按键时序、边界情况等看代码看不出原因的地方，用中文注释写明原因。
- Win32 调用只放在 `internal/win`。热键解析、配置、翻译和流程编排都不引用它，方便单测。
- 模型密钥只写在被 Git 忽略的 `transhelper.toml`。内置模板 `internal/config/template.toml` 的 `api_key` 留空，地址用 `http://localhost:8000/v1` 这种占位。
- 示例配置和测试里不写内网地址，用 `http://localhost:8000/v1` 这样的占位地址。
- `.ps1` 用带 BOM 的 UTF-8 保存：Windows PowerShell 5.1 读无 BOM 的文件时按系统代码页解码，中文会乱码。Go 源码用无 BOM 的 UTF-8。
- 全仓库行尾统一 LF，见 `.gitattributes`。构建产物输出到 `dist/`，该目录已被忽略。

## 目录

- `scripts/`：安装 Go、设置当前会话环境、构建发布包、计算下一个版本号、提交和拉取 GitHub Issues，以及生成 Release 说明。
- `.github/workflows/`：推送到 main 后构建并发布 Release。
- `.cursor/skills/`：提交和拉取 GitHub Issues 的说明。
- `cmd/transhelper/`：正式程序入口。
- `cmd/pecheck/`：检查发布 exe 的 ASLR、DEP 和符号表。
- `cmd/transhelper/winres/`：图标、清单和版本信息资源。
- `cmd/trytranslate/`：用真实模型翻译样例句。
- `internal/config/`：读取和写回 TOML 配置。默认内容在 `template.toml`。
- `internal/hotkey/`：热键字符串解析。
- `internal/translate/`：翻译客户端。
- `internal/app/`：一次热键的流程编排。
- `internal/win/`：全部 Win32 调用。
- `.tools/`：Go 编译器和缓存，不入库。

## 计划与问题

- 计划和问题都提交为 GitHub Issues，不再写 `TODO.md`。
- 动手前先看 [.cursor/skills/github-issue-pull/SKILL.md](.cursor/skills/github-issue-pull/SKILL.md)，避免和现有 issue 重复。新的按 [.cursor/skills/github-issue-submit/SKILL.md](.cursor/skills/github-issue-submit/SKILL.md) 提交。
- 正文必须有「问题」「复现步骤」「期望结果」，而且不能空着。
- issue 是公开的，不写密钥、token 和内网地址，也不上传测试文件。
- token 只放在被忽略的 `.tools/secrets/github-token`。用 `scripts\github-issues.ps1 save-token` 写入，不要贴到对话或仓库里。

## 文档同步

每次修改项目，在同一次提交里更新受影响的文档：

- `README.md`：功能、用法、配置项、构建方式有变化时。
- `AGENTS.md`：约定、目录结构、工具链有变化时。

## Git

- 远端 `origin` 是 `git@github.com:pureGavin/transHelper.git`。提交身份写在本仓库的本地配置里，邮箱是 `puregavin@foxmail.com`，不要改全局 Git 配置。
- 做完某个 issue 时，提交说明末尾写 `Closes #N`。推送到 `main` 后 GitHub 会关掉它。
- 提交说明末尾还要写用户能感受到的变化，每行一条，以 `更新：`、`修复：` 或 `删除：` 开头。中英文冒号都可以。没有这些行时，这次推送的 Release 会失败。
- 每完成一次提交，都用 AskQuestion 询问是否推送到 GitHub，并说明推送 `main` 会触发一次 Release。没有得到同意就不推送，也不强推。
- 提交用显式路径 `git add <路径>`，不用 `git add -A`。每个里程碑提交一次，方便 `git revert` 或 `git reset` 回滚。
- `.gitignore` 是黑名单：出现新的杂项文件，先补忽略规则，再提交。
- 提交信息用中文：第一行概括改了什么，空一行后写清楚为什么改，包括动机、要解决的问题、取舍。例如：

  ```
  调整忽略规则：.cursor 下只跟踪 rules 和 skills

  整个 .cursor/ 被忽略时，Cursor 查找 rules 也会跳过它，规则可能不生效。
  现在只放行 rules 和 skills，调试日志等仍留在本地。

  更新：.cursor 下的 rules 和 skills 会进入仓库
  ```

- `.cursor/` 下只有 `rules/` 和 `skills/` 入库，调试日志、plans 等留在本地。
