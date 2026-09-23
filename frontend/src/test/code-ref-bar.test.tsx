import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "@/lib/i18n";
import CodeRefBar from "@/pages/repoview/code-ref-bar";
import { vi } from "vitest";

vi.mock("@/components/compare-dialog", () => ({
  CompareDialog: ({ open }: { open: boolean }) => (
    <div data-testid="compare-dialog" data-open={String(open)} />
  ),
}));

const branches = [
  { name: "main", is_head: true },
  { name: "dev", is_head: false },
];
const tags = [{ name: "v1.0", sha: "abcdef1234567890", message: "" }];

function renderBar(overrides: Record<string, unknown> = {}) {
  const handlers = {
    onCompareOpenChange: vi.fn(),
    onSelectRef: vi.fn(),
    onOpenRefs: vi.fn(),
    onOpenDir: vi.fn(),
  };
  render(
    <I18nProvider>
      <CodeRefBar
        owner="alice"
        name="demo"
        refName="main"
        branches={branches}
        tags={tags}
        emptyRepo={false}
        crumbs={["src", "main.go"]}
        hasBlob
        compareOpen={false}
        {...handlers}
        {...overrides}
      />
    </I18nProvider>,
  );
  return handlers;
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("CodeRefBar", () => {
  it("展示当前 ref 与面包屑", () => {
    renderBar();
    expect(screen.getByRole("button", { name: /main/ })).toBeInTheDocument();
    // 根节点为仓库名，其余为路径分段
    expect(screen.getByRole("button", { name: "demo" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "src" })).toBeInTheDocument();
    // 最后一段是文件名，不是按钮
    expect(screen.queryByRole("button", { name: "main.go" })).not.toBeInTheDocument();
  });

  it("点击根/目录面包屑回传路径", async () => {
    const user = userEvent.setup();
    const { onOpenDir } = renderBar();

    await user.click(screen.getByRole("button", { name: "demo" }));
    expect(onOpenDir).toHaveBeenCalledWith("");

    await user.click(screen.getByRole("button", { name: "src" }));
    expect(onOpenDir).toHaveBeenCalledWith("src");
  });

  it("切换分支时回调 onSelectRef", async () => {
    const user = userEvent.setup();
    const { onSelectRef } = renderBar();

    await user.click(screen.getByRole("button", { name: /main/ }));
    await user.click(await screen.findByRole("menuitem", { name: /dev/ }));
    expect(onSelectRef).toHaveBeenCalledWith("dev");
  });

  it("选择 tag 时回调 onSelectRef", async () => {
    const user = userEvent.setup();
    const { onSelectRef } = renderBar();

    await user.click(screen.getByRole("button", { name: /main/ }));
    await user.click(await screen.findByRole("menuitem", { name: /v1\.0/ }));
    expect(onSelectRef).toHaveBeenCalledWith("v1.0");
  });

  it("菜单中的比较项打开 CompareDialog", async () => {
    const user = userEvent.setup();
    const { onCompareOpenChange } = renderBar();

    await user.click(screen.getByRole("button", { name: /main/ }));
    await user.click(await screen.findByRole("menuitem", { name: /compare/i }));
    expect(onCompareOpenChange).toHaveBeenCalledWith(true);
  });

  it("管理按钮回调 onOpenRefs", async () => {
    const user = userEvent.setup();
    const { onOpenRefs } = renderBar();

    await user.click(screen.getByTitle("Branches & tags"));
    expect(onOpenRefs).toHaveBeenCalled();
  });

  it("空仓库禁用 ref 下拉且隐藏面包屑", () => {
    renderBar({ emptyRepo: true, crumbs: [] });
    expect(screen.getByRole("button", { name: /main/ })).toBeDisabled();
    expect(screen.queryByRole("button", { name: "demo" })).not.toBeInTheDocument();
  });
});
