// WebAuthn / Passkey 浏览器侧辅助：JSON（base64url）↔ ArrayBuffer 互转，
// 以及注册 / 登录流程封装。仅在安全上下文（HTTPS 或 localhost）可用。
import type {
  PasskeyBegin,
  PasskeyCreationOptionsJSON,
  PasskeyRequestOptionsJSON,
} from "@/lib/api/types";

export function base64urlToBuffer(value: string): ArrayBuffer {
  const pad = value.length % 4 === 0 ? "" : "=".repeat(4 - (value.length % 4));
  const base64 = (value + pad).replace(/-/g, "+").replace(/_/g, "/");
  const raw = atob(base64);
  const buf = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) buf[i] = raw.charCodeAt(i);
  return buf.buffer;
}

export function bufferToBase64url(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf);
  let raw = "";
  for (let i = 0; i < bytes.length; i++) raw += String.fromCharCode(bytes[i]);
  return btoa(raw).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

/** passkey 是否在当前浏览器可用（安全上下文 + WebAuthn API）。 */
export function passkeySupported(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.PublicKeyCredential !== "undefined" &&
    typeof navigator !== "undefined" &&
    typeof navigator.credentials !== "undefined"
  );
}

function toCreationOptions(json: PasskeyCreationOptionsJSON): PublicKeyCredentialCreationOptions {
  const pk = json.publicKey;
  return {
    challenge: base64urlToBuffer(pk.challenge),
    rp: pk.rp,
    user: {
      id: base64urlToBuffer(pk.user.id),
      name: pk.user.name,
      displayName: pk.user.displayName,
    },
    pubKeyCredParams: pk.pubKeyCredParams as PublicKeyCredentialParameters[],
    timeout: pk.timeout,
    attestation: pk.attestation as AttestationConveyancePreference | undefined,
    authenticatorSelection: pk.authenticatorSelection as AuthenticatorSelectionCriteria | undefined,
    excludeCredentials: pk.excludeCredentials?.map((c) => ({
      id: base64urlToBuffer(c.id),
      type: c.type as PublicKeyCredentialType,
      transports: c.transports as AuthenticatorTransport[] | undefined,
    })),
    extensions: pk.extensions as AuthenticationExtensionsClientInputs | undefined,
  };
}

function toRequestOptions(json: PasskeyRequestOptionsJSON): PublicKeyCredentialRequestOptions {
  const pk = json.publicKey;
  return {
    challenge: base64urlToBuffer(pk.challenge),
    rpId: pk.rpId,
    timeout: pk.timeout,
    userVerification: pk.userVerification as UserVerificationRequirement | undefined,
    allowCredentials: pk.allowCredentials?.map((c) => ({
      id: base64urlToBuffer(c.id),
      type: c.type as PublicKeyCredentialType,
      transports: c.transports as AuthenticatorTransport[] | undefined,
    })),
    extensions: pk.extensions as AuthenticationExtensionsClientInputs | undefined,
  };
}

/** 将 PublicKeyCredential 序列化为服务端可解析的 JSON（base64url 字段）。 */
export function creationCredentialToJSON(cred: PublicKeyCredential): Record<string, unknown> {
  const resp = cred.response as AuthenticatorAttestationResponse;
  const transports = typeof resp.getTransports === "function" ? resp.getTransports() : undefined;
  return {
    id: cred.id,
    rawId: bufferToBase64url(cred.rawId),
    type: cred.type,
    response: {
      attestationObject: bufferToBase64url(resp.attestationObject),
      clientDataJSON: bufferToBase64url(resp.clientDataJSON),
      transports,
    },
    clientExtensionResults: cred.getClientExtensionResults(),
  };
}

/** 将登录断言序列化为服务端可解析的 JSON。 */
export function assertionCredentialToJSON(cred: PublicKeyCredential): Record<string, unknown> {
  const resp = cred.response as AuthenticatorAssertionResponse;
  return {
    id: cred.id,
    rawId: bufferToBase64url(cred.rawId),
    type: cred.type,
    response: {
      authenticatorData: bufferToBase64url(resp.authenticatorData),
      clientDataJSON: bufferToBase64url(resp.clientDataJSON),
      signature: bufferToBase64url(resp.signature),
      userHandle: resp.userHandle ? bufferToBase64url(resp.userHandle) : undefined,
    },
    clientExtensionResults: cred.getClientExtensionResults(),
  };
}

/** 执行注册 ceremony，返回可提交给服务端的 credential JSON。 */
export async function createPasskey(
  begin: PasskeyBegin<PasskeyCreationOptionsJSON>,
): Promise<Record<string, unknown>> {
  const cred = (await navigator.credentials.create({
    publicKey: toCreationOptions(begin.public_key),
  })) as PublicKeyCredential | null;
  if (!cred) throw new Error("passkey registration was cancelled");
  return creationCredentialToJSON(cred);
}

/** 执行登录 ceremony，返回可提交给服务端的 assertion JSON。 */
export async function getPasskeyAssertion(
  begin: PasskeyBegin<PasskeyRequestOptionsJSON>,
): Promise<Record<string, unknown>> {
  const cred = (await navigator.credentials.get({
    publicKey: toRequestOptions(begin.public_key),
  })) as PublicKeyCredential | null;
  if (!cred) throw new Error("passkey sign-in was cancelled");
  return assertionCredentialToJSON(cred);
}
