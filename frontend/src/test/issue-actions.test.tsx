import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "@/lib/i18n";
import { IssueActions } from "@/components/issues/issue-actions";
import type { Issue } from "@/lib/api";
import { vi, type Mock } from "vitest";

vi.mock("@/lib/api", () => ({
  ApiError: class ApiError extends Error {},
  api: {
    setIssueState: vi.fn(),
    updateIssue: vi.fn(),
    deleteIssue: vi.fn(),
    listByok: vi.fn(),
    createCopilot: vi.fn(),
  },
}));

const { api } = (await import("@/lib/api")) as unknown as {
  api: {
    setIssueState: Mock;
    updateIssue: Mock;
    deleteIssue: Mock;
    listByok: Mock;
    createCopilot: Mock;
  };
};

function makeIssue(state: "open" | "closed"): Issue {
  return {
    id: 1,
    number: 3,
    title: "bug",
    body: "body",
    state,
    author: "alice",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    closed_at: state === "closed" ? "2026-01-02T00:00:00Z" : null,
  };
}

function renderActions(issue: Issue) {
  const onChanged = vi.fn();
  render(
    <MemoryRouter>
      <I18nProvider>
        <IssueActions owner="alice" name="demo" issue={issue} canWrite canTriage onChanged={onChanged} />
      </I18nProvider>
    </MemoryRouter>,
  );
  return { onChanged };
}

beforeEach(() => {
  vi.clearAllMocks();
  api.updateIssue.mockResolvedValue(makeIssue("closed"));
  api.setIssueState.mockResolvedValue(makeIssue("open"));
});

describe("IssueActions close with comment", () => {
  it("关闭时弹出对话框并提交评论与原因", async () => {
    const user = userEvent.setup();
    const { onChanged } = renderActions(makeIssue("open"));

    await user.click(screen.getByRole("button", { name: "Close" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Close with comment")).toBeInTheDocument();

    await user.type(
      screen.getByPlaceholderText("Leave a comment (optional)"),
      "fixed in #12",
    );
    await user.selectOptions(within(dialog).getByLabelText("Close reason"), "not_planned");
    await user.click(within(dialog).getByTestId("confirm-close"));

    await waitFor(() =>
      expect(api.updateIssue).toHaveBeenCalledWith("alice", "demo", 3, {
        state: "closed",
        comment: "fixed in #12",
        state_reason: "not_planned",
      }),
    );
    expect(onChanged).toHaveBeenCalled();
  });

  it("重开已关闭 issue 直接调用状态接口", async () => {
    const user = userEvent.setup();
    renderActions(makeIssue("closed"));

    await user.click(screen.getByRole("button", { name: "Reopen" }));
    await waitFor(() =>
      expect(api.setIssueState).toHaveBeenCalledWith("alice", "demo", 3, "open"),
    );
    expect(api.updateIssue).not.toHaveBeenCalled();
  });
});
