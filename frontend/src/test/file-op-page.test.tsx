import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { I18nProvider } from "@/lib/i18n";
import FileOpPage from "@/pages/repoview/file-op-page";
import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";

vi.mock("@/lib/api", () => {
  class ApiError extends Error {
    status: number;
    code?: string;
    constructor(status: number, message: string, code?: string) {
      super(message);
      this.name = "ApiError";
      this.status = status;
      this.code = code;
    }
  }
  return {
    ApiError,
    api: {
      branches: vi.fn(),
      blob: vi.fn(),
      createCommit: vi.fn(),
    },
  };
});

// 用 textarea 替身避免在 jsdom 里拉起 CodeMirror。
vi.mock("@/components/code-editor-lazy", () => ({
  default: ({ value, onDocChange }: { value: string; onDocChange?: (v: string) => void }) => (
    <textarea
      aria-label="content"
      value={value}
      onChange={(e) => onDocChange?.(e.target.value)}
    />
  ),
}));

const { api } = (await import("@/lib/api")) as typeof import("@/lib/api");

function renderPage(path: string, mode: "new" | "edit") {
  return render(
    <I18nProvider>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/repo/:owner/:name/new" element={<FileOpPage mode={mode} />} />
          <Route path="/repo/:owner/:name/edit" element={<FileOpPage mode={mode} />} />
          <Route path="*" element={<div>landed</div>} />
        </Routes>
      </MemoryRouter>
    </I18nProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  (api.branches as Mock).mockResolvedValue([{ name: "main", is_head: true }]);
  (api.blob as Mock).mockResolvedValue({
    path: "README.md",
    encoding: "utf-8",
    content: "# Hi",
    size: 4,
  });
  (api.createCommit as Mock).mockResolvedValue({ sha: "abc", branch: "main", message: "m" });
});

describe("FileOpPage", () => {
  it("新建文件：预填当前目录并提交 create", async () => {
    const user = userEvent.setup();
    renderPage("/repo/alice/demo/new?kind=file&dir=src&ref=main", "new");

    expect(await screen.findByText("New file")).toBeInTheDocument();
    const pathInput = screen.getByLabelText("Path") as HTMLInputElement;
    await waitFor(() => expect(pathInput.value).toBe("src/"));

    await user.clear(pathInput);
    await user.type(pathInput, "src/hello.py");
    await user.type(screen.getByLabelText("content"), "print(1)");
    await user.click(screen.getByRole("button", { name: "Commit changes" }));

    await waitFor(() =>
      expect(api.createCommit).toHaveBeenCalledWith("alice", "demo", "main", "Add src/hello.py", [
        { path: "src/hello.py", action: "create", content: "print(1)" },
      ]),
    );
  });

  it("新建文件夹：提交 .gitkeep 占位", async () => {
    const user = userEvent.setup();
    renderPage("/repo/alice/demo/new?kind=dir&dir=docs&ref=main", "new");

    expect(await screen.findByText("New folder")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Commit changes" }));

    await waitFor(() =>
      expect(api.createCommit).toHaveBeenCalledWith(
        "alice",
        "demo",
        "main",
        "Create directory docs",
        [{ path: "docs/.gitkeep", action: "create", content: "" }],
      ),
    );
  });

  it("编辑文件：按 ref+path 拉取内容并提交 update", async () => {
    const user = userEvent.setup();
    renderPage("/repo/alice/demo/edit?path=README.md&ref=main", "edit");

    await waitFor(() =>
      expect(api.blob).toHaveBeenCalledWith("alice", "demo", "main", "README.md"),
    );
    expect(await screen.findByText("Edit file")).toBeInTheDocument();
    await waitFor(() =>
      expect((screen.getByLabelText("content") as HTMLTextAreaElement).value).toBe("# Hi"),
    );

    await user.click(screen.getByRole("button", { name: "Commit changes" }));
    await waitFor(() =>
      expect(api.createCommit).toHaveBeenCalledWith("alice", "demo", "main", "Update README.md", [
        { path: "README.md", action: "update", content: "# Hi" },
      ]),
    );
  });

  it("二进制文件不可编辑", async () => {
    (api.blob as Mock).mockResolvedValue({
      path: "logo.png",
      encoding: "binary",
      content: "",
      size: 10,
    });
    renderPage("/repo/alice/demo/edit?path=logo.png&ref=main", "edit");

    expect(
      await screen.findByText("This file can't be edited in the browser (binary or too large)."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Commit changes" })).toBeDisabled();
  });
});
