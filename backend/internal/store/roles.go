package store

// 仓库角色（从低到高）：read < triage < write < maintain < admin < owner。
//
//   - read      只读：浏览代码 / issue / PR、clone
//   - triage    read + 议题 / PR 管理（标签、里程碑、指派、关闭/重开、评论），不能推代码
//   - write     triage + 推代码 / 文件操作 / release / pages / 触发流水线
//   - maintain  write + 仓库设置（webhook、deploy key、分支保护、环境变量/密钥）
//   - admin     maintain + 协作者与可见性管理
//   - owner     仓库所有者（用户本人或组织 owner），最高
//
// owner 不是可授予协作者的角色，只能由「本人 / 组织 owner」天然获得。
const (
	RoleRead     = "read"
	RoleTriage   = "triage"
	RoleWrite    = "write"
	RoleMaintain = "maintain"
	RoleAdmin    = "admin"
	RoleOwner    = "owner"
)

var roleRanks = map[string]int{
	RoleRead:     1,
	RoleTriage:   2,
	RoleWrite:    3,
	RoleMaintain: 4,
	RoleAdmin:    5,
	RoleOwner:    6,
}

// RoleRank 返回角色等级；未知角色为 0。
func RoleRank(role string) int { return roleRanks[role] }

// RoleAtLeast 判断 role 是否达到 min 等级。
func RoleAtLeast(role, min string) bool { return RoleRank(role) >= RoleRank(min) }

// ValidCollabRole 报告该角色是否可作为协作者权限授予（owner 不可授予）。
func ValidCollabRole(role string) bool {
	r, ok := roleRanks[role]
	return ok && r >= 1 && r <= 5
}
