import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "@/lib/i18n";
import RepoPulls from "@/pages/RepoPulls";
import { vi, type Mock } from "vitest";

vi.mock("@/lib/api", () => ({
  ApiError: class ApiError extends Error {},
  api: {
    listPulls: vi.fn(),
    branches: vi.fn(),
    pullDiff: vi.fn(),
    listPullReviews: vi.fn(),
    pullCodeowners: vi.fn(),
    listComments: vi.fn(),
    me: vi.fn(),
    createPull: vi.fn(),
    mergePull: vi.fn(),
    setPullState: vi.fn(),
    setPullDraft: vi.fn(),
    setPullAutoMerge: vi.fn(),
    dequeuePull: vi.fn(),
    applySuggestion: vi.fn(),
    postComment: vi.fn(),
    deleteComment: vi.fn(),
  },
}));

const { api } = (await import("@/lib/api")) as unknown as {
  api: Record<string, Mock>;
};

const pr = {
  id: 1,
  number: 7,
  title: "add feature",
  body: "hello",
  source_branch: "feat",
  target_branch: "main",
  base_sha: "a".repeat(40),
  head_sha: "b".repeat(40),
  state: "open",
  draft: false,
  auto_merge: false,
  author: "alice",
  created_at: "2026-01-02T00:00:00Z",
  updated_at: "2026-01-02T00:00:00Z",
  merged_at: null,
  merged_by: "",
};

function renderPage() {
  return render(
    <I18nProvider>
      <MemoryRouter>
        <RepoPulls owner="alice" name="demo" role="owner" />
      </MemoryRouter>
    </I18nProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  api.listPulls.mockResolvedValue({ items: [pr], total: 1 });
  api.branches.mockResolvedValue([]);
  api.pullDiff.mockResolvedValue({
    files: [{ path: "a.txt", status: "M", insertions: 1, deletions: 0 }],
    patch:
      "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1,1 +1,2 @@\n hello\n+world\n",
  });
  api.listPullReviews.mockResolvedValue({
    reviews: [
      {
        id: 1,
        reviewer: "bob",
        state: "approve",
        body: "lgtm",
        commit_sha: "b".repeat(40),
        created_at: "2026-01-02T00:00:00Z",
      },
    ],
    summary: { approvals: 1, request_changes: 0 },
  });
  api.pullCodeowners.mockResolvedValue({ owners: [], missing: [], satisfied: true });
  api.listComments.mockResolvedValue([]);
  api.me.mockResolvedValue({ username: "alice" });
});

describe("RepoPulls detail", () => {
  it("展开 PR 不应抛错", async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => expect(screen.getByText("add feature")).toBeInTheDocument());
    await user.click(screen.getByText("add feature"));
    await waitFor(() => expect(screen.getByText("Reviews")).toBeInTheDocument());
    expect(screen.getAllByText("hello").length).toBeGreaterThan(0);
  });
});

describe("RepoPulls detail hardening", () => {
  it("字段缺失/为 null 时不应白屏", async () => {
    const user = userEvent.setup();
    api.listPulls.mockResolvedValue({ items: [{ ...pr, body: null }], total: 1 });
    api.pullDiff.mockResolvedValue({ files: null, patch: undefined });
    api.listPullReviews.mockResolvedValue({});
    api.pullCodeowners.mockResolvedValue({ owners: null, missing: null, satisfied: false });
    api.listComments.mockResolvedValue(null);

    renderPage();
    await waitFor(() => expect(screen.getByText("add feature")).toBeInTheDocument());
    await user.click(screen.getByText("add feature"));
    // 详情区应正常渲染（Reviews 标题存在），且未抛错
    await waitFor(() => expect(screen.getByText("Reviews")).toBeInTheDocument());
  });
});
