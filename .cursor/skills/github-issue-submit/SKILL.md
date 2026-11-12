---
name: github-issue-submit
description: >
  把 transHelper 的开发计划和发现的问题提交为 GitHub Issue。
  用户汇报缺陷、提出计划、要求记一个 issue，或需要把待办留档时使用。
  正文必须写清问题、复现步骤和期望结果，不上传测试文件。
---

# 提交 GitHub Issue

计划和问题都提交到 `pureGavin/transHelper` 的 Issues，不写 `TODO.md`。

## 凭据

token 只放在被忽略的 `.tools/secrets/github-token`。没有这个文件时，请用户在自己的终端运行：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/github-issues.ps1 save-token
```

新建或更换 token：打开 GitHub → Settings → Developer settings → Fine-grained tokens，Resource owner 选 `pureGavin`，仓库只选 `transHelper`，权限只给 Issues 的 Read and write。不要把 token 打印出来，不要写进仓库、提交说明或 issue。

## 步骤

1. 先查重：`powershell -NoProfile -ExecutionPolicy Bypass -File scripts/github-issues.ps1 list`
2. 按下面的模板把正文写到 `.tools/tmp/issue-body.md`。这一步只是本地草稿。
3. 创建：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/github-issues.ps1 create -Title "标题" -BodyFile .tools/tmp/issue-body.md -Label bug
```

4. 把脚本打印的编号和链接告诉用户。

缺陷用标签 `bug`。计划、还没做的验证用标签 `enhancement`。

## 正文模板

```markdown
## 问题

遇到了什么。如果是计划，写要做什么、为什么做。

## 复现步骤

1. 第一步
2. 第二步

计划则写现在怎样能看到这个缺口。样例输入和输出用文字或短代码块写在步骤里。

## 期望结果

完成后可以怎样验收。

## 环境

可选。软件版本、Windows 版本、Teams 版本。
```

`## 问题`、`## 复现步骤`、`## 期望结果` 都必须有，而且不能空着。脚本会拒绝缺段、token、密钥值和内网地址。

不上传文件。确实需要附件时，issue 建好后告诉用户：到网页上把文件拖进评论框。

不写密钥、内网地址，也不要抄聊天里的 token。
