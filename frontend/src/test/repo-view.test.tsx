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
      blob: vi.fn(),
      blame: vi.fn(),
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

function renderRepo(owner: string, rest = "") {
  return render(
    <I18nProvider>
      <MemoryRouter initialEntries={[`/repo/${owner}/demo${rest}`]}>
        <Routes>
          <Route path="/repo/:owner/:name" element={<RepoView />} />
          <Route path="/repo/:owner/:name/*" element={<RepoView />} />
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
    (api.getRepo as Mock).mockResolvedValue(repo({ role: "write" }));
    renderRepo("alice");
    await waitFor(() => expect(screen.getAllByText("a test repo").length).toBeGreaterThan(0));
    expect(screen.queryByText("Settings")).not.toBeInTheDocument();
  });

  it("owner 显示 settings 标签", async () => {
    (api.me as Mock).mockResolvedValue({ username: "alice" });
    (api.getRepo as Mock).mockResolvedValue(repo({ role: "owner" }));
    renderRepo("alice");
    await waitFor(() => expect(screen.getByText("Settings")).toBeInTheDocument());
  });

  it("组织仓库：组织 owner 显示 settings 标签", async () => {
    // 组织仓库的 owner 是组织名，me !== owner；权限应依据后端返回的 role。
    (api.me as Mock).mockResolvedValue({ username: "alice" });
    (api.getRepo as Mock).mockResolvedValue(
      repo({ owner: "acme", role: "owner" }),
    );
    renderRepo("acme");
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

  it("路径化 blob 路由按 ref/path 加载文件", async () => {
    (api.me as Mock).mockResolvedValue({ username: "bob" });
    (api.getRepo as Mock).mockResolvedValue(repo());
    (api.blob as Mock).mockResolvedValue({
      path: "docs/x.md",
      encoding: "utf-8",
      content: "# Hi",
      size: 4,
    });
    renderRepo("alice", "/blob/docs/x.md?ref=main");
    await waitFor(() =>
      expect(api.blob).toHaveBeenCalledWith("alice", "demo", "main", "docs/x.md"),
    );
  });

  it("旧查询参数 blob URL 自动重定向到路径化路由", async () => {
    (api.me as Mock).mockResolvedValue({ username: "bob" });
    (api.getRepo as Mock).mockResolvedValue(repo());
    (api.blob as Mock).mockResolvedValue({
      path: "docs/x.md",
      encoding: "utf-8",
      content: "# Hi",
      size: 4,
    });
    renderRepo("alice", "?file=docs/x.md&ref=main");
    await waitFor(() =>
      expect(api.blob).toHaveBeenCalledWith("alice", "demo", "main", "docs/x.md"),
    );
  });

  it("子目录中的文件在目录树加载完成后仍保持可见", async () => {
    // 回归：打开子目录中的文件时 currentDir 变为 ""，会触发按根目录重新取树；
    // 该请求若晚于 blob 返回，不得清空已加载的文件内容（否则深层文件看不到了）。
    (api.me as Mock).mockResolvedValue({ username: "bob" });
    (api.getRepo as Mock).mockResolvedValue(repo());
    (api.branches as Mock).mockResolvedValue([{ name: "main", is_head: true }]);
    (api.blob as Mock).mockResolvedValue({
      path: "src/a/b/c/deep.md",
      encoding: "utf-8",
      content: "# Deep Heading",
      size: 16,
    });
    // 目录树请求晚于 blob 返回，构造出会覆盖 blob 的竞态。
    (api.tree as Mock).mockImplementation(
      () => new Promise((resolve) => setTimeout(() => resolve({ entries: [] }), 30)),
    );

    renderRepo("alice", "/blob/src/a/b/c/deep.md?ref=main");

    expect(await screen.findByRole("heading", { name: "Deep Heading" })).toBeInTheDocument();
    // 等待目录树请求返回后，文件内容仍应保留。
    await new Promise((r) => setTimeout(r, 60));
    expect(screen.getByRole("heading", { name: "Deep Heading" })).toBeInTheDocument();
  });
});
