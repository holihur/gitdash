// Package grpcserver 提供 gitdash 的授权面 gRPC 服务（AuthzService）。
//
// 设计目标：把 SSH 服务的鉴权逻辑收敛为稳定的 RPC 契约。默认 all-in-one 部署下
// SSH 仍直接读 store；设置 GITDASH_ROLE=ssh 后，独立 SSH 网关通过本服务鉴权。
// 本服务只做「决策」，不接触仓库数据。默认不启动，需显式配置 GITDASH_GRPC_ADDR。
//
// 安全默认：
//   - 仅在显式设置 GITDASH_GRPC_ADDR 时监听（默认关闭，老部署零影响）；
//   - 必须提供 GITDASH_GRPC_TOKEN，所有 RPC 经一元拦截器校验 Bearer 令牌；
//   - 跨机可选用 TLS（GITDASH_GRPC_TLS_CERT/KEY）；
//   - 不注册 reflection，避免暴露接口面。
package grpcserver

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"gitdash/backend/internal/grpcserver/authzv1"
	"gitdash/backend/internal/store"
)

// maxRecvMsgSize 授权请求都很小；限制入站消息体，降低滥用面。
const maxRecvMsgSize = 1 << 20 // 1 MiB

// AuthzServer 实现 authzv1.AuthzService，决策逻辑与 internal/sshserver 的
// PublicKeyCallback / CanRead / CanWrite 保持一致。
type AuthzServer struct {
	authzv1.UnimplementedAuthzServiceServer
	st *store.Store
}

// New 创建授权面服务实现。
func New(st *store.Store) *AuthzServer {
	return &AuthzServer{st: st}
}

// AuthorizePublicKey 比对 SSH 公钥并返回登录身份：普通用户返回用户名，
// 仓库 deploy key 返回合成身份 "deploy:<owner>/<repo>:<rw>"（见 store.DeployIdentity）。
//
// 与 sshserver.PublicKeyCallback 语义一致，均委派 store.MatchSSHKey：
//  1. 先按 (type, wire-blob) 匹配用户公钥，命中后校验封禁；
//  2. 再按指纹匹配仓库 deploy key，命中后校验仓库/属主封禁；
//  3. 未命中返回 authorized=false。
//
// 注意：比对在服务端完成，绝不回传公钥列表，避免授权面成为信息泄漏点。
func (s *AuthzServer) AuthorizePublicKey(_ context.Context, req *authzv1.AuthorizePublicKeyRequest) (*authzv1.AuthorizePublicKeyResponse, error) {
	identity, authorized, reason, err := s.st.MatchSSHKey(req.GetKeyType(), req.GetKeyBlob())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "match public key: %v", err)
	}
	if !authorized {
		return &authzv1.AuthorizePublicKeyResponse{Authorized: false, Reason: reason}, nil
	}
	return &authzv1.AuthorizePublicKeyResponse{Authorized: true, Username: identity}, nil
}

// IsIPBanned 报告来源 IP 是否命中管理员黑名单（IP/CIDR）。
func (s *AuthzServer) IsIPBanned(_ context.Context, req *authzv1.IsIPBannedRequest) (*authzv1.IsIPBannedResponse, error) {
	return &authzv1.IsIPBannedResponse{Banned: s.st.IsIPBanned(req.GetIp())}, nil
}

// CanRead 报告用户对仓库是否有读权限（clone/fetch/archive）。
func (s *AuthzServer) CanRead(_ context.Context, req *authzv1.CanReadRequest) (*authzv1.CanReadResponse, error) {
	return &authzv1.CanReadResponse{
		Allowed: s.st.CanRead(req.GetOwner(), req.GetRepo(), req.GetUsername()),
	}, nil
}

// CanWrite 报告用户对仓库是否有写权限（push）。
func (s *AuthzServer) CanWrite(_ context.Context, req *authzv1.CanWriteRequest) (*authzv1.CanWriteResponse, error) {
	return &authzv1.CanWriteResponse{
		Allowed: s.st.CanWrite(req.GetOwner(), req.GetRepo(), req.GetUsername()),
	}, nil
}

// BranchProtection 返回指定分支的保护规则（无规则时 protected=false）。
func (s *AuthzServer) BranchProtection(_ context.Context, req *authzv1.BranchProtectionRequest) (*authzv1.BranchProtectionResponse, error) {
	prot, err := s.st.GetBranchProtection(req.GetOwner(), req.GetRepo(), req.GetBranch())
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return &authzv1.BranchProtectionResponse{}, nil
		}
		return nil, status.Errorf(codes.Internal, "get branch protection: %v", err)
	}
	return &authzv1.BranchProtectionResponse{
		Protected:      true,
		BlockDeletion:  prot.BlockDeletion,
		BlockForcePush: prot.BlockForcePush,
	}, nil
}

// NewGRPCServer 构造带令牌校验的 gRPC server（未启动监听）。测试可直接复用。
// extra 用于追加 ServerOption（如 grpc.Creds 开启 TLS）。
func NewGRPCServer(st *store.Store, token string, extra ...grpc.ServerOption) (*grpc.Server, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("grpc authz: GITDASH_GRPC_TOKEN must be set")
	}
	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(tokenAuthInterceptor(token)),
		grpc.MaxRecvMsgSize(maxRecvMsgSize),
	}
	opts = append(opts, extra...)
	srv := grpc.NewServer(opts...)
	authzv1.RegisterAuthzServiceServer(srv, New(st))
	return srv, nil
}

// Serve 在 addr 上启动授权面 gRPC 服务（明文，阻塞，直到 listener 关闭或出错）。
func Serve(addr, token string, st *store.Store) error {
	return serve(addr, st, token)
}

// ServeTLS 在 addr 上启动授权面 gRPC 服务（TLS，阻塞）。
// 跨机部署时应启用 TLS，避免服务令牌在网络上明文传输。
func ServeTLS(addr, token, certFile, keyFile string, st *store.Store) error {
	creds, err := credentials.NewServerTLSFromFile(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("grpc authz tls: %w", err)
	}
	return serve(addr, st, token, grpc.Creds(creds))
}

func serve(addr string, st *store.Store, token string, extra ...grpc.ServerOption) error {
	srv, err := NewGRPCServer(st, token, extra...)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return srv.Serve(ln)
}

// tokenAuthInterceptor 校验每个一元 RPC 的 Bearer 令牌（常数时间比较）。
func tokenAuthInterceptor(token string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !hasValidToken(ctx, token) {
			return nil, status.Error(codes.Unauthenticated, "invalid service token")
		}
		return handler(ctx, req)
	}
}

// hasValidToken 从 incoming metadata 的 authorization 头提取 Bearer 令牌并比较。
func hasValidToken(ctx context.Context, token string) bool {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}
	for _, v := range md.Get("authorization") {
		v = strings.TrimSpace(v)
		if len(v) >= 7 && strings.EqualFold(v[:7], "bearer ") {
			v = strings.TrimSpace(v[7:])
		}
		if subtle.ConstantTimeCompare([]byte(v), []byte(token)) == 1 {
			return true
		}
	}
	return false
}
