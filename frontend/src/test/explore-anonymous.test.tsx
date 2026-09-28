import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "@/lib/i18n";
import { ThemeProvider } from "@/lib/theme";
import Explore from "@/pages/Explore";
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
      listExplore: vi.fn(),
      listTopics: vi.fn(),
      getBadges: vi.fn(),
      globalSearch: vi.fn(),
      searchCode: vi.fn(),
      star: vi.fn(),
      unstar: vi.fn(),
      authProviders: vi.fn(),
      version: vi.fn(),
    },
  };
});

const { api } = (await import("@/lib/api")) as unknown as {
  api: Record<string, ReturnType<typeof vi.fn>>;
};

const REPO = {
  id: 1,
  owner: "alice",
  name: "demo",
  description: "demo repo",
  private: false,
  default_branch: "main",
  created_at: "2026-01-01T00:00:00Z",
  stars: 3,
  starred: false,
};

beforeEach(() => {
  vi.clearAllMocks();
  api.listExplore.mockResolvedValue({ items: [REPO], total: 1 });
  api.listTopics.mockResolvedValue([]);
  api.getBadges.mockResolvedValue([]);
});

function renderExplore(anonymous: boolean) {
  return render(
    <ThemeProvider>
      <I18nProvider>
        <MemoryRouter initialEntries={["/explore"]}>
          <Explore anonymous={anonymous} />
        </MemoryRouter>
      </I18nProvider>
    </ThemeProvider>,
  );
}

describe("anonymous explore", () => {
  it("explore 列表匿名可读，但隐藏点赞按钮", async () => {
    renderExplore(true);
    await waitFor(() => expect(screen.getByText("alice/demo")).toBeTruthy());
    // 匿名下点赞按钮不渲染（点击会 401）
    expect(screen.queryByRole("button", { name: "3" })).toBeNull();
  });

  it("登录用户在 explore 列表可见点赞按钮", async () => {
    renderExplore(false);
    await waitFor(() => expect(screen.getByText("alice/demo")).toBeTruthy());
    expect(screen.getByRole("button", { name: "3" })).toBeTruthy();
  });

  it("登录页提供匿名 Explore 入口", async () => {
    api.version.mockResolvedValue({ version: "0.0.0-test" });
    api.authProviders.mockResolvedValue({});
    render(
      <ThemeProvider>
        <I18nProvider>
          <MemoryRouter initialEntries={["/"]}>
            <Login onAuthed={() => undefined} />
          </MemoryRouter>
        </I18nProvider>
      </ThemeProvider>,
    );
    const link = await screen.findByRole("link", { name: "Explore" });
    expect(link.getAttribute("href")).toBe("/explore");
  });
});
