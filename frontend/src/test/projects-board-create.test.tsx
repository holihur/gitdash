import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
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
      updateColumn: vi.fn(),
      updateSwimlane: vi.fn(),
      deleteColumn: vi.fn(),
      deleteSwimlane: vi.fn(),
      setCardAssignees: vi.fn(),
      setCardLabels: vi.fn(),
      listLabels: vi.fn().mockResolvedValue([]),
      listCollabs: vi.fn().mockResolvedValue([]),
      listIssues: vi.fn().mockResolvedValue({ items: [], total: 0 }),
      me: vi.fn().mockResolvedValue({ username: "alice" }),
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
    <MemoryRouter>
      <I18nProvider>
        <ProjectsBoard
          owner="alice"
          name="demo"
          project={project}
          role="owner"
          onBack={() => {}}
          onProjectChanged={() => {}}
        />
      </I18nProvider>
    </MemoryRouter>,
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

describe("ProjectsBoard 删除列时迁移卡片", () => {
  it("有卡片的列删前先选迁移目标", async () => {
    const user = userEvent.setup();
    (api.getBoard as Mock).mockResolvedValue({
      ...board,
      cards: [
        {
          id: 99,
          project_id: 1,
          column_id: 11,
          swimlane_id: 21,
          issue_number: 0,
          issue_title: "",
          issue_state: "",
          title: "Task",
          body: "",
          note: "Task",
          start_date: "",
          due_date: "",
          position: 0,
          created_at: "2026-01-01T00:00:00Z",
        },
      ],
    });
    (api.deleteColumn as Mock).mockResolvedValue(null);
    renderBoard();

    await waitFor(() => expect(screen.getByText("Task")).toBeInTheDocument());
    await user.click(screen.getAllByTitle("Column actions")[0]);
    await user.click(await screen.findByRole("menuitem", { name: /delete column/i }));

    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(api.deleteColumn).toHaveBeenCalledWith("alice", "demo", 1, 11, 12));
  });
});
