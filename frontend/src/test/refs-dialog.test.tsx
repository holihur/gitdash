import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "@/lib/i18n";
import RefsDialog from "@/components/refs-dialog";
import { vi } from "vitest";

function jsonRes(body: unknown, status = 200, headers: Record<string, string> = {}) {
  return Promise.resolve(
    new Response(status === 204 ? null : JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json", ...headers },
    }),
  );
}

interface StubOptions {
  branches?: unknown[];
  branchTotal?: number;
  tags?: unknown[];
  tagTotal?: number;
}

/** 按 URL 路由 fetch，并记录调用（方法 / 请求体）。 */
function stubFetch(opts: StubOptions = {}) {
  const calls: { url: string; method: string; body: string | null }[] = [];
  const branches = opts.branches ?? [
    { name: "main", is_head: true, note: "主线" },
    { name: "dev", is_head: false },
  ];
  const tags = opts.tags ?? [
    { name: "v1.0", sha: "abcdef1234567890", message: "", note: "首个版本" },
  ];
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = (init?.method ?? "GET").toUpperCase();
      calls.push({ url, method, body: (init?.body as string) ?? null });
      if (url.includes("/note")) return jsonRes({ kind: "branch", name: "dev", note: "x" });
      if (url.includes("/branches")) {
        return jsonRes(branches, 200, { "X-Total-Count": String(opts.branchTotal ?? branches.length) });
      }
      if (url.includes("/tags")) {
        return jsonRes(tags, 200, { "X-Total-Count": String(opts.tagTotal ?? tags.length) });
      }
      return jsonRes({}, 404);
    }),
  );
  return calls;
}

function renderDialog(canWrite = true) {
  return render(
    <I18nProvider>
      <RefsDialog
        open
        onClose={() => {}}
        owner="alice"
        repo="demo"
        current="main"
        canWrite={canWrite}
        onRefresh={() => {}}
      />
    </I18nProvider>,
  );
}

beforeEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
});

describe("RefsDialog", () => {
  it("分支与标签分属独立 tab，并展示各自备注", async () => {
    stubFetch();
    renderDialog();

    // 分支 tab 默认激活
    expect(await screen.findByText("main")).toBeInTheDocument();
    expect(screen.getByText("主线")).toBeInTheDocument();
    // 标签备注此时不应渲染（未激活的 tab 不挂载）
    expect(screen.queryByText("首个版本")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("tab", { name: /Tags/ }));
    expect(await screen.findByText("v1.0")).toBeInTheDocument();
    expect(screen.getByText("首个版本")).toBeInTheDocument();
  });

  it("编辑备注后以 PUT 保存", async () => {
    const calls = stubFetch();
    renderDialog();

    await screen.findByText("dev");
    // 每行一个编辑按钮：main 在前，dev 在后
    const editButtons = screen.getAllByTitle("Edit note");
    await userEvent.click(editButtons[1]);

    const textarea = await screen.findByPlaceholderText(/Add a note for this branch/i);
    await userEvent.type(textarea, "新备注");
    await userEvent.click(screen.getByRole("button", { name: "Save note" }));

    await waitFor(() => {
      const put = calls.find((c) => c.method === "PUT" && c.url.includes("/refs/branch/dev/note"));
      expect(put).toBeTruthy();
      expect(put?.body).toContain("新备注");
    });
  });

  it("分页：超过一页时显示分页并可翻页", async () => {
    const many = Array.from({ length: 10 }, (_, i) => ({ name: `b${i}`, is_head: false }));
    const calls = stubFetch({ branches: many, branchTotal: 25 });
    renderDialog();

    await screen.findByText("b0");
    // 第 2 页按钮
    await userEvent.click(screen.getByRole("button", { name: "2" }));

    await waitFor(() => {
      expect(
        calls.some((c) => c.url.includes("/branches") && c.url.includes("offset=10")),
      ).toBe(true);
    });
  });

  it("只读用户隐藏创建 / 删除 / 备注操作", async () => {
    stubFetch();
    renderDialog(false);

    await screen.findByText("dev");
    expect(screen.queryByTitle("Edit note")).not.toBeInTheDocument();
    expect(screen.queryByTitle("Delete branch")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Create" })).not.toBeInTheDocument();
  });
});
