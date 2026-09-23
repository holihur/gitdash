package api

import "gitdash/backend/internal/gitsvc"

// createRepoReq 创建仓库请求体。
type createRepoReq struct {
	Name        string `json:"name"`        // 仓库名
	Description string `json:"description"` // 仓库描述
	Template    string `json:"template"`    // 模板：空 = 空仓库；"readme" = 默认模版（README.md + .gitdash.yml）
	Private     *bool  `json:"private"`     // 是否私有，默认 true
	Namespace   string `json:"namespace"`   // 可选：组织名（成员可把仓库建到组织下）

	// 从模版仓库创建：指定源仓库（owner/name），克隆其内容与历史。
	TemplateOwner string `json:"template_owner"`
	TemplateName  string `json:"template_name"`
}

// forkRepoReq fork 仓库请求体。
type forkRepoReq struct {
	Name      string `json:"name"`      // 目标仓库名，缺省用源仓库名
	Namespace string `json:"namespace"` // 可选：组织命名空间
}

// importRepoReq 导入仓库请求体。
type importRepoReq struct {
	URL        string `json:"url"`         // 外部仓库地址（http(s)/ssh/git）
	Name       string `json:"name"`        // 目标仓库名，可选，缺省从 URL 推断
	Namespace  string `json:"namespace"`   // 可选：组织命名空间
	Private    *bool  `json:"private"`     // 是否私有，默认 true
	PrivateKey string `json:"private_key"` // 可选：拉取私有仓库用的 SSH 私钥
}

// setMirrorReq 设置推送镜像请求体。
type setMirrorReq struct {
	URL        string `json:"url"`         // 镜像目标地址
	PrivateKey string `json:"private_key"` // 可选：推送用的 SSH 私钥
}

// createRefReq 创建分支/标签请求体。
type createRefReq struct {
	Type string `json:"type"` // 类型：branch 或 tag
	Name string `json:"name"` // 分支/标签名
	From string `json:"from"` // 起点引用，可选，缺省 HEAD
}

// setRefNoteReq 设置分支/标签备注请求体。
type setRefNoteReq struct {
	Note string `json:"note"` // 备注内容，空串表示清除备注
}

// writeCommitReq 写入提交请求体。
type writeCommitReq struct {
	Branch  string              `json:"branch"`  // 目标分支，可选，默认 main
	Message string              `json:"message"` // 提交信息
	Changes []gitsvc.FileChange `json:"changes"` // 文件变更列表（create/update/delete/delete_tree/move）
}

// revertCommitReq 撤销提交请求体。
type revertCommitReq struct {
	Branch  string `json:"branch"`  // 目标分支（必填，通常为当前查看的分支）
	Message string `json:"message"` // 提交信息，可选，缺省自动生成 "Revert <原始主题>"
}

// setRepoVisibilityReq 设置仓库可见性请求体。
type setRepoVisibilityReq struct {
	Private    *bool  `json:"private"`    // 兼容：是否私有
	Visibility string `json:"visibility"` // private | public | anonymous
}

// setRepoTemplateReq 设置模版仓库请求体。
type setRepoTemplateReq struct {
	IsTemplate *bool `json:"is_template"` // 是否为模版仓库（必填）
}

// setRepoDefaultBranchReq 设置默认分支请求体。
type setRepoDefaultBranchReq struct {
	Branch string `json:"branch"` // 默认分支名（必须已存在）
}

// setRepoIssuesReq 开启/关闭 issue 功能请求体。
type setRepoIssuesReq struct {
	HasIssues *bool `json:"has_issues"` // 是否启用 issue（必填）
}

// setRepoDescriptionReq 修改仓库描述请求体。
type setRepoDescriptionReq struct {
	Description string `json:"description"` // 仓库描述（可为空，最长 500 字符）
}

// addCollabReq 添加/更新协作者请求体。
type addCollabReq struct {
	Username   string `json:"username"`   // 协作者用户名
	Permission string `json:"permission"` // 权限：read 或 write
}

// setRepoEnvVarReq 设置仓库级流水线环境变量请求体。
type setRepoEnvVarReq struct {
	Key   string `json:"key"`   // 环境变量名（[A-Za-z_][A-Za-z0-9_]*）
	Value string `json:"value"` // 环境变量值
}

// setRepoSecretReq 设置仓库 CI secret 请求体（值加密存储，永不回传）。
type setRepoSecretReq struct {
	Name  string `json:"name"`  // secret 名（[A-Za-z_][A-Za-z0-9_]*）
	Value string `json:"value"` // secret 值
}
