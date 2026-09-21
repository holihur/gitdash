package authz

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"gitdash/backend/internal/grpcserver/authzv1"
)

// defaultRPCTimeout 单次授权决策的超时；授权面不可达时快速失败，避免 SSH 握手挂起。
const defaultRPCTimeout = 5 * time.Second

// RemoteAuthorizer 通过授权面 gRPC 做鉴权决策（SSH 网关独立部署时使用）。
type RemoteAuthorizer struct {
	client  authzv1.AuthzServiceClient
	timeout time.Duration
}

// NewRemote 用已建立的 AuthzService 客户端创建授权器。
func NewRemote(client authzv1.AuthzServiceClient) *RemoteAuthorizer {
	return &RemoteAuthorizer{client: client, timeout: defaultRPCTimeout}
}

func (a *RemoteAuthorizer) AuthorizePublicKey(ctx context.Context, keyType string, keyBlob []byte) (string, bool, string, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	resp, err := a.client.AuthorizePublicKey(ctx, &authzv1.AuthorizePublicKeyRequest{
		KeyType: keyType,
		KeyBlob: keyBlob,
	})
	if err != nil {
		return "", false, "", err
	}
	return resp.GetUsername(), resp.GetAuthorized(), resp.GetReason(), nil
}

func (a *RemoteAuthorizer) IsIPBanned(ctx context.Context, ip string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	resp, err := a.client.IsIPBanned(ctx, &authzv1.IsIPBannedRequest{Ip: ip})
	if err != nil {
		return false, err
	}
	return resp.GetBanned(), nil
}

func (a *RemoteAuthorizer) CanRead(ctx context.Context, owner, repo, username string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	resp, err := a.client.CanRead(ctx, &authzv1.CanReadRequest{Owner: owner, Repo: repo, Username: username})
	if err != nil {
		return false, err
	}
	return resp.GetAllowed(), nil
}

func (a *RemoteAuthorizer) CanWrite(ctx context.Context, owner, repo, username string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	resp, err := a.client.CanWrite(ctx, &authzv1.CanWriteRequest{Owner: owner, Repo: repo, Username: username})
	if err != nil {
		return false, err
	}
	return resp.GetAllowed(), nil
}

func (a *RemoteAuthorizer) BranchProtection(ctx context.Context, owner, repo, branch string) (BranchProtectionRule, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	resp, err := a.client.BranchProtection(ctx, &authzv1.BranchProtectionRequest{
		Owner:  owner,
		Repo:   repo,
		Branch: branch,
	})
	if err != nil {
		return BranchProtectionRule{}, err
	}
	return BranchProtectionRule{
		Protected:      resp.GetProtected(),
		BlockDeletion:  resp.GetBlockDeletion(),
		BlockForcePush: resp.GetBlockForcePush(),
	}, nil
}

var _ Authorizer = (*RemoteAuthorizer)(nil)

// DialOptions 连接授权面的参数。
type DialOptions struct {
	// Addr 授权面地址，如 "authz.internal:9443"。
	Addr string
	// Token 服务令牌（与 API 端 GITDASH_GRPC_TOKEN 一致），随 Bearer 头发送。
	Token string
	// CAFile 可选：设置后启用 TLS，并用该文件作为 CA（校验服务端证书）。
	// 留空则使用明文连接（仅建议内网/回环）。
	CAFile string
	// ServerName 可选：TLS 校验使用的 SNI（默认取 Addr 的主机名）。
	ServerName string
}

// Dial 建立到授权面的连接并返回客户端连接与授权器。
// 调用方负责关闭返回的 *grpc.ClientConn。
func Dial(opts DialOptions) (*grpc.ClientConn, *RemoteAuthorizer, error) {
	addr := strings.TrimSpace(opts.Addr)
	if addr == "" {
		return nil, nil, fmt.Errorf("authz: address is required")
	}
	token := strings.TrimSpace(opts.Token)
	if token == "" {
		return nil, nil, fmt.Errorf("authz: service token is required")
	}

	var creds credentials.TransportCredentials
	if caFile := strings.TrimSpace(opts.CAFile); caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, nil, fmt.Errorf("authz: read CA %s: %w", caFile, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, nil, fmt.Errorf("authz: no certificate found in %s", caFile)
		}
		creds = credentials.NewTLS(&tls.Config{
			RootCAs:    pool,
			ServerName: strings.TrimSpace(opts.ServerName),
			MinVersion: tls.VersionTLS12,
		})
	} else {
		creds = insecure.NewCredentials()
	}

	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(creds),
		grpc.WithUnaryInterceptor(bearerClientInterceptor(token)),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("authz: dial %s: %w", addr, err)
	}
	return conn, NewRemote(authzv1.NewAuthzServiceClient(conn)), nil
}

// bearerClientInterceptor 给每个一元 RPC 附加 Authorization: Bearer <token>。
func bearerClientInterceptor(token string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
