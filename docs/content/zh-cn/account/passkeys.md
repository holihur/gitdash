---
title: "Passkey（WebAuthn）"
weight: 6
summary: "使用 Touch ID、Windows Hello、安全密钥或密码管理器免密码登录。"
---

Passkey 基于 WebAuthn/FIDO2 标准，让你无需密码即可登录。

## 注册 Passkey

1. 打开 **个人资料 → Passkey**。
2. 点击 **添加 Passkey**，为它取个名字（如 `MacBook`、`YubiKey`）。
3. 按浏览器提示完成验证（Touch ID、Windows Hello、硬件安全密钥或密码管理器）。

可注册多个 Passkey，随时在同一卡片中删除。

## 使用 Passkey 登录

在登录页点击 **使用 Passkey 登录**，用认证器确认即可。Passkey 支持 discoverable
（无需先输入用户名）。

## 自托管部署说明

WebAuthn 会把凭据绑定到域名（*Relying Party ID*）与 origin。gitdash 默认按请求推导，
常规 HTTPS 部署无需配置。位于反向代理之后或使用非标准域名时，请设置：

| 变量 | 说明 |
| --- | --- |
| `GITDASH_WEBAUTHN_RPID` | Relying Party ID，如 `git.example.com`（必须是域名，不能是 IP） |
| `GITDASH_WEBAUTHN_ORIGINS` | 允许的 origin，逗号分隔，如 `https://git.example.com` |
| `GITDASH_WEBAUTHN_RP_NAME` | 浏览器弹窗中展示的名称（默认 `gitdash`） |

Passkey 需要安全上下文：HTTPS，或本地开发时的 `http://localhost`。
