---
name: github-issue-pull
description: >
  拉取 transHelper 仓库的 GitHub Issues。
  开始做功能或修缺陷之前、用户询问有哪些 issue 或计划、或提到某个 issue 编号时使用。
---

# 拉取 GitHub Issue

计划和问题以 `pureGavin/transHelper` 的 Issues 为准，不读 `TODO.md`。

## 凭据

公开 issue 不需要 token。有 `.tools/secrets/github-token` 时脚本会带上。不要打印 token。

## 步骤

看未关闭的概况：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/github-issues.ps1 list
```

按状态或标签过滤：`-State all`、`-State closed`、`-Label bug`、`-Label enhancement`。

读一个 issue 的正文和全部评论：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/github-issues.ps1 get -Number 1
```

动手前先看有没有已经在做的 issue，避免重复建。

## 做完之后

提交说明里引用对应编号。做完时在说明末尾写 `Closes #N`。推送到 `main` 后，GitHub 会关掉这个 issue。
