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
      login: vi.fn(),
      register: vi.fn(),
      authProviders: vi.fn(),
      version: vi.fn(),
    },
  };
});

const { api } = (await import("@/lib/api")) as unknown as {
  api: Record<string, ReturnType<typeof vi.fn>>;
};

beforeEach(() => {
  vi.clearAllMocks();
  api.version.mockResolvedValue({ version: "0.0.0-test" });
  api.authProviders.mockResolvedValue({});
});

describe("Login session", () => {
  // 回归：浏览器请求下服务端只回 {username}（token 走 httpOnly cookie），
  // 前端不能因缺少 token 而静默不登录。
  it("登录响应没有 token 时也进入已登录状态", async () => {
    api.login.mockResolvedValue({ username: "alice" }); // 无 token
    const onAuthed = vi.fn();
    render(
      <ThemeProvider>
        <I18nProvider>
          <MemoryRouter initialEntries={["/"]}>
            <Login onAuthed={onAuthed} />
          </MemoryRouter>
        </I18nProvider>
      </ThemeProvider>,
    );

    await userEvent.type(await screen.findByLabelText("Username"), "alice");
    await userEvent.type(screen.getByLabelText("Password"), "alice-pass-123");
    const submit = screen.getAllByRole("button", { name: "Sign in" }).slice(-1)[0];
    await userEvent.click(submit);

    await waitFor(() => expect(onAuthed).toHaveBeenCalledWith("alice"));
  });
});
