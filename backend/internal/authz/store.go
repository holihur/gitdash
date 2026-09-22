package authz

import (
	"context"
	"errors"

	"gitdash/backend/internal/store"
)

// StoreAuthorizer 直接读数据库做鉴权决策，等价于拆分前 sshserver 的进程内行为。
type StoreAuthorizer struct {
	st *store.Store
}

// NewStore 创建基于本地 store 的授权器。
func NewStore(st *store.Store) *StoreAuthorizer {
	return &StoreAuthorizer{st: st}
}

// AuthorizePublicKey 匹配用户公钥或仓库 deploy key，返回登录身份。
// deploy key 的身份为合成字符串（store.DeployIdentity），SSH 网关据此限制仓库范围。
func (a *StoreAuthorizer) AuthorizePublicKey(ctx context.Context, keyType string, keyBlob []byte) (string, bool, string, error) {
	return a.st.MatchSSHKey(keyType, keyBlob)
}

func (a *StoreAuthorizer) IsIPBanned(_ context.Context, ip string) (bool, error) {
	return a.st.IsIPBanned(ip), nil
}

func (a *StoreAuthorizer) CanRead(_ context.Context, owner, repo, username string) (bool, error) {
	return a.st.CanRead(owner, repo, username), nil
}

func (a *StoreAuthorizer) CanWrite(_ context.Context, owner, repo, username string) (bool, error) {
	return a.st.CanWrite(owner, repo, username), nil
}

func (a *StoreAuthorizer) BranchProtection(_ context.Context, owner, repo, branch string) (BranchProtectionRule, error) {
	prot, err := a.st.GetBranchProtection(owner, repo, branch)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return BranchProtectionRule{}, nil
		}
		return BranchProtectionRule{}, err
	}
	return BranchProtectionRule{
		Protected:      true,
		BlockDeletion:  prot.BlockDeletion,
		BlockForcePush: prot.BlockForcePush,
	}, nil
}

var _ Authorizer = (*StoreAuthorizer)(nil)
