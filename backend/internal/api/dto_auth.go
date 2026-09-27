package api

import "encoding/json"

// 请求体 DTO（仅供 swaggo 文档引用；handler 解析逻辑不变）。

// registerReq 注册请求体。
//
//nolint:unused // 仅供 swagger @Param 注解引用
type registerReq struct {
	Username string `json:"username"` // 用户名（4-32 位小写字母/数字/_/-，字母或数字开头）
	Password string `json:"password"` // 密码（至少 8 位，且包含小写字母/大写字母/数字/特殊字符中的至少 3 类）
}

// loginReq 登录请求体。
//
//nolint:unused // 仅供 swagger @Param 注解引用
type loginReq struct {
	Username string `json:"username"` // 用户名
	Password string `json:"password"` // 密码
}

// mfaVerifyReq MFA 二次验证请求体。
//
//nolint:unused // 仅供 swagger @Param 注解引用
type mfaVerifyReq struct {
	MFAToken string `json:"mfa_token"` // 登录时返回的临时 MFA token
	Code     string `json:"code"`      // 认证器 TOTP 代码
}

// changePasswordReq 修改密码请求体。
//
//nolint:unused // 仅供 swagger @Param 注解引用
type changePasswordReq struct {
	Current string `json:"current_password"` // 当前密码
	New     string `json:"new_password"`     // 新密码（至少 8 位，且包含小写字母/大写字母/数字/特殊字符中的至少 3 类）
}

// updateProfileReq 更新个人资料请求体。
//
//nolint:unused // 仅供 swagger @Param 注解引用
type updateProfileReq struct {
	Email       *string `json:"email"`        // 邮箱；空串表示清除
	NotifyEmail *bool   `json:"notify_email"` // 邮件通知开关（可选）
	Bio         *string `json:"bio"`          // 个人简介（可选）
}

// mfaActivateReq 激活 MFA 请求体。
//
//nolint:unused // 仅供 swagger @Param 注解引用
type mfaActivateReq struct {
	Code string `json:"code"` // 认证器 TOTP 代码
}

// mfaDisableReq 关闭 MFA 请求体。
//
//nolint:unused // 仅供 swagger @Param 注解引用
type mfaDisableReq struct {
	Password string `json:"password"` // 当前密码
	Code     string `json:"code"`     // 认证器 TOTP 代码
}

// deleteAccountReq 注销账号请求体。
//
//nolint:unused // 仅供 swagger @Param 注解引用
type deleteAccountReq struct {
	Password string `json:"password"` // 当前密码
	Code     string `json:"code"`     // 启用 MFA 时的二次验证码
}

// createKeyReq 添加 SSH 公钥请求体。
//
//nolint:unused // 仅供 swagger @Param 注解引用
type createKeyReq struct {
	Name      string `json:"name"`       // 密钥名称
	PublicKey string `json:"public_key"` // SSH 公钥内容（OpenSSH 格式）
}

// addGPGKeyReq 添加 GPG 公钥请求体。
//
//nolint:unused // 仅供 swagger @Param 注解引用
type addGPGKeyReq struct {
	Armor string `json:"armor"` // armored 格式的 GPG 公钥
}

// adminLoginReq 管理端登录请求体。
//
//nolint:unused // 仅供 swagger @Param 注解引用
type adminLoginReq struct {
	Username string `json:"username"` // 管理员用户名
	Password string `json:"password"` // 管理员密码
}

// adminChangePasswordReq 修改管理员密码请求体。
//
//nolint:unused // 仅供 swagger @Param 注解引用
type adminChangePasswordReq struct {
	Current string `json:"current_password"` // 当前密码
	New     string `json:"new_password"`     // 新密码（至少 8 位，且包含小写字母/大写字母/数字/特殊字符中的至少 3 类）
}

// ---- passkey（WebAuthn）----

// passkeyRegisterReq 开始注册 passkey 请求体。
//
//nolint:unused // 仅供 swagger @Param 注解引用
type passkeyRegisterReq struct {
	Name string `json:"name"` // 凭据名称（可空，默认由服务端推断）
}

// passkeyRegisterFinishReq 完成 passkey 注册请求体。
//
//nolint:unused // 仅供 swagger @Param 注解引用
type passkeyRegisterFinishReq struct {
	SessionID  string          `json:"session_id"` // begin 阶段返回的会话 ID
	Name       string          `json:"name"`       // 凭据名称（可空）
	Credential json.RawMessage `json:"credential"` // 浏览器 PublicKeyCredential 的 JSON 表达（base64url 字段）
}

// passkeyLoginBeginReq 开始 passkey 登录请求体。
//
//nolint:unused // 仅供 swagger @Param 注解引用
type passkeyLoginBeginReq struct {
	Username string `json:"username"` // 可选：限定该用户的凭据；为空则为 discoverable 登录
}

// passkeyLoginFinishReq 完成 passkey 登录请求体。
//
//nolint:unused // 仅供 swagger @Param 注解引用
type passkeyLoginFinishReq struct {
	SessionID  string          `json:"session_id"` // begin 阶段返回的会话 ID
	Credential json.RawMessage `json:"credential"` // 浏览器 PublicKeyCredential 的 JSON 表达（base64url 字段）
}
