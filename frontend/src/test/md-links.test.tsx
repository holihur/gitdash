import { describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "@/lib/i18n";
import { MarkdownView } from "@/components/markdown";
import {
  buildRepoHref,
  headingAnchorMap,
  parseRepoLink,
  resolveRepoPath,
  rewriteMarkdownHtml,
  slugifyHeading,
} from "@/lib/md-links";
import { buildRepoPath, parseRepoRoute } from "@/lib/repo-url";

describe("slugifyHeading", () => {
  it("按 GitHub 规则生成锚点 slug", () => {
    expect(slugifyHeading("OAuth 2.0 provider (applications)")).toBe(
      "oauth-20-provider-applications",
    );
    expect(slugifyHeading("CLI (gitdash-cli)")).toBe("cli-gitdash-cli");
    // 去标点后保留的空格会形成连续连字符
    expect(slugifyHeading("Backup & Restore")).toBe("backup--restore");
    expect(slugifyHeading("备份与恢复")).toBe("备份与恢复");
  });
});

describe("resolveRepoPath", () => {
  it("相对当前文件目录解析，并处理 ./ ../ 与仓库根绝对路径", () => {
    expect(resolveRepoPath("README.md", "docs/projects.md")).toBe("docs/projects.md");
    expect(resolveRepoPath("docs/runners.md", "./runners.zh-CN.md")).toBe(
      "docs/runners.zh-CN.md",
    );
    expect(resolveRepoPath("docs/copilot.md", "../deps/agent")).toBe("deps/agent");
    expect(resolveRepoPath("README.md", "/packaging/gitdash.service")).toBe(
      "packaging/gitdash.service",
    );
  });
});

describe("parseRepoLink", () => {
  const ctx = { owner: "alice", name: "demo", ref: "main", path: "README.md" };

  it("区分文件 / 目录并剥离锚点", () => {
    expect(parseRepoLink(ctx, "docs/projects.md")).toEqual({
      path: "docs/projects.md",
      kind: "file",
      hash: undefined,
    });
    expect(parseRepoLink(ctx, "docs/")).toEqual({ path: "docs", kind: "dir", hash: undefined });
    expect(parseRepoLink(ctx, "docs/guide.md#install")).toEqual({
      path: "docs/guide.md",
      kind: "file",
      hash: "install",
    });
  });

  it("站外链接、页内锚点与既有应用路由不改写", () => {
    expect(parseRepoLink(ctx, "https://example.com")).toBeNull();
    expect(parseRepoLink(ctx, "mailto:a@b.c")).toBeNull();
    expect(parseRepoLink(ctx, "#backup--restore")).toBeNull();
    expect(parseRepoLink(ctx, "/repo/alice/demo?file=x")).toBeNull();
  });
});

describe("buildRepoHref", () => {
  it("生成代码浏览路由并保持 ref", () => {
    const href = buildRepoHref(
      { owner: "alice", name: "demo", ref: "feature/x", path: "README.md" },
      { path: "docs/projects.md", kind: "file" },
    );
    expect(href).toBe("/repo/alice/demo/blob/docs/projects.md?ref=feature%2Fx");
  });

  it("目录与越级相对路径生成 tree 路由", () => {
    const href = buildRepoHref(
      { owner: "alice", name: "demo", ref: "main", path: "docs/copilot.md" },
      { path: "deps/agent", kind: "dir" },
    );
    expect(href).toBe("/repo/alice/demo/tree/deps/agent?ref=main");
  });
});

describe("repo-url 路径化路由", () => {
  it("解析 tree / blob / blame / tab 路径", () => {
    expect(parseRepoRoute("")).toEqual({ tab: "code", kind: "tree", path: "" });
    expect(parseRepoRoute("tree/docs/sub")).toEqual({
      tab: "code",
      kind: "tree",
      path: "docs/sub",
    });
    expect(parseRepoRoute("blob/docs/projects.md")).toEqual({
      tab: "code",
      kind: "blob",
      path: "docs/projects.md",
    });
    expect(parseRepoRoute("blame/README.md")).toEqual({
      tab: "code",
      kind: "blame",
      path: "README.md",
    });
    expect(parseRepoRoute("issues")).toEqual({ tab: "issues", kind: "tree", path: "" });
    expect(parseRepoRoute("commits")).toEqual({ tab: "commits", kind: "tree", path: "" });
  });

  it("构造仓库路径并编码文件名", () => {
    expect(buildRepoPath("alice", "demo", { tab: "code" })).toBe("/repo/alice/demo");
    expect(
      buildRepoPath("alice", "demo", { tab: "code", kind: "tree", path: "docs/sub" }),
    ).toBe("/repo/alice/demo/tree/docs/sub");
    expect(
      buildRepoPath("alice", "demo", { tab: "code", kind: "blob", path: "a b/c.md" }),
    ).toBe("/repo/alice/demo/blob/a%20b/c.md");
    expect(buildRepoPath("alice", "demo", { tab: "issues" })).toBe("/repo/alice/demo/issues");
  });
});

describe("rewriteMarkdownHtml", () => {
  it("改写仓库内相对链接并写入 data 属性", () => {
    const html = rewriteMarkdownHtml(
      '<p><a href="docs/projects.md">项目</a></p>',
      { owner: "alice", name: "demo", ref: "main", path: "README.md" },
    );
    expect(html).toContain('data-repo-path="docs/projects.md"');
    expect(html).toContain('data-repo-kind="file"');
    expect(html).toContain('href="/repo/alice/demo/blob/docs/projects.md?ref=main"');
  });

  it("页内锚点指向对应标题 id", () => {
    const html = rewriteMarkdownHtml(
      [
        '<h2>Backup &amp; Restore</h2>',
        '<p><a href="#backup--restore">跳转</a></p>',
      ].join(""),
    );
    expect(html).toContain('id="md-heading-0"');
    expect(html).toContain('href="#md-heading-0"');
  });

  it("外部链接保持不变", () => {
    const html = rewriteMarkdownHtml(
      '<p><a href="https://example.com/x.md">外链</a></p>',
      { owner: "alice", name: "demo", ref: "main", path: "README.md" },
    );
    expect(html).toContain('href="https://example.com/x.md"');
    expect(html).not.toContain("data-repo-path");
  });
});

describe("headingAnchorMap", () => {
  it("重名标题按 GitHub 规则追加序号", () => {
    const root = document.createElement("div");
    root.innerHTML = '<h1 id="a">Intro</h1><h2 id="b">Intro</h2>';
    const map = headingAnchorMap(root);
    expect(map.get("intro")).toBe("a");
    expect(map.get("intro-1")).toBe("b");
  });
});

describe("MarkdownView 仓库内引用", () => {
  it("渲染后链接指向代码浏览路由，点击回调可拦截", async () => {
    const onOpenRepoLink = vi.fn();
    render(
      <I18nProvider>
        <MarkdownView
          text={"[项目](docs/projects.md)"}
          repo={{ owner: "alice", name: "demo", ref: "main", path: "README.md" }}
          onOpenRepoLink={onOpenRepoLink}
        />
      </I18nProvider>,
    );

    const link = await waitFor(() => {
      const el = document.querySelector<HTMLAnchorElement>("a[data-repo-path]");
      if (!el) throw new Error("link not rendered");
      return el;
    });
    expect(link.getAttribute("href")).toBe("/repo/alice/demo/blob/docs/projects.md?ref=main");

    link.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true, button: 0 }));
    expect(onOpenRepoLink).toHaveBeenCalledWith({
      path: "docs/projects.md",
      kind: "file",
      hash: undefined,
    });
  });

  it("无仓库上下文时普通链接不被改写", async () => {
    render(
      <I18nProvider>
        <MarkdownView text={"[外链](https://example.com)"} />
      </I18nProvider>,
    );
    const link = await screen.findByRole("link", { name: "外链" });
    expect(link.getAttribute("href")).toBe("https://example.com");
    expect(link.hasAttribute("data-repo-path")).toBe(false);
  });
});
