package api

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"gitdash/backend/internal/store"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// Passkey（WebAuthn / FIDO2）支持：注册凭据、凭据管理，以及免密码登录。
//
// RP ID / Origin 默认由请求推导（反代场景沿用 reqBase 的 X-Forwarded-* 逻辑），
// 也可用环境变量显式固定，适配多域名 / 非标准端口部署：
//
//	GITDASH_WEBAUTHN_RPID      relying party id（默认取请求 Host 的主机名）
//	GITDASH_WEBAUTHN_ORIGINS   允许的 origin，逗号分隔（默认取请求 scheme://host）
//	GITDASH_WEBAUTHN_RP_NAME   relying party 展示名（默认 "gitdash"）

const (
	webAuthnSessionTTL = 5 * time.Minute
	// passkeyChallengeBodyLimit 限制 attestation / assertion JSON 大小。
	passkeyChallengeBodyLimit = 256 << 10
)

// webauthnUser 实现 webauthn.User 接口。user handle 使用用户主键的 8 字节大端编码，
// 稳定且不暴露用户名（discoverable 登录时由 userHandle 反查用户）。
type webauthnUser struct {
	id          int64
	username    string
	credentials []webauthn.Credential
}

func (u *webauthnUser) WebAuthnID() []byte { return webauthnUserHandle(u.id) }
func (u *webauthnUser) WebAuthnName() string {
	return u.username
}
func (u *webauthnUser) WebAuthnDisplayName() string { return u.username }
func (u *webauthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}

func webauthnUserHandle(id int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(id))
	return b
}

func webauthnCredentialID(cred *webauthn.Credential) string {
	return base64.RawURLEncoding.EncodeToString(cred.ID)
}

// webauthnRequest 依据请求推导 RP 配置并构造 WebAuthn 客户端。
func (a *API) webauthnRequest(r *http.Request) (*webauthn.WebAuthn, error) {
	return webauthn.New(&webauthn.Config{
		RPDisplayName: webauthnRPDisplayName(),
		RPID:          webauthnRPID(r),
		RPOrigins:     webauthnOrigins(r),
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
			UserVerification: protocol.VerificationPreferred,
		},
		AttestationPreference: protocol.PreferNoAttestation,
	})
}

func webauthnRPDisplayName() string {
	if v := strings.TrimSpace(os.Getenv("GITDASH_WEBAUTHN_RP_NAME")); v != "" {
		return v
	}
	return "gitdash"
}

// webauthnRPID 返回 relying party id：显式配置优先，否则取请求 Host 的主机名（去端口）。
func webauthnRPID(r *http.Request) string {
	if v := strings.TrimSpace(os.Getenv("GITDASH_WEBAUTHN_RPID")); v != "" {
		return v
	}
	base := reqBase(r)
	if u, err := url.Parse(base); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	host := r.Host
	if i := strings.LastIndexByte(host, ':'); i > 0 && !strings.Contains(host, "]") {
		host = host[:i]
	}
	return host
}

