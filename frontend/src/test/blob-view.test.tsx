import { createRef } from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "@/lib/i18n";
import BlobView from "@/pages/repoview/blob-view";
import type { Blame, Blob } from "@/lib/api";
import { vi, type Mock } from "vitest";

vi.mock("@/lib/api", () => ({
  ApiError: class ApiError extends Error {},
  api: { rawFileUrl: vi.fn(() => "/raw/file") },
}));
vi.mock("@/components/code-editor-lazy", () => ({
  default: ({ value }: { value: string }) => <pre data-testid="codemirror">{value}</pre>,
}));
vi.mock("@/components/markdown", () => ({
  MarkdownView: ({ text }: { text: string }) => <div data-testid="markdown">{text}</div>,
}));

const { api } = (await import("@/lib/api")) as unknown as {
  api: { rawFileUrl: Mock };
};

function makeBlob(path: string, over: Partial<Blob> = {}): Blob {
  return { path, size: 5, encoding: "utf-8", content: "hello", ...over };
}

function renderView(blob: Blob, extra: Record<string, unknown> = {}) {
  const handlers = {
    onToggleBlame: vi.fn(),
    onEdit: vi.fn(),
    onRename: vi.fn(),
    onDelete: vi.fn(),
    onOpenRepoLink: vi.fn(),
  };
  render(
    <I18nProvider>
      <BlobView
        owner="alice"
        name="demo"
        refName="main"
        blob={blob}
        blame={null}
        blameParam={false}
        codeHostRef={createRef<HTMLDivElement>()}
        {...handlers}
        {...extra}
      />
    </I18nProvider>,
  );
  return handlers;
}

async function openMenu(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: "More actions" }));
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("BlobView", () => {
  it("代码文件用编辑器渲染内容", () => {
    renderView(makeBlob("src/main.go", { content: "package main" }));
    expect(screen.getByTestId("codemirror")).toHaveTextContent("package main");
    // 展示大小徽标
    expect(screen.getByText("5 B")).toBeInTheDocument();
  });

  it("Markdown 文件走 Markdown 渲染", () => {
    renderView(makeBlob("README.md", { content: "# hi" }));
    expect(screen.getByTestId("markdown")).toHaveTextContent("# hi");
    expect(screen.queryByTestId("codemirror")).not.toBeInTheDocument();
  });

  it("图片使用 raw 地址内联预览", () => {
    api.rawFileUrl.mockReturnValue("/raw/pic.png");
    renderView(makeBlob("assets/pic.png", { encoding: "binary", content: "" }));
    const img = screen.getByRole("img");
    expect(img).toHaveAttribute("src", "/raw/pic.png");
    expect(api.rawFileUrl).toHaveBeenCalledWith("alice", "demo", "main", "assets/pic.png");
  });

  it("PDF 使用 iframe 内联预览", () => {
    renderView(makeBlob("doc.pdf", { encoding: "binary", content: "" }));
    expect(screen.getByTitle("doc.pdf")).toBeInTheDocument();
  });

  it("二进制文件给出不可预览提示", () => {
    renderView(makeBlob("bin.dat", { encoding: "binary", content: "" }));
    expect(screen.getByText("Binary file")).toBeInTheDocument();
    expect(
      screen.getByText("This file cannot be previewed in the browser."),
    ).toBeInTheDocument();
  });

  it("超大文件展示截断徽标", () => {
    renderView(makeBlob("big.txt", { encoding: "truncated" }));
    expect(screen.getByText("File too large, showing size only")).toBeInTheDocument();
  });

  it("blame 表格展示作者与行内容", () => {
    const blame: Blame = {
      path: "src/main.go",
      commits: { abc1234: { sha: "abc1234", author: "alice", date: "2026-01-01T00:00:00Z", message: "add" } },
      lines: [
        { line: 1, commit: "abc1234", content: "package main" },
        { line: 2, commit: "abc1234", content: "func main() {}" },
      ],
    };
    renderView(makeBlob("src/main.go"), { blameParam: true, blame });
    expect(screen.queryByTestId("codemirror")).not.toBeInTheDocument();
    expect(screen.getAllByText("alice")).toHaveLength(2);
    expect(screen.getByText("package main")).toBeInTheDocument();
    expect(screen.getAllByRole("link")[0]).toHaveAttribute(
      "href",
      "/repo/alice/demo/commits?commit=abc1234&ref=main",
    );
  });

  it("菜单可切换 blame / 编辑 / 重命名 / 删除", async () => {
    const user = userEvent.setup();
    const h = renderView(makeBlob("src/main.go"));

    await openMenu(user);
    await user.click(await screen.findByRole("menuitemcheckbox", { name: /blame/i }));
    expect(h.onToggleBlame).toHaveBeenCalled();

    await openMenu(user);
    await user.click(await screen.findByRole("menuitem", { name: /edit file/i }));
    expect(h.onEdit).toHaveBeenCalled();

    await openMenu(user);
    await user.click(await screen.findByRole("menuitem", { name: /rename/i }));
    expect(h.onRename).toHaveBeenCalled();

    await openMenu(user);
    await user.click(await screen.findByRole("menuitem", { name: /delete file/i }));
    expect(h.onDelete).toHaveBeenCalled();
  });

  it("二进制文件菜单不提供 blame / 编辑", async () => {
    const user = userEvent.setup();
    renderView(makeBlob("bin.dat", { encoding: "binary", content: "" }));
    await openMenu(user);
    const menu = await screen.findByRole("menu");
    expect(within(menu).queryByText(/blame/i)).not.toBeInTheDocument();
    expect(within(menu).queryByText(/edit file/i)).not.toBeInTheDocument();
    // rename / delete 仍然可用
    expect(within(menu).getByText(/rename/i)).toBeInTheDocument();
  });
});
