import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "@/lib/i18n";
import CommentSection from "@/components/comment-section";
import { vi, type Mock } from "vitest";

vi.mock("@/lib/api", () => ({
  ApiError: class ApiError extends Error {},
  api: {
    listComments: vi.fn(),
    postComment: vi.fn(),
    updateComment: vi.fn(),
    deleteComment: vi.fn(),
    me: vi.fn(),
  },
}));
vi.mock("@/components/markdown", () => ({
  MarkdownView: ({ text }: { text: string }) => <div data-testid="md">{text}</div>,
}));

const { api } = (await import("@/lib/api")) as unknown as {
  api: {
    listComments: Mock;
    postComment: Mock;
    updateComment: Mock;
    deleteComment: Mock;
    me: Mock;
  };
};

const comment = {
  id: 1,
  number: 1,
  author: "me",
  body: "old body",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

function renderSection() {
  render(
    <I18nProvider>
      <CommentSection owner="alice" name="demo" number={1} />
    </I18nProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  api.listComments.mockResolvedValue([comment]);
  api.me.mockResolvedValue({ username: "me" });
});

describe("CommentSection editing", () => {
  it("作者可编辑评论并提交新正文", async () => {
    const user = userEvent.setup();
    api.updateComment.mockResolvedValue({ ...comment, body: "new body" });
    renderSection();

    await screen.findByTestId("md");
    await user.click(await screen.findByTitle("Edit comment"));

    const textarea = screen.getByDisplayValue("old body");
    await user.clear(textarea);
    await user.type(textarea, "new body");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(api.updateComment).toHaveBeenCalledWith("alice", "demo", 1, "new body"),
    );
    expect(api.listComments).toHaveBeenCalledTimes(2);
  });

  it("非作者看不到编辑 / 删除按钮", async () => {
    api.me.mockResolvedValue({ username: "other" });
    renderSection();

    await screen.findByTestId("md");
    expect(screen.queryByTitle("Edit comment")).not.toBeInTheDocument();
    expect(screen.queryByTitle("Delete comment")).not.toBeInTheDocument();
  });
});
