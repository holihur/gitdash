package api

import "gitdash/backend/internal/gitsvc"

// createRepoReq 创建仓库请求体。
type createRepoReq struct {
	Name        string `json:"name"`        // 仓库名
	Description string `json:"description"` // 仓库描述
	Template    string `json:"template"`    // 模板：空 = 空仓库；"readme" = 默认模版（README.md）
	Private     *bool  `json:"private"`     // 是否私有，默认 true
	Namespace   string `json:"namespace"`   // 可选：组织名（成员可把仓库建到组织下）
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

// writeCommitReq 写入提交请求体。
type writeCommitReq struct {
	Branch  string              `json:"branch"`  // 目标分支，可选，默认 main
	Message string              `json:"message"` // 提交信息
	Changes []gitsvc.FileChange `json:"changes"` // 文件变更列表（create/update/delete/delete_tree）
}

// setRepoVisibilityReq 设置仓库可见性请求体。
type setRepoVisibilityReq struct {
	Private *bool `json:"private"` // 是否私有（必填）
}

// addCollabReq 添加/更新协作者请求体。
type addCollabReq struct {
	Username   string `json:"username"`   // 协作者用户名
	Permission string `json:"permission"` // 权限：read 或 write
}
