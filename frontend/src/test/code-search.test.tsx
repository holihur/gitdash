import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "@/lib/i18n";
import { CodeSearch, HighlightText } from "@/components/code-search";
import { vi, type Mock } from "vitest";

vi.mock("@/lib/api", () => ({
  ApiError: class ApiError extends Error {},
  api: { searchRepo: vi.fn() },
}));

const { api } = (await import("@/lib/api")) as unknown as {
  api: { searchRepo: Mock };
};

function renderSearch(extra: Record<string, unknown> = {}) {
  const setParams = vi.fn();
  render(
    <I18nProvider>
      <CodeSearch
        owner="alice"
        name="demo"
        refName="main"
        setParams={setParams}
        {...extra}
      />
    </I18nProvider>,
  );
  return setParams;
}

const input = () => screen.getByLabelText(/Search code in this repository/);

beforeEach(() => {
  vi.clearAllMocks();
});

describe("CodeSearch", () => {
  it("防抖 300ms 后发起搜索并渲染结果", async () => {
    const user = userEvent.setup();
    api.searchRepo.mockResolvedValue([
      { path: "src/main.go", line: 3, text: "func main() {}" },
    ]);
    renderSearch();

    await user.type(input(), "main");
    await waitFor(() => expect(api.searchRepo).toHaveBeenCalledTimes(1), { timeout: 1500 });
    expect(api.searchRepo).toHaveBeenCalledWith("alice", "demo", "main", "main");

    expect(await screen.findByText(/func/)).toBeInTheDocument();
    // 命中子串被高亮
    expect(screen.getByText("main")).toBeInTheDocument();
  });

  it("Enter 立即搜索（不等防抖）", async () => {
    const user = userEvent.setup();
    api.searchRepo.mockResolvedValue([]);
    renderSearch();

    await user.type(input(), "go");
    await user.keyboard("{Enter}");
    await waitFor(() =>
      expect(api.searchRepo).toHaveBeenCalledWith("alice", "demo", "go", "main"),
    );
  });

  it("点击结果回传 file 与 line", async () => {
    const user = userEvent.setup();
    api.searchRepo.mockResolvedValue([{ path: "src/main.go", line: 7, text: "hit" }]);
    const setParams = renderSearch();

    await user.type(input(), "hit");
    const result = await screen.findByTitle("src/main.go:7");
    await user.click(result);

    expect(setParams).toHaveBeenCalledWith({ file: "src/main.go", line: "7" });
  });

  it("无命中时展示空态", async () => {
    const user = userEvent.setup();
    api.searchRepo.mockResolvedValue([]);
    renderSearch();

    await user.type(input(), "nothing");
    expect(await screen.findByText("No results")).toBeInTheDocument();
  });

  it("搜索失败时清空结果且不崩溃", async () => {
    const user = userEvent.setup();
    api.searchRepo.mockRejectedValue(new Error("boom"));
    renderSearch();

    await user.type(input(), "x");
    expect(await screen.findByText("No results")).toBeInTheDocument();
  });

  it("清空输入后隐藏结果面板", async () => {
    const user = userEvent.setup();
    api.searchRepo.mockResolvedValue([{ path: "a.go", line: 1, text: "x" }]);
    renderSearch();

    await user.type(input(), "x");
    expect(await screen.findByTitle("a.go:1")).toBeInTheDocument();

    await user.clear(input());
    await waitFor(() => expect(screen.queryByTitle("a.go:1")).not.toBeInTheDocument());
  });

  it("渲染同一行的 leading / actions 插槽", () => {
    renderSearch({
      leading: <span data-testid="leading">branch</span>,
      actions: <button data-testid="actions">new</button>,
    });
    expect(screen.getByTestId("leading")).toBeInTheDocument();
    expect(screen.getByTestId("actions")).toBeInTheDocument();
  });
});

describe("HighlightText", () => {
  it("高亮所有出现位置", () => {
    render(
      <p>
        <HighlightText text="foo bar foo baz foo" q="foo" />
      </p>,
    );
    expect(screen.getAllByText("foo")).toHaveLength(3);
  });

  it("大小写不敏感且保留原文大小写", () => {
    render(
      <p>
        <HighlightText text="Foo foo" q="foo" />
      </p>,
    );
    const marks = screen.getAllByText(/foo/i);
    expect(marks).toHaveLength(2);
    expect(marks[0].tagName).toBe("MARK");
    expect(marks[0]).toHaveTextContent("Foo");
  });

  it("无命中时原样返回", () => {
    render(
      <p>
        <HighlightText text="hello" q="zzz" />
      </p>,
    );
    expect(screen.getByText("hello")).toBeInTheDocument();
  });
});
