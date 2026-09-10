import { req } from "./core";
import type { LoginResult, MFAEnroll, MFAStatus, User } from "./types";

export const authApi = {
  // auth
  register: (username: string, password: string) =>
    req<{ token: string; username: string }>("/auth/register", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    }),
  login: (username: string, password: string) =>
    req<LoginResult>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    }),
  mfaVerify: (mfaToken: string, code: string) =>
    req<{ token: string; username: string }>("/auth/mfa-verify", {
      method: "POST",
      body: JSON.stringify({ mfa_token: mfaToken, code }),
    }),
  mfaEmailResend: (mfaToken: string) =>
    req<{ sent: boolean }>("/auth/mfa-email/resend", {
      method: "POST",
      body: JSON.stringify({ mfa_token: mfaToken }),
    }),
  logout: () => req<null>("/auth/logout", { method: "POST" }),
  me: () => req<User>("/me"),
  verifyEmail: (token: string) =>
    req<{ username: string; email_verified: boolean }>("/me/email/verify", {
      method: "POST",
      body: JSON.stringify({ token }),
    }),
  resendEmailVerification: () =>
    req<{ sent: boolean }>("/me/email/resend", { method: "POST" }),


  // profile
  changePassword: (currentPassword: string, newPassword: string) =>
    req<null>("/me/password", {
      method: "POST",
      body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
    }),
  updateProfile: (email: string) =>
    req<{ username: string; email: string }>("/me/profile", {
      method: "POST",
      body: JSON.stringify({ email }),
    }),
  mfaStatus: () => req<MFAStatus>("/me/mfa"),
  mfaEnroll: () => req<MFAEnroll>("/me/mfa/enroll", { method: "POST" }),
  mfaActivate: (code: string) =>
    req<null>("/me/mfa/activate", { method: "POST", body: JSON.stringify({ code }) }),
  mfaDisable: (password: string, code: string) =>
    req<null>("/me/mfa/disable", {
      method: "POST",
      body: JSON.stringify({ password, code }),
    }),
  mfaEmailEnroll: () =>
    req<{ sent: boolean }>("/me/mfa/email/enroll", { method: "POST" }),
  mfaEmailActivate: (code: string) =>
    req<null>("/me/mfa/email/activate", { method: "POST", body: JSON.stringify({ code }) }),
  mfaEmailSend: () => req<null>("/me/mfa/email/send", { method: "POST" }),
};
