package authz

import (
	"bytes"
	"context"
	"errors"

	"golang.org/x/crypto/ssh"

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

// AuthorizePublicKey 遍历已登记公钥，按 (type, wire-blob) 精确匹配，命中后校验封禁。
func (a *StoreAuthorizer) AuthorizePublicKey(_ context.Context, keyType string, keyBlob []byte) (string, bool, string, error) {
	keys, err := a.st.PublicKeys()
	if err != nil {
		return "", false, "", err
	}
	for _, ka := range keys {
		parsed, _, _, _, perr := ssh.ParseAuthorizedKey([]byte(ka.Line))
		if perr != nil {
			continue
		}
		if parsed.Type() == keyType && bytes.Equal(parsed.Marshal(), keyBlob) {
			if a.st.IsUserBanned(ka.Username) {
				return "", false, "account is banned", nil
			}
			return ka.Username, true, "", nil
		}
	}
	return "", false, "unknown public key", nil
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
