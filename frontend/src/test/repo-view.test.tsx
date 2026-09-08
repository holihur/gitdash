import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { I18nProvider } from "@/lib/i18n";
import RepoView from "@/pages/RepoView";
import { vi, type Mock } from "vitest";

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
    cloneCommand: (owner: string, name: string) => `git clone ssh://x/${owner}/${name}.git`,
    cloneUrl: (owner: string, name: string) => `ssh://x/${owner}/${name}.git`,
    api: {
      me: vi.fn(),
      getRepo: vi.fn(),
      branches: vi.fn().mockResolvedValue([]),
      listTags: vi.fn().mockResolvedValue([]),
      tree: vi.fn().mockResolvedValue({ entries: [] }),
    },
  };
});

const { api, ApiError } = (await import("@/lib/api")) as typeof import("@/lib/api");

type Repo = Parameters<typeof api.getRepo>[0] extends never ? never : Record<string, unknown> & {
  id: number;
  owner: string;
  name: string;
  description: string;
  created_at: string;
};

const repo = (over: Partial<Repo> = {}): Repo => ({
  id: 1,
  owner: "alice",
  name: "demo",
  description: "a test repo",
  created_at: "2026-01-01T00:00:00Z",
  ...over,
});

function renderRepo(owner: string) {
  return render(
    <I18nProvider>
      <MemoryRouter initialEntries={[`/${owner}/demo`]}>
        <Routes>
          <Route path="/:owner/:name" element={<RepoView />} />
        </Routes>
      </MemoryRouter>
    </I18nProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("RepoView", () => {
  it("加载成功：展示仓库名与描述", async () => {
    (api.me as Mock).mockResolvedValue({ username: "bob" });
    (api.getRepo as Mock).mockResolvedValue(repo());
    renderRepo("alice");
    await waitFor(() => expect(screen.getAllByText("a test repo").length).toBeGreaterThan(0));
    expect(document.querySelector("h1")?.textContent).toContain("demo");
  });

  it("非 owner 不显示 settings 标签", async () => {
    (api.me as Mock).mockResolvedValue({ username: "bob" });
    (api.getRepo as Mock).mockResolvedValue(repo());
    renderRepo("alice");
    await waitFor(() => expect(screen.getAllByText("a test repo").length).toBeGreaterThan(0));
    expect(screen.queryByText("Settings")).not.toBeInTheDocument();
  });

  it("owner 显示 settings 标签", async () => {
    (api.me as Mock).mockResolvedValue({ username: "alice" });
    (api.getRepo as Mock).mockResolvedValue(repo());
    renderRepo("alice");
    await waitFor(() => expect(screen.getByText("Settings")).toBeInTheDocument());
  });

  it("仓库不存在：展示 not found 文案", async () => {
    (api.me as Mock).mockResolvedValue({ username: "bob" });
    (api.getRepo as Mock).mockRejectedValue(new ApiError(404, "repo not found", "repo_not_found"));
    renderRepo("nobody");
    await waitFor(() =>
      expect(screen.getByText("Repository not found: repo not found")).toBeInTheDocument(),
    );
  });
});
