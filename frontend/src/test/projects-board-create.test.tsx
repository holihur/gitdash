import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { I18nProvider } from "@/lib/i18n";
import ProjectsBoard from "@/components/projects-board";

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
      getBoard: vi.fn(),
      createCard: vi.fn(),
      updateProject: vi.fn(),
      updateCard: vi.fn(),
      deleteCard: vi.fn(),
      createColumn: vi.fn(),
      createSwimlane: vi.fn(),
      deleteColumn: vi.fn(),
      deleteSwimlane: vi.fn(),
    },
  };
});

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

// jsdom 无布局尺寸，虚拟化不会挂载泳道；返回全部条目。
vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getTotalSize: () => count * 100,
    getVirtualItems: () =>
      Array.from({ length: count }, (_, i) => ({
        index: i,
        key: i,
        start: i * 100,
        size: 100,
        end: (i + 1) * 100,
      })),
    measureElement: () => {},
  }),
}));

const { api } = (await import("@/lib/api")) as typeof import("@/lib/api");

const project = {
  id: 1,
  owner: "alice",
  repo: "demo",
  name: "Board",
  description: "",
  card_count: 0,
  created_at: "2026-01-01T00:00:00Z",
};
const board = {
  project,
  columns: [
    { id: 11, project_id: 1, name: "To Do", position: 0 },
    { id: 12, project_id: 1, name: "Done", position: 1 },
  ],
  swimlanes: [{ id: 21, project_id: 1, name: "Default", position: 0 }],
  cards: [],
};

function renderBoard() {
  return render(
    <I18nProvider>
      <ProjectsBoard
        owner="alice"
        name="demo"
        project={project}
        role="owner"
        onBack={() => {}}
        onProjectChanged={() => {}}
      />
    </I18nProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  (api.getBoard as Mock).mockResolvedValue(board);
  (api.createCard as Mock).mockResolvedValue({ id: 99 });
});

describe("ProjectsBoard 从列表 / 甘特视图创建卡片", () => {
  it("列表视图下通过全局 Add card 选择列并创建卡片", async () => {
    const user = userEvent.setup();
    renderBoard();

    // 切换到列表视图（此时没有任何卡片，仍应能新建）
    await user.click(await screen.findByRole("button", { name: "List" }));
    await user.click(screen.getByRole("button", { name: "Add card" }));

    const dialog = await screen.findByRole("dialog");
    await user.type(within(dialog).getByLabelText("Name"), "From list");
    await user.selectOptions(within(dialog).getByLabelText("Column"), "12");
    await user.click(within(dialog).getByRole("button", { name: "Add card" }));

    await waitFor(() =>
      expect(api.createCard).toHaveBeenCalledWith(
        "alice",
        "demo",
        1,
        expect.objectContaining({ column_id: 12, swimlane_id: 21, title: "From list" }),
      ),
    );
  });

  it("甘特视图下同样可以打开新建卡片对话框", async () => {
    const user = userEvent.setup();
    renderBoard();

    await user.click(await screen.findByRole("button", { name: "Gantt" }));
    await user.click(screen.getByRole("button", { name: "Add card" }));

    const dialog = await screen.findByRole("dialog");
    // 默认选中首列 / 首泳道
    expect((within(dialog).getByLabelText("Column") as HTMLSelectElement).value).toBe("11");
    expect((within(dialog).getByLabelText("Swimlane") as HTMLSelectElement).value).toBe("21");
  });
});
