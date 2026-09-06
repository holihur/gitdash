package api

// 请求体 DTO（仅供 swaggo 文档引用；handler 解析逻辑不变）。

// registerReq 注册请求体。
type registerReq struct {
	Username string `json:"username"` // 用户名（2-32 位小写字母/数字/_/-，字母或数字开头）
	Password string `json:"password"` // 密码（至少 8 位）
}

// loginReq 登录请求体。
type loginReq struct {
	Username string `json:"username"` // 用户名
	Password string `json:"password"` // 密码
}

// mfaVerifyReq MFA 二次验证请求体。
type mfaVerifyReq struct {
	MFAToken string `json:"mfa_token"` // 登录时返回的临时 MFA token
	Code     string `json:"code"`      // 认证器 TOTP 代码
}

// changePasswordReq 修改密码请求体。
type changePasswordReq struct {
	Current string `json:"current_password"` // 当前密码
	New     string `json:"new_password"`     // 新密码（至少 8 位）
}

// updateProfileReq 更新个人资料请求体。
type updateProfileReq struct {
	Email *string `json:"email"` // 邮箱；空串表示清除
}

// mfaActivateReq 激活 MFA 请求体。
type mfaActivateReq struct {
	Code string `json:"code"` // 认证器 TOTP 代码
}

// mfaDisableReq 关闭 MFA 请求体。
type mfaDisableReq struct {
	Password string `json:"password"` // 当前密码
	Code     string `json:"code"`     // 认证器 TOTP 代码
}

// createKeyReq 添加 SSH 公钥请求体。
type createKeyReq struct {
	Name      string `json:"name"`       // 密钥名称
	PublicKey string `json:"public_key"` // SSH 公钥内容（OpenSSH 格式）
}

// addGPGKeyReq 添加 GPG 公钥请求体。
type addGPGKeyReq struct {
	Armor string `json:"armor"` // armored 格式的 GPG 公钥
}

// adminLoginReq 管理端登录请求体。
type adminLoginReq struct {
	Username string `json:"username"` // 管理员用户名
	Password string `json:"password"` // 管理员密码
}

// adminChangePasswordReq 修改管理员密码请求体。
type adminChangePasswordReq struct {
	Current string `json:"current_password"` // 当前密码
	New     string `json:"new_password"`     // 新密码（至少 8 位）
}
