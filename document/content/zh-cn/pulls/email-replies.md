---
title: "邮件回复"
weight: 4
summary: "通过回复通知邮件来评论 issue / PR。"
---

Gitdash 可以把对通知邮件的回复转成对应 issue / PR 上的评论。该功能是**无状态**的：
路由信息编码在签名的 `Reply-To` 地址里，服务端不需要保存 token 表。

## 工作方式

1. 设置 `GITDASH_MAIL_REPLY_DOMAIN` 且配置了邮件密钥后，每封通知邮件会多出两个头：

   ```
   Message-ID: <gitdash.pull.acme.web.42.9f1c...@mail.example.com>
   Reply-To:   reply+<payload>.<sig>@mail.example.com
   ```

   token **按收件人**生成，因此谁回复就以谁的身份发言。

2. 收件人回复邮件。邮件服务商（或 MTA 管道）把解析后的邮件 POST 给 Gitdash：

   ```
   POST /api/mail/inbound
   X-Gitdash-Mail-Secret: <共享密钥>
   Content-Type: application/json

   {"to": "reply+<payload>.<sig>@mail.example.com",
    "from": "alice@example.com",
    "subject": "Re: [acme/web#42] Fix the thing",
    "text": "Looks good to me!\n\n> 引用旧内容\n-- \nAlice"}
   ```

3. Gitdash 验签、清洗正文（去掉 `>` 引用、`-- ` 签名、`On ... wrote:` 引导行），
   以 token 绑定用户的身份创建评论，并照常触发收件箱 / webhook 通知。

## 线程化与幂等

每条评论在创建时都会分配一个 `Message-ID`，该评论的通知邮件复用同一值，
因此回复邮件在任何邮件客户端都会归入原通知线程。MTA 适配层应回传回复邮件
自身的 `Message-ID` 及其 `In-Reply-To` / `References`：

```json
{"to": "reply+<payload>.<sig>@mail.example.com",
 "from": "alice@example.com",
 "subject": "Re: [acme/web#42] Fix the thing",
 "text": "Looks good to me!",
 "message_id": "<reply-123@mail.example.com>",
 "in_reply_to": "<gitdash.issue.acme.web.42.9f1c@mail.example.com>",
 "references": "<gitdash...@mail.example.com>"}
```

Gitdash 会把两者存到评论上（`message_id` / `in_reply_to`，评论 API 可见），
并以 `message_id` 作为幂等键：同一封邮件重复投递会返回已有评论（`200`）
而不重复落库。`in_reply_to` 缺失时取 `references` 的最后一个。

## token 格式

- `payload = base64url(owner|repo|kind|number|username|exp)`，无填充。
- `sig = base64url(HMAC-SHA256(secret, payload)[:10])`，无填充。
- `exp` 为 Unix 时间戳，token 有效期 90 天。
- `kind` 取 `issue` 或 `pull`。

只有持有密钥的一方能签发 token，地址也只发给目标收件人。与任何邮件流程一样，
转发邮件即转发了评论能力。

## 配置

| 环境变量 | 作用 |
| --- | --- |
| `GITDASH_MAIL_REPLY_DOMAIN` | `Reply-To` / `Message-ID` 使用的域名；不设即关闭该功能。 |
| `GITDASH_MAIL_SECRET` | 回复 token 的 HMAC 密钥；回退到 `GITDASH_SECRET_KEY`。为空则不生成 `Reply-To`。 |
| `GITDASH_MAIL_INBOUND_SECRET` | `POST /api/mail/inbound` 要求的共享密钥；回退到 `GITDASH_MAIL_SECRET`。为空则端点禁用（401）。 |

入站端点**不**经过用户登录鉴权，设计上只允许邮件服务商 / MTA 管道调用。
建议放在内网，或叠加服务商自身的签名校验。

## Mailgun 入站示例

配置一个 webhook，把 Mailgun 字段映射到 Gitdash 的请求体：

```python
# 伪代码：POST https://gitdash.example.com/api/mail/inbound
requests.post(url, headers={"X-Gitdash-Mail-Secret": SECRET}, json={
    "to": body["recipient"],
    "from": body["sender"],
    "subject": body["subject"],
    "text": body["stripped-text"] or body["body-plain"],
})
```

## Postfix `aliases` 管道示例

把整个 `reply+...@` 域投递到脚本：

```
# /etc/postfix/virtual
reply+@mail.example.com   gitdash-inbound
```

```ini
# /etc/postfix/master.cf
gitdash-inbound unix  -  n  n  -  -  pipe
  flags=DRhu user=gitdash argv=/usr/local/bin/gitdash-inbound.py ${recipient} ${sender}
```

脚本用 Python 的 `email` 模块解析邮件、取纯文本正文，再把
`{to, from, subject, text}` POST 到 `/api/mail/inbound`。共享密钥存放在 root 可读的
文件里，通过请求头传入。

## 安全说明

- 缺少共享密钥返回 `401`；签名错误或 `exp` 过期返回 `400`；仓库不存在返回 `404`。
- 只接受 `issue` 与 `pull`，不支持行内评论。
- 被封禁的仓库 / 组织与其他接口一样会被拒绝。
- 清洗后正文上限 10000 字符。
