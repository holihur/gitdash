import { createRef } from "react";
import { render, screen } from "@testing-library/react";
import { I18nProvider } from "@/lib/i18n";
import { RepoCodeBody } from "@/pages/repoview/repo-code-body";
import type { Blob, Commit, TreeEntry } from "@/lib/api";
import { vi } from "vitest";

vi.mock("@/pages/repoview/blob-view", () => ({
  default: ({ blob }: { blob: Blob }) => <div data-testid="blob-view">{blob.path}</div>,
}));
vi.mock("@/components/tree-listing", () => ({
  default: ({ entries }: { entries: TreeEntry[] }) => (
    <div data-testid="tree-listing">{entries.map((e) => e.name).join(",")}</div>
  ),
}));
vi.mock("@/components/markdown", () => ({
  MarkdownWithToc: ({ text }: { text: string }) => <div data-testid="readme">{text}</div>,
}));

const noop = () => {};
const commit: Commit = {
  sha: "abcdef1234567890",
  author: "alice",
  date: "2026-01-01T00:00:00Z",
  message: "initial commit",
};

function renderBody(overrides: Record<string, unknown> = {}) {
  render(
    <I18nProvider>
      <RepoCodeBody
        owner="alice"
        name="demo"
        refName="main"
        locale="en"
        latestCommit={null}
        error=""
        emptyRepo={false}
        commands={[]}
        copy={noop}
        blob={null}
        blame={null}
        blameParam={false}
        codeHostRef={createRef<HTMLDivElement>()}
        onToggleBlame={noop}
        onEditBlob={noop}
        onDeleteBlob={noop}
        onOpenRepoLink={noop}
        entries={[]}
        currentDir=""
        onOpenEntry={noop}
        onEditPath={noop}
        onRenamePath={noop}
        onRemoveEntry={noop}
        readmeContent={null}
        readmeEntryName={null}
        readmePath=""
        {...overrides}
      />
    </I18nProvider>,
  );
}

describe("RepoCodeBody", () => {
  it("展示最近提交条（信息 / 作者 / 短 sha）", () => {
    renderBody({ latestCommit: commit });
    expect(screen.getByText("initial commit")).toBeInTheDocument();
    expect(screen.getByText("alice")).toBeInTheDocument();
    expect(screen.getByText("abcdef1")).toBeInTheDocument();
  });

  it("错误时展示错误卡片", () => {
    renderBody({ error: "something broke" });
    expect(screen.getByText("something broke")).toBeInTheDocument();
  });

  it("空仓库展示引导命令与 SSH 提示", () => {
    renderBody({
      emptyRepo: true,
      commands: ["git init", "git push origin main"],
    });
    expect(screen.getByText("Empty repository")).toBeInTheDocument();
    expect(screen.getByText("git init")).toBeInTheDocument();
    expect(screen.getByText("git push origin main")).toBeInTheDocument();
    expect(screen.getByText(/add your public key/i)).toBeInTheDocument();
  });

  it("有 blob 时交给 BlobView", () => {
    renderBody({
      blob: { path: "src/main.go", size: 5, encoding: "utf-8", content: "x" },
    });
    expect(screen.getByTestId("blob-view")).toHaveTextContent("src/main.go");
    expect(screen.queryByTestId("tree-listing")).not.toBeInTheDocument();
  });

  it("无 blob 时展示目录列表", () => {
    renderBody({
      entries: [
        { name: "README.md", type: "blob" },
        { name: "src", type: "tree" },
      ] as TreeEntry[],
    });
    expect(screen.getByTestId("tree-listing")).toHaveTextContent("README.md,src");
  });

  it("根目录有 README 时渲染其内容", () => {
    renderBody({
      readmeContent: "# hello",
      readmeEntryName: "README.md",
      readmePath: "README.md",
    });
    expect(screen.getByText("README.md")).toBeInTheDocument();
    expect(screen.getByTestId("readme")).toHaveTextContent("# hello");
  });
});