// webauthnOrigins 返回允许的 origin 列表：显式配置优先，否则取当前请求 origin。
func webauthnOrigins(r *http.Request) []string {
	if v := strings.TrimSpace(os.Getenv("GITDASH_WEBAUTHN_ORIGINS")); v != "" {
		out := make([]string, 0, 2)
		for _, part := range strings.Split(v, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return []string{reqBase(r)}
}

// loadWebAuthnUser 读取用户及其全部 passkey，组装为 webauthn.User。
func (a *API) loadWebAuthnUser(username string) (*webauthnUser, error) {
	ua, err := a.store.GetByUsername(username)
	if err != nil {
		return nil, err
	}
	recs, err := a.store.ListWebAuthnCredentials(username)
	if err != nil {
		return nil, err
	}
	creds := make([]webauthn.Credential, 0, len(recs))
	for _, rec := range recs {
		var c webauthn.Credential
		if err := json.Unmarshal(rec.Data, &c); err != nil {
			continue // 跳过损坏记录，避免单个坏凭据阻断登录
		}
		creds = append(creds, c)
	}
	return &webauthnUser{id: ua.ID, username: ua.Username, credentials: creds}, nil
}

// webauthnDiscoverable 解析 discoverable 登录：按凭据 ID 定位记录，并校验 user handle 一致。
func (a *API) webauthnDiscoverable(rawID, userHandle []byte) (webauthn.User, error) {
	rec, err := a.store.WebAuthnCredentialByCredentialID(base64.RawURLEncoding.EncodeToString(rawID))
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(webauthnUserHandle(rec.UserID), userHandle) {
		return nil, errors.New("credential user handle mismatch")
	}
	return a.loadWebAuthnUser(rec.Username)
}

// ---- ceremony session ----

func (a *API) saveWebAuthnSession(s *webauthn.SessionData) (string, error) {
	token, err := newSessionToken()
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	expires := time.Now().Add(webAuthnSessionTTL).UTC().Format(time.RFC3339)
	if err := a.store.PutWebAuthnSession(token, raw, expires); err != nil {
		return "", err
	}
	return token, nil
}

func (a *API) takeWebAuthnSession(token string) (*webauthn.SessionData, error) {
	if strings.TrimSpace(token) == "" {
		return nil, store.ErrNotFound
	}
	raw, err := a.store.TakeWebAuthnSession(token, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	var s webauthn.SessionData
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// readOptionalJSON 解码可选 JSON 请求体；空 body 视为零值，非法 JSON 返回错误。
func readOptionalJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, passkeyChallengeBodyLimit)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_json_body", "invalid json body")
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_json_body", "invalid json body")
		return err
	}
	return nil
}

// passkeyUnavailable 统一处理 WebAuthn 配置/构造失败。
func passkeyUnavailable(w http.ResponseWriter) {
	writeCode(w, http.StatusServiceUnavailable, "passkey_unavailable",
		"passkey is not available for this origin; configure GITDASH_WEBAUTHN_RPID/ORIGINS")
}

// ---- 凭据管理（需登录）----

// passkeyDTO 将存储记录转换为脱敏的 API 表达。
func passkeyDTO(rec store.WebAuthnCredentialRecord) store.WebAuthnCredential {
	dto := store.WebAuthnCredential{
		ID:           rec.ID,
		Name:         rec.Name,
		CredentialID: rec.CredentialID,
		CreatedAt:    rec.CreatedAt,
		LastUsedAt:   rec.LastUsedAt,
	}
	var c webauthn.Credential
	if json.Unmarshal(rec.Data, &c) == nil {
		for _, t := range c.Transport {
			dto.Transports = append(dto.Transports, string(t))
		}
		dto.BackupEligible = c.Flags.BackupEligible
		dto.BackupState = c.Flags.BackupState
	}
	return dto
}

// listPasskeys 列出当前用户的全部 passkey。
//
//	@Summary     列出 Passkey
//	@Description 返回当前用户已注册的 passkey 列表。
//	@Tags        users
//	@Produce     json
//	@Security    BearerAuth
//	@Success     200 {object} map[string]any
//	@Failure     401 {object} map[string]string
//	@Router      /me/passkeys [get]
func (a *API) listPasskeys(w http.ResponseWriter, r *http.Request) {
	recs, err := a.store.ListWebAuthnCredentials(userFrom(r))
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]store.WebAuthnCredential, 0, len(recs))
	for _, rec := range recs {
		out = append(out, passkeyDTO(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{"passkeys": out})
}

// passkeyRegisterBegin 开始注册一个新的 passkey。
//
//	@Summary     开始注册 Passkey
//	@Description 生成 WebAuthn 注册挑战与选项；返回 session_id 供 finish 使用。
//	@Tags        users
//	@Accept      json
//	@Produce     json
//	@Security    BearerAuth
//	@Param       body body passkeyRegisterReq false "凭据名称（可空）"
//	@Success     200 {object} map[string]any
//	@Failure     401 {object} map[string]string
//	@Failure     503 {object} map[string]string
//	@Router      /me/passkeys/register/begin [post]
func (a *API) passkeyRegisterBegin(w http.ResponseWriter, r *http.Request) {
	username := userFrom(r)
	user, err := a.loadWebAuthnUser(username)
	if err != nil {
		internalError(w, err)
		return
	}
	wl, err := a.webauthnRequest(r)
	if err != nil {
		passkeyUnavailable(w)
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := readOptionalJSON(w, r, &in); err != nil {
		return
	}
	opts := []webauthn.RegistrationOption{
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementPreferred),
		webauthn.WithExtensions(webauthn.WithExtensionCredProps()),
	}
	if len(user.credentials) > 0 {
		opts = append(opts, webauthn.WithExclusions(
			webauthn.Credentials(user.credentials).CredentialDescriptors()))
	}
	creation, session, err := wl.BeginRegistration(user, opts...)
	if err != nil {
		passkeyUnavailable(w)
		return
	}
	token, err := a.saveWebAuthnSession(session)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": token,
		"public_key": creation,
		"name":       strings.TrimSpace(in.Name),
	})
}

