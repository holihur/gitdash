// Package authz 定义 SSH 服务所需的鉴权决策抽象，使 internal/sshserver 不再
// 直接依赖 *store.Store，从而支持 API 与 SSH 分机部署。提供两种实现：
//
//   - StoreAuthorizer：进程内直接读数据库（默认，API/SSH 同机部署）；
//   - RemoteAuthorizer：通过授权面 gRPC（AuthzService）决策（SSH 网关独立部署）。
//
// 两者语义必须保持一致（见各方法注释与 proto/authz/v1/authz.proto）。
package authz

import "context"

// BranchProtectionRule 分支保护规则（无规则时 Protected=false）。
// 强推（force push）的祖先判断需要本地仓库，由调用方拿到规则后自行完成。
type BranchProtectionRule struct {
	Protected      bool
	BlockDeletion  bool
	BlockForcePush bool
}

// Authorizer 抽象 SSH 网关所需的全部鉴权决策。
// 所有方法都以错误返回表示“授权面不可用”；调用方应据此失败关闭（fail-closed）。
type Authorizer interface {
	// AuthorizePublicKey 比对 SSH 公钥并返回其所属用户。
	// authorized=false 时 reason 说明原因（如 "unknown public key"、"account is banned"）。
	AuthorizePublicKey(ctx context.Context, keyType string, keyBlob []byte) (username string, authorized bool, reason string, err error)

	// IsIPBanned 报告来源 IP 是否命中管理员黑名单（IP/CIDR）。
	IsIPBanned(ctx context.Context, ip string) (banned bool, err error)

	// CanRead 报告用户对仓库是否有读权限（clone/fetch/archive）。
	CanRead(ctx context.Context, owner, repo, username string) (allowed bool, err error)

	// CanWrite 报告用户对仓库是否有写权限（push）。
	CanWrite(ctx context.Context, owner, repo, username string) (allowed bool, err error)

	// BranchProtection 返回指定分支的保护规则（供 pre-receive hook 校验 push）。
	BranchProtection(ctx context.Context, owner, repo, branch string) (BranchProtectionRule, error)
}
