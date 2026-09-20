import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "@/lib/i18n";
import { ThemeProvider } from "@/lib/theme";
import Login from "@/pages/Login";

vi.mock("@/lib/api", () => {
  class ApiError extends Error {
    status: number;
    code?: string;
    constructor(status: number, message: string, code?: string) {
      super(message);
      this.name = "ApiError";
      this.status = status;
      this.code = code;
    }
  }
  return {
    ApiError,
    api: {
      authProviders: vi.fn(),
      version: vi.fn(),
      forgotPassword: vi.fn(),
      resetPassword: vi.fn(),
    },
  };
});

const { api } = (await import("@/lib/api")) as unknown as {
  api: {
    authProviders: ReturnType<typeof vi.fn>;
    version: ReturnType<typeof vi.fn>;
    forgotPassword: ReturnType<typeof vi.fn>;
    resetPassword: ReturnType<typeof vi.fn>;
  };
};

function renderLogin(entry = "/") {
  return render(
    <ThemeProvider>
      <I18nProvider>
        <MemoryRouter initialEntries={[entry]}>
          <Login onAuthed={() => undefined} />
        </MemoryRouter>
      </I18nProvider>
    </ThemeProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  api.version.mockResolvedValue({ version: "0.0.0-test" });
  api.forgotPassword.mockResolvedValue({ sent: true });
  api.resetPassword.mockResolvedValue({ username: "alice", reset: true });
});

describe("Login password reset", () => {
  it("SMTP 可用时展示忘记密码入口并提交邮箱", async () => {
    api.authProviders.mockResolvedValue({ password_reset: { enabled: true } });
    renderLogin();

    const link = await screen.findByText("Forgot password?");
    await userEvent.click(link);

    expect(await screen.findByText("Reset your password")).toBeTruthy();
    await userEvent.type(screen.getByLabelText("Email"), "alice@example.com");
    await userEvent.click(screen.getByRole("button", { name: "Send reset link" }));

    await waitFor(() => expect(api.forgotPassword).toHaveBeenCalledWith("alice@example.com"));
  });

  it("SMTP 不可用时不展示忘记密码入口", async () => {
    api.authProviders.mockResolvedValue({ password_reset: { enabled: false } });
    renderLogin();
    await waitFor(() => expect(api.authProviders).toHaveBeenCalled());
    expect(screen.queryByText("Forgot password?")).toBeNull();
  });

  it("携带 reset_password token 时展示重置表单并提交", async () => {
    api.authProviders.mockResolvedValue({ password_reset: { enabled: true } });
    renderLogin("/?reset_password=tok-123");

    expect(await screen.findByText("Choose a new password")).toBeTruthy();
    await userEvent.type(screen.getByLabelText("New password"), "New-pass-123456");
    await userEvent.type(screen.getByLabelText("Confirm password"), "New-pass-123456");
    await userEvent.click(screen.getByRole("button", { name: "Reset password" }));

    await waitFor(() =>
      expect(api.resetPassword).toHaveBeenCalledWith("tok-123", "New-pass-123456"),
    );
  });
});
