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
    },
  };
});

const { api } = (await import("@/lib/api")) as unknown as {
  api: { authProviders: ReturnType<typeof vi.fn>; version: ReturnType<typeof vi.fn> };
};

function renderLogin() {
  return render(
    <ThemeProvider>
      <I18nProvider>
        <MemoryRouter initialEntries={["/"]}>
          <Login onAuthed={() => undefined} />
        </MemoryRouter>
      </I18nProvider>
    </ThemeProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  api.version.mockResolvedValue({ version: "0.0.0-test" });
});

describe("Login providers", () => {
  it("provider 接口成功时展示 OAuth 按钮", async () => {
    api.authProviders.mockResolvedValue({
      github: { enabled: true },
      google: { enabled: false },
      oidc: { enabled: false, name: "OIDC" },
    });
    renderLogin();
    await waitFor(() => expect(screen.getByText("Continue with GitHub")).toBeTruthy());
  });

  // 回归 issue #1：providers 请求失败不再静默吞掉，给出提示与重试入口
  it("provider 接口失败时提示并可重试", async () => {
    api.authProviders.mockRejectedValueOnce(new Error("boom"));
    renderLogin();

    await waitFor(() => expect(screen.getByText("Failed to load sign-in providers")).toBeTruthy());

    api.authProviders.mockResolvedValueOnce({ github: { enabled: true } });
    await userEvent.click(screen.getByRole("button", { name: "Retry" }));

    await waitFor(() => expect(screen.getByText("Continue with GitHub")).toBeTruthy());
    expect(screen.queryByText("Failed to load sign-in providers")).toBeNull();
  });
});
