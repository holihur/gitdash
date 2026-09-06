package api

// 请求体 DTO（仅供 swaggo 文档引用；handler 解析逻辑不变）。

// createPATReq 创建个人访问令牌请求体。
type createPATReq struct {
	Name   string   `json:"name"`   // 令牌名称（必填，<=100 字符）
	Scopes []string `json:"scopes"` // 授权范围（可选，默认 ["repo"]；仅接受 repo/inbox/keys）
}
