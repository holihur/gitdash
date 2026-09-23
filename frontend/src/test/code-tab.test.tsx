import { render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode, Ref } from "react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "@/lib/i18n";
import CodeTab from "@/pages/repoview/code-tab";
import type { Blob, Repo } from "@/lib/api";
import { vi } from "vitest";

vi.mock("@/components/file-tree", () => ({
  default: ({ onOpenDir, onOpenFile }: { onOpenDir: (d: string) => void; onOpenFile: (f: string) => void }) => (
    <div data-testid="file-tree">
      <button onClick={() => onOpenDir("dir1")}>tree-dir</button>
      <button onClick={() => onOpenFile("f.ts")}>tree-file</button>
    </div>
  ),
}));
vi.mock("@/components/outline", () => ({
  default: ({ onJumpToLine }: { onJumpToLine: (n: number) => void }) => (
    <div data-testid="outline">
      <button onClick={() => onJumpToLine(42)}>outline-line</button>
    </div>
  ),
}));
vi.mock("@/components/language-bar", () => ({
  LanguageBar: () => <div data-testid="language-bar" />,
}));
vi.mock("@/pages/repoview/code-ref-bar", () => ({
  default: () => <div data-testid="code-ref-bar" />,
}));
vi.mock("@/pages/repoview/repo-code-body", () => ({
  RepoCodeBody: ({ codeHostRef }: { codeHostRef?: Ref<HTMLDivElement> }) => (
    <div data-testid="repo-code-body" ref={codeHostRef}>
      <div className="cm-content">
        {[1, 2, 3].map((n) => (
          <div key={n} className="cm-line">
            line {n}
          </div>
        ))}
      </div>
    </div>
  ),
}));
vi.mock("@/components/code-search", () => ({
  CodeSearch: ({ leading, actions }: { leading?: ReactNode; actions?: ReactNode }) => (
    <div data-testid="code-search">
      {leading}
      {actions}
    </div>
  ),
}));

const repo: Repo = {
  languages: [{ language: "Go", percent: 100, color: "#00ADD8" }],
} as unknown as Repo;

const blob: Blob = {
  path: "src/main.go",
  size: 5,
  encoding: "utf-8",
  content: "package main",
};

function renderTab(overrides: Record<string, unknown> = {}) {
  const setParams = vi.fn();
  const result = render(
    <I18nProvider>
      <CodeTab
        owner="alice"
        name="demo"
        repo={repo}
        refName="main"
        locale="en"
        path={[]}
        currentDir=""
        branches={[{ name: "main", is_head: true }]}
        tags={[]}
        entries={[]}
        dirLatestCommit={null}
        blob={null}
        blame={null}
        error=""
        emptyRepo={false}
        readmeContent={null}
        readmeEntryName={null}
        blameParam={false}
        lineParam={null}
        setParams={setParams}
        commands={[]}
        openRefs={vi.fn()}
        openCreateDialog={vi.fn()}
        openEditDialog={vi.fn()}
        renameEntry={vi.fn()}
        removeEntry={vi.fn()}
        copy={vi.fn()}
        {...overrides}
      />
    </I18nProvider>,
  );
  return { setParams, ...result };
}

const scrollIntoView = vi.fn();

beforeEach(() => {
  vi.clearAllMocks();
  Element.prototype.scrollIntoView = scrollIntoView;
});

describe("CodeTab", () => {
  it("打开文件后渲染左侧文件树与右侧大纲", () => {
    renderTab({ blob });
    expect(screen.getAllByTestId("file-tree")).toHaveLength(1);
    expect(screen.getByTestId("outline")).toBeInTheDocument();
    expect(screen.getByTestId("repo-code-body")).toBeInTheDocument();
    expect(screen.getByTestId("language-bar")).toBeInTheDocument();
  });

  it("目录列表（无 blob）不渲染文件树与大纲", () => {
    renderTab();
    expect(screen.queryByTestId("file-tree")).not.toBeInTheDocument();
    expect(screen.queryByTestId("outline")).not.toBeInTheDocument();
  });

  it("blame 模式隐藏大纲", () => {
    renderTab({ blob, blameParam: true });
    expect(screen.queryByTestId("outline")).not.toBeInTheDocument();
  });

  it("桌面文件树打开目录 / 文件回传参数", async () => {
    const user = userEvent.setup();
    const { setParams } = renderTab({ blob });
    const tree = screen.getByTestId("file-tree");

    await user.click(within(tree).getByText("tree-dir"));
    expect(setParams).toHaveBeenCalledWith({
      path: "dir1",
      file: null,
      line: null,
      blame: null,
    });

    await user.click(within(tree).getByText("tree-file"));
    expect(setParams).toHaveBeenCalledWith({ file: "f.ts", line: null, blame: null });
  });

  it("窄屏：Files 抽屉打开后跳转文件并关闭", async () => {
    const user = userEvent.setup();
    const { setParams } = renderTab({ blob });

    await user.click(screen.getByRole("button", { name: "Files" }));
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByText("tree-file"));

    expect(setParams).toHaveBeenCalledWith({ file: "f.ts", line: null, blame: null });
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("窄屏：大纲抽屉跳转行号", async () => {
    const user = userEvent.setup();
    const { setParams } = renderTab({ blob });

    await user.click(screen.getByRole("button", { name: "Outline" }));
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByText("outline-line"));

    expect(setParams).toHaveBeenCalledWith({ line: "42" });
  });

  it("New 菜单触发新建文件 / 文件夹", async () => {
    const user = userEvent.setup();
    const openCreateDialog = vi.fn();
    renderTab({ blob, openCreateDialog });

    await user.click(screen.getByRole("button", { name: /^New/ }));
    await user.click(await screen.findByRole("menuitem", { name: /new file/i }));
    expect(openCreateDialog).toHaveBeenCalledWith("create-file");

    await user.click(screen.getByRole("button", { name: /^New/ }));
    await user.click(await screen.findByRole("menuitem", { name: /new folder/i }));
    expect(openCreateDialog).toHaveBeenCalledWith("create-dir");
  });

  it("lineParam 滚动并高亮目标行，卸载后清理", () => {
    const { unmount } = renderTab({ blob, lineParam: 2 });

    const line2 = screen.getByText("line 2");
    expect(scrollIntoView).toHaveBeenCalled();
    expect(line2.style.backgroundColor).not.toBe("");

    unmount();
    expect(line2.style.backgroundColor).toBe("");
  });

  it("无 lineParam 时不高亮任何行", () => {
    renderTab({ blob });
    expect(screen.getByText("line 1").style.backgroundColor).toBe("");
    expect(scrollIntoView).not.toHaveBeenCalled();
  });
});
