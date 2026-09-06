package api

// createOrgReq 创建组织请求体。
type createOrgReq struct {
	Name    string `json:"name"`    // 组织名（2-32 字符，小写字母/数字/_/-，字母数字开头）
	Display string `json:"display"` // 显示名
}

// addOrgMemberReq 添加组织成员请求体。
type addOrgMemberReq struct {
	Username string `json:"username"` // 成员用户名
	Role     string `json:"role"`     // 角色：member 或 owner，默认 member
}

// createWebhookReq 创建 webhook 请求体。
type createWebhookReq struct {
	URL    string `json:"url"`    // 回调地址，http(s) URL，最长 2048 字符
	Secret string `json:"secret"` // 可选签名密钥，至少 16 字符
}

// setPipelineReq 设置流水线开关请求体。
type setPipelineReq struct {
	Enabled bool `json:"enabled"` // 是否启用流水线
}

// createPipelineRunReq 手动触发流水线请求体。
type createPipelineRunReq struct {
	Ref string `json:"ref"` // 可选分支名，为空时使用仓库默认分支
}
