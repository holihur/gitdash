import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "@/lib/i18n";
import CommitsTab from "@/pages/repoview/commits-tab";
import { vi, type Mock } from "vitest";

vi.mock("@/lib/api", () => ({
  ApiError: class ApiError extends Error {},
  api: {
    commits: vi.fn(),
    commitDiff: vi.fn(),
    revertCommit: vi.fn(),
  },
}));

const { api } = (await import("@/lib/api")) as unknown as {
  api: { commits: Mock; commitDiff: Mock; revertCommit: Mock };
};

const commits = [
  { sha: "a".repeat(40), author: "alice", date: "2026-01-02T00:00:00Z", message: "edit file" },
  { sha: "b".repeat(40), author: "alice", date: "2026-01-01T00:00:00Z", message: "add file" },
];

function renderTab(role: string) {
  return render(
    <I18nProvider>
      <CommitsTab owner="alice" name="demo" refName="main" emptyRepo={false} role={role} />
    </I18nProvider>,
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
