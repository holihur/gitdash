import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "@/lib/i18n";
import CommitsTab from "@/pages/repoview/commits-tab";
import { vi, type Mock } from "vitest";

vi.mock("@/lib/api", () => ({
  ApiError: class ApiError extends Error {},
  api: {
    commits: vi.fn(),
    commitDiff: vi.fn(),
    commitInfo: vi.fn(),
    revertCommit: vi.fn(),
  },
}));

const { api } = (await import("@/lib/api")) as unknown as {
  api: { commits: Mock; commitDiff: Mock; commitInfo: Mock; revertCommit: Mock };
};

const commits = [
  { sha: "a".repeat(40), author: "alice", date: "2026-01-02T00:00:00Z", message: "edit file" },
  { sha: "b".repeat(40), author: "alice", date: "2026-01-01T00:00:00Z", message: "add file" },
];

function renderTab(role: string, initialEntry = "/commits") {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <I18nProvider>
        <CommitsTab owner="alice" name="demo" refName="main" emptyRepo={false} role={role} />
      </I18nProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  api.commits.mockResolvedValue(commits);
});

describe("CommitsTab revert", () => {
  it("write 权限显示撤销按钮并调用 API", async () => {
    const user = userEvent.setup();
    api.revertCommit.mockResolvedValue({ sha: "c".repeat(40), branch: "main" });
    renderTab("owner");
    await waitFor(() => expect(screen.getByText("edit file")).toBeInTheDocument());

    const row = screen.getByText("edit file").closest("tr") as HTMLElement;
    await user.click(within(row).getByRole("button", { name: /revert/i }));

    const dialog = await screen.findByRole("alertdialog");
    expect(within(dialog).getByText(/Create a new commit on main/)).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: /revert/i }));

    await waitFor(() =>
      expect(api.revertCommit).toHaveBeenCalledWith(
        "alice",
        "demo",
        "a".repeat(40),
        "main",
      ),
    );
  });

  it("read 权限不显示撤销按钮", async () => {
    renderTab("read");
    await waitFor(() => expect(screen.getByText("edit file")).toBeInTheDocument());
    expect(screen.queryByRole("button", { name: /revert/i })).not.toBeInTheDocument();
  });
});

describe("CommitsTab pagination", () => {
  it("加载更多会追加下一页提交", async () => {
    const user = userEvent.setup();
    const firstPage = Array.from({ length: 30 }, (_, i) => ({
      sha: String(i).padStart(40, "0"),
      author: "alice",
      date: "2026-01-02T00:00:00Z",
      message: `page1-${i}`,
    }));
    const secondPage = [
      {
        sha: "f".repeat(40),
        author: "alice",
        date: "2025-01-01T00:00:00Z",
        message: "older commit",
      },
    ];
    api.commits.mockReset();
    api.commits.mockResolvedValueOnce(firstPage).mockResolvedValueOnce(secondPage);

    renderTab("read");
    await waitFor(() => expect(screen.getByText("page1-0")).toBeInTheDocument());
    expect(api.commits).toHaveBeenLastCalledWith("alice", "demo", "main", undefined, 30, 0);

    await user.click(screen.getByRole("button", { name: /load more/i }));
    await waitFor(() => expect(screen.getByText("older commit")).toBeInTheDocument());
    expect(api.commits).toHaveBeenLastCalledWith("alice", "demo", "main", undefined, 30, 30);
  });

  it("不足一页时不显示加载更多", async () => {
    renderTab("read");
    await waitFor(() => expect(screen.getByText("edit file")).toBeInTheDocument());
    expect(screen.queryByRole("button", { name: /load more/i })).not.toBeInTheDocument();
  });
});

describe("CommitsTab blame deep-link", () => {
  const sha = "c".repeat(40);

  beforeEach(() => {
    api.commitInfo.mockResolvedValue({
      sha,
      author: "zoe",
      date: "2026-01-03T00:00:00Z",
      message: "focused commit message",
    });
    api.commitDiff.mockResolvedValue({
      files: [{ path: "x.ts", status: "M", insertions: 1, deletions: 0 }],
      patch: "",
      base_sha: "",
      head_sha: sha,
    });
  });

  it("?commit= 自动展开并展示提交元数据", async () => {
    renderTab("read", `/commits?commit=${sha}`);

    await waitFor(() => expect(api.commitInfo).toHaveBeenCalledWith("alice", "demo", sha));
    expect(api.commitDiff).toHaveBeenCalledWith("alice", "demo", sha);
    expect(await screen.findByText("focused commit message")).toBeInTheDocument();
    expect(screen.getByText("zoe")).toBeInTheDocument();
  });

  it("关闭按钮收起聚焦的提交", async () => {
    const user = userEvent.setup();
    renderTab("read", `/commits?commit=${sha}`);

    await screen.findByText("focused commit message");
    await user.click(screen.getByRole("button", { name: /close/i }));
    await waitFor(() =>
      expect(screen.queryByText("focused commit message")).not.toBeInTheDocument(),
    );
  });

  it("无 commit 参数时不请求单提交接口", async () => {
    renderTab("read");
    await waitFor(() => expect(screen.getByText("edit file")).toBeInTheDocument());
    expect(api.commitInfo).not.toHaveBeenCalled();
  });
});