// passkeyRegisterFinish 完成 passkey 注册并持久化凭据。
//
//	@Summary     完成 Passkey 注册
//	@Description 校验 attestation 后保存凭据；返回新建的 passkey。
//	@Tags        users
//	@Accept      json
//	@Produce     json
//	@Security    BearerAuth
//	@Param       body body passkeyRegisterFinishReq true "session_id 与 attestation"
//	@Success     201 {object} store.WebAuthnCredential
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Router      /me/passkeys/register/finish [post]
func (a *API) passkeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	username := userFrom(r)
	var in struct {
		SessionID  string          `json:"session_id"`
		Name       string          `json:"name"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if len(in.Credential) == 0 {
		writeCode(w, http.StatusBadRequest, "invalid_passkey_response", "missing credential")
		return
	}
	session, err := a.takeWebAuthnSession(in.SessionID)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "passkey_challenge_expired", "registration challenge expired, try again")
		return
	}
	wl, err := a.webauthnRequest(r)
	if err != nil {
		passkeyUnavailable(w)
		return
	}
	user, err := a.loadWebAuthnUser(username)
	if err != nil {
		internalError(w, err)
		return
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(in.Credential)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_passkey_response", "invalid credential response")
		return
	}
	cred, err := wl.CreateCredential(user, *session, parsed)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_passkey_response", "could not verify credential")
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = passkeyDefaultName(cred)
	}
	data, err := json.Marshal(cred)
	if err != nil {
		internalError(w, err)
		return
	}
	id, err := a.store.AddWebAuthnCredential(username, name, webauthnCredentialID(cred), data)
	if errors.Is(err, store.ErrExists) {
		writeCode(w, http.StatusConflict, "passkey_already_registered", "this passkey is already registered")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	dto := store.WebAuthnCredential{
		ID:             id,
		Name:           name,
		CredentialID:   webauthnCredentialID(cred),
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		BackupEligible: cred.Flags.BackupEligible,
		BackupState:    cred.Flags.BackupState,
	}
	for _, t := range cred.Transport {
		dto.Transports = append(dto.Transports, string(t))
	}
	writeJSON(w, http.StatusCreated, dto)
}

// passkeyDefaultName 依据 authenticator 是否可备份给一个通用名称。
func passkeyDefaultName(cred *webauthn.Credential) string {
	if cred.Flags.BackupEligible {
		return "Passkey"
	}
	return "Security key"
}

// deletePasskey 删除当前用户名下的一条 passkey。
//
//	@Summary     删除 Passkey
//	@Description 删除指定的 passkey；不存在返回 404。
//	@Tags        users
//	@Produce     json
//	@Security    BearerAuth
//	@Param       id path int true "passkey id"
//	@Success     204 {object} nil
//	@Failure     401 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Router      /me/passkeys/{id} [delete]
func (a *API) deletePasskey(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_passkey_id", "invalid passkey id")
		return
	}
	if err := a.store.DeleteWebAuthnCredential(userFrom(r), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "passkey")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- 登录（无需登录）----

// passkeyLoginBegin 开始 passkey 登录。
//
//	@Summary     开始 Passkey 登录
//	@Description 生成 WebAuthn 断言挑战；可携带 username 以限定凭据（否则为 discoverable 登录）。
//	@Tags        auth
//	@Accept      json
//	@Produce     json
//	@Param       body body passkeyLoginBeginReq false "可选 username"
//	@Success     200 {object} map[string]any
//	@Failure     429 {object} map[string]string
//	@Failure     503 {object} map[string]string
//	@Router      /auth/passkey/begin [post]
func (a *API) passkeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
	}
	if err := readOptionalJSON(w, r, &in); err != nil {
		return
	}
	username := strings.ToLower(strings.TrimSpace(in.Username))
	rateKey := a.rateKey("passkey:"+username, clientIP(r))
	if a.rateBlocked(rateKey) {
		writeCode(w, http.StatusTooManyRequests, "too_many_attempts", "too many attempts, try again later")
		return
	}
	wl, err := a.webauthnRequest(r)
	if err != nil {
		passkeyUnavailable(w)
		return
	}
	var (
		assertion *protocol.CredentialAssertion
		session   *webauthn.SessionData
	)
	user, uerr := a.loadWebAuthnUser(username)
	if username != "" && uerr == nil && len(user.credentials) > 0 {
		assertion, session, err = wl.BeginLogin(user)
	} else {
		assertion, session, err = wl.BeginDiscoverableLogin()
	}
	if err != nil {
		passkeyUnavailable(w)
		return
	}
	token, err := a.saveWebAuthnSession(session)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": token,
		"public_key": assertion,
	})
}

// passkeyLoginFinish 完成 passkey 登录并签发会话。
//
//	@Summary     完成 Passkey 登录
//	@Description 校验 assertion 后签发正式会话 token / cookie。
//	@Tags        auth
//	@Accept      json
//	@Produce     json
//	@Param       body body passkeyLoginFinishReq true "session_id 与 assertion"
//	@Success     200 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     429 {object} map[string]string
//	@Router      /auth/passkey/finish [post]
func (a *API) passkeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SessionID  string          `json:"session_id"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if len(in.Credential) == 0 {
		writeCode(w, http.StatusBadRequest, "invalid_passkey_response", "missing credential")
		return
	}
	ipKey := a.rateKey("passkey-finish", clientIP(r))
	if a.rateBlocked(ipKey) {
		writeCode(w, http.StatusTooManyRequests, "too_many_attempts", "too many attempts, try again later")
		return
	}
	session, err := a.takeWebAuthnSession(in.SessionID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "passkey_challenge_expired", "passkey challenge expired, try again")
		return
	}
	wl, err := a.webauthnRequest(r)
	if err != nil {
		passkeyUnavailable(w)
		return
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(in.Credential)
	if err != nil {
		a.rateFail(ipKey)
		writeCode(w, http.StatusUnauthorized, "invalid_passkey_response", "invalid credential response")
		return
	}
	var (
		username string
		cred     *webauthn.Credential
	)
	if len(session.UserID) > 0 { // 已知用户名：以限定凭据列表校验
		ua, uerr := a.store.GetByID(int64(binary.BigEndian.Uint64(session.UserID)))
		if uerr != nil {
			a.rateFail(ipKey)
			writeCode(w, http.StatusUnauthorized, "invalid_passkey_response", "could not verify credential")
			return
		}
		user, uerr := a.loadWebAuthnUser(ua.Username)
		if uerr != nil {
			internalError(w, uerr)
			return
		}
		cred, err = wl.ValidateLogin(user, *session, parsed)
		username = ua.Username
	} else { // discoverable / 密码less 登录
		user, c, uerr := wl.ValidatePasskeyLogin(a.webauthnDiscoverable, *session, parsed)
		cred, err = c, uerr
		if user != nil {
			username = user.WebAuthnName()
		}
	}
	if err != nil || cred == nil || username == "" {
		a.rateFail(ipKey)
		writeCode(w, http.StatusUnauthorized, "invalid_passkey_response", "could not verify credential")
		return
	}
	a.rateReset(ipKey)
	if data, merr := json.Marshal(cred); merr == nil {
		_ = a.store.UpdateWebAuthnCredential(webauthnCredentialID(cred), data)
	}
	a.startSession(w, r, http.StatusOK, username)
}
