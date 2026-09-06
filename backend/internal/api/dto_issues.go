package api

// createIssueReq 创建 Issue 请求体。
type createIssueReq struct {
	Title string `json:"title"` // Issue 标题（必填，最长 200 字符）
	Body  string `json:"body"`  // Issue 正文（可选，最长 10000 字符）
}

// setIssueStateReq 修改 Issue 状态请求体。
type setIssueStateReq struct {
	State string `json:"state"` // 目标状态（open 或 closed）
}

// setIssueLabelsReq 设置 Issue 标签请求体。
type setIssueLabelsReq struct {
	LabelIDs []int64 `json:"label_ids"` // 要设置的标签 ID 列表
}

// setIssueMilestoneReq 设置 Issue 里程碑请求体。
type setIssueMilestoneReq struct {
	MilestoneID int64 `json:"milestone_id"` // 里程碑 ID（0 = 清除）
}

// createLabelReq 创建标签请求体。
type createLabelReq struct {
	Name  string `json:"name"`  // 标签名称（必填，最长 50 字符）
	Color string `json:"color"` // 标签颜色（6 位十六进制，可带 # 前缀）
}

// updateLabelReq 更新标签请求体。
type updateLabelReq struct {
	Name  string `json:"name"`  // 新名称（空则保持不变）
	Color string `json:"color"` // 新颜色（空则保持不变）
}

// createMilestoneReq 创建里程碑请求体。
type createMilestoneReq struct {
	Title       string `json:"title"`       // 里程碑标题（必填）
	Description string `json:"description"` // 里程碑描述（可选）
}

// updateMilestoneReq 更新里程碑请求体。
type updateMilestoneReq struct {
	Title       string `json:"title"`       // 新标题
	Description string `json:"description"` // 新描述
	State       string `json:"state"`       // 状态（open/closed，空则保持不变）
}

// createPullReq 创建 PR 请求体。
type createPullReq struct {
	Title        string `json:"title"`         // PR 标题（必填）
	Body         string `json:"body"`          // PR 正文（可选）
	SourceBranch string `json:"source_branch"` // 源分支（必填，可带 refs/heads/ 前缀）
	TargetBranch string `json:"target_branch"` // 目标分支（必填，可带 refs/heads/ 前缀）
}

// mergePullReq 合并 PR 请求体。
type mergePullReq struct {
	Method string `json:"method"` // 合并方式（fast-forward/merge/squash，空 = fast-forward）
}

// setPullStateReq 修改 PR 状态请求体。
type setPullStateReq struct {
	State string `json:"state"` // 目标状态（open 或 closed）
}
