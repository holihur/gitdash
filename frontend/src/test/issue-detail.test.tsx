import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "@/lib/i18n";
import IssueDetail from "@/pages/repoview/issue-detail";
import { vi, type Mock } from "vitest";

vi.mock("@/lib/api", () => ({
  ApiError: class ApiError extends Error {},
  api: {
    getIssue: vi.fn(),
    listLabels: vi.fn(),
    listMilestones: vi.fn(),
    listIssueEvents: vi.fn(),
    listCollabs: vi.fn(),
    setIssueLabels: vi.fn(),
    setIssueMilestone: vi.fn(),
    setIssueAssignees: vi.fn(),
    subscribeIssue: vi.fn(),
    unsubscribeIssue: vi.fn(),
  },
}));
vi.mock("@/components/markdown", () => ({
  MarkdownView: ({ text }: { text: string }) => <div data-testid="md">{text}</div>,
}));
vi.mock("@/components/comment-section", () => ({
  default: ({ events }: { events?: { id: number }[] }) => (
    <div data-testid="comments" data-events={(events ?? []).length} />
  ),
}));
vi.mock("@/components/issues/issue-actions", () => ({
  IssueActions: () => <div data-testid="actions" />,
}));

const { api } = (await import("@/lib/api")) as unknown as {
  api: {
    getIssue: Mock;
    listLabels: Mock;
    listMilestones: Mock;
    listIssueEvents: Mock;
    listCollabs: Mock;
    setIssueLabels: Mock;
    setIssueMilestone: Mock;
    setIssueAssignees: Mock;
    subscribeIssue: Mock;
    unsubscribeIssue: Mock;
  };
};

const issue = {
  id: 1,
  number: 1,
  title: "bug",
  body: "body",
  state: "open" as const,
  author: "alice",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  closed_at: null,
  labels: [],
  milestone: null,
  assignees: ["bob"],
  comment_count: 1,
  subscribed: false,
  linked_pulls: [
    {
      id: 1,
      number: 5,
      title: "fix bug",
      body: "",
      source_branch: "fix",
      target_branch: "main",
      base_sha: "",
      head_sha: "",
      state: "open" as const,
      draft: false,
      auto_merge: false,
      author: "bob",
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      merged_at: null,
      merged_by: "",
    },
  ],
};

function renderDetail() {
  render(
    <MemoryRouter>
      <I18nProvider>
        <IssueDetail owner="alice" name="demo" role="owner" number={1} />
      </I18nProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  api.getIssue.mockResolvedValue(issue);
  api.listLabels.mockResolvedValue([]);
  api.listMilestones.mockResolvedValue([]);
  api.listIssueEvents.mockResolvedValue([
    { id: 1, kind: "issue", number: 1, actor: "alice", action: "labeled", detail: "bug", created_at: "2026-01-01T00:00:00Z" },
  ]);
  api.listCollabs.mockResolvedValue([
    { owner: "alice", repo: "demo", username: "carol", permission: "write", created_at: "2026-01-01T00:00:00Z" },
  ]);
  api.setIssueAssignees.mockResolvedValue(issue);
  api.setIssueLabels.mockResolvedValue(issue);
  api.setIssueMilestone.mockResolvedValue(issue);
  api.subscribeIssue.mockResolvedValue({ subscribed: true });
  api.unsubscribeIssue.mockResolvedValue({ subscribed: false });
});

describe("IssueDetail enhancements", () => {
  it("展示负责人选项、时间线与关联 PR", async () => {
    renderDetail();
    // owner + collab + 当前负责人
    expect(await screen.findByRole("button", { name: /carol/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /bob/ })).toBeInTheDocument();
    expect(screen.getByTestId("comments")).toHaveAttribute("data-events", "1");
    expect(screen.getByText("Linked pull requests")).toBeInTheDocument();
    expect(screen.getByText("#5")).toBeInTheDocument();
  });

  it("保存负责人变更调用接口", async () => {
    const user = userEvent.setup();
    renderDetail();
    await user.click(await screen.findByRole("button", { name: /carol/ }));
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(api.setIssueAssignees).toHaveBeenCalledWith("alice", "demo", 1, ["bob", "carol"]),
    );
  });

  it("订阅按钮调用订阅接口", async () => {
    const user = userEvent.setup();
    renderDetail();
    await user.click(await screen.findByRole("button", { name: /Subscribe/ }));
    await waitFor(() => expect(api.subscribeIssue).toHaveBeenCalledWith("alice", "demo", 1));
  });
});
