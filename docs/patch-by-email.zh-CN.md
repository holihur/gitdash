# 邮件提交补丁（patch by email）

gitdash 可以把邮件里的补丁系列转成 pull request —— 也就是 `git send-email` /
`git format-patch` 工作流。贡献者产出一份 mbox，gitdash 把它应用到新分支并开 PR。

## 用法

```sh
git format-patch -1 --stdout | \
  curl -X POST "https://gitdash.example.com/api/users/acme/repos/web/patches" \
       -H "Authorization: Bearer $GITDASH_PAT" \
       -H "Content-Type: text/plain" --data-binary @-
```

服务端在临时克隆里对目标分支执行 `git am --3way`，push 一个新分支
`patches/<时间戳>`，并创建一个普通 PR，目标为仓库默认分支（可用 `?target=<branch>`
覆盖）。PR 标题取第一封补丁的 `Subject:`（可用 `?title=` 覆盖），正文列出该系列
每封补丁的主题。

## API

| | |
| --- | --- |
| 方法 | `POST` |
| 路径 | `/api/users/{owner}/repos/{name}/patches` |
| 鉴权 | 对仓库有写权限的 PAT / 登录会话 |
| 请求体 | mbox（`text/plain`，最大 20 MiB） |
| Query | `target`（默认仓库默认分支）、`title` |

响应：

- `201`：返回创建的 `PullRequest`。
- `400 empty_patch`：请求体为空。
- `400 patch_failed`：`git am` 无法应用（基线错误、冲突等）。
- `401`：未认证。
- `404`：无写权限 / 仓库不存在。

## 说明

- 一个 mbox 里拼接的多封补丁会按顺序应用；`git am` 会保留补丁里的作者信息
  （提交者为 `gitdash` 服务身份）。
- 应用失败会 `git am --abort`，不会残留分支。
- 生成的 PR 与网页端创建的 PR 完全一致，因此分支保护、CI、合并门禁都会生效。
- 第一封补丁的提交元数据（作者姓名/邮箱）会被保留。

## 示例

```sh
# 本地创建两个提交，作为同一系列提交
git format-patch -2 --stdout > series.mbox
curl -X POST "https://gitdash.example.com/api/users/acme/repos/web/patches?title=Fix+widgets" \
     -H "Authorization: Bearer $GITDASH_PAT" \
     -H "Content-Type: text/plain" --data-binary @series.mbox
```
