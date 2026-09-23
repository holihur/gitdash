import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { I18nProvider } from "@/lib/i18n";
import { ProjectBoardView } from "@/components/projects-board-view";
import type { ProjectCard, ProjectColumn, ProjectSwimlane } from "@/lib/api";

// jsdom 里没有真实尺寸，虚拟化不会挂载任何泳道/卡片；返回全部条目以便断言。
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

const column: ProjectColumn = { id: 1, project_id: 1, name: "To Do", position: 0 };
const lane: ProjectSwimlane = { id: 1, project_id: 1, name: "Default", position: 0 };

function textCard(over: Partial<ProjectCard> = {}): ProjectCard {
  return {
    id: 10,
    project_id: 1,
    column_id: 1,
    swimlane_id: 1,
    issue_number: 0,
    issue_title: "",
    issue_state: "",
    title: "First card",
    body: "some details",
    note: "First card",
    start_date: "",
    due_date: "",
    position: 0,
    created_at: "2026-01-01T00:00:00Z",
    ...over,
  };
}

function renderBoard(cards: ProjectCard[], columns: ProjectColumn[] = [column], lanes: ProjectSwimlane[] = [lane]) {
  const cardsByCell = new Map([[`${lane.id}:${column.id}`, cards]]);
  const handlers = {
    onEditCard: vi.fn(),
    onDeleteCard: vi.fn(),
    onDeleteColumn: vi.fn(),
    onDeleteLane: vi.fn(),
    onAddCard: vi.fn(),
    onMoveCard: vi.fn(),
    onRenameColumn: vi.fn(),
    onMoveColumn: vi.fn(),
    onRenameLane: vi.fn(),
    onMoveLane: vi.fn(),
  };
  render(
    <I18nProvider>
      <ProjectBoardView
        columns={columns}
        lanes={lanes}
        cardsByCell={cardsByCell}
        hasUngrouped={false}
        busy={false}
        {...handlers}
      />
    </I18nProvider>,
  );
  return handlers;
}

describe("ProjectBoardView 卡片渲染", () => {
  it("文本卡片（issue_number = 0）显示标题与详情，而不是当作 issue #0", async () => {
    renderBoard([textCard()]);
    expect(screen.getByText("First card")).toBeInTheDocument();
    expect(await screen.findByText("some details")).toBeInTheDocument();
    expect(screen.queryByText("#0")).not.toBeInTheDocument();
  });

  it("issue 卡片显示 issue 标题与编号", () => {
    renderBoard([
      textCard({
        issue_number: 42,
        issue_title: "Fix the thing",
        issue_state: "open",
        title: "",
        body: "",
        note: "",
      }),
    ]);
    expect(screen.getByText("Fix the thing")).toBeInTheDocument();
    expect(screen.getByText("#42")).toBeInTheDocument();
  });
});

describe("ProjectBoardView 列 / 泳道管理", () => {
  it("列操作菜单可重命名并提交", async () => {
    const user = userEvent.setup();
    const h = renderBoard([textCard()]);

    await user.click(screen.getByTitle("Column actions"));
    await user.click(await screen.findByRole("menuitem", { name: /rename column/i }));

    const input = screen.getByDisplayValue("To Do");
    await user.clear(input);
    await user.type(input, "Doing{Enter}");
    expect(h.onRenameColumn).toHaveBeenCalledWith(1, "Doing");
  });

  it("多列时可左右移动", async () => {
    const user = userEvent.setup();
    const columns: ProjectColumn[] = [
      { id: 1, project_id: 1, name: "To Do", position: 0 },
      { id: 2, project_id: 1, name: "Done", position: 1 },
    ];
    const h = renderBoard([], columns);

    await user.click(screen.getAllByTitle("Column actions")[0]);
    await user.click(await screen.findByRole("menuitem", { name: /move right/i }));
    expect(h.onMoveColumn).toHaveBeenCalledWith(1, 1);
  });

  it("泳道操作菜单可上移 / 删除", async () => {
    const user = userEvent.setup();
    const lanes: ProjectSwimlane[] = [
      { id: 1, project_id: 1, name: "A", position: 0 },
      { id: 2, project_id: 1, name: "B", position: 1 },
    ];
    const h = renderBoard([], [column], lanes);

    await user.click(screen.getAllByTitle("Swimlane actions")[1]);
    await user.click(await screen.findByRole("menuitem", { name: /move up/i }));
    expect(h.onMoveLane).toHaveBeenCalledWith(2, -1);
  });

  it("卡片菜单可移动到其它列（键盘可达）", async () => {
    const user = userEvent.setup();
    const columns: ProjectColumn[] = [
      { id: 1, project_id: 1, name: "To Do", position: 0 },
      { id: 2, project_id: 1, name: "Done", position: 1 },
    ];
    const h = renderBoard([textCard()], columns);

    await user.click(screen.getByTitle("Move card"));
    await user.click(await screen.findByRole("menuitem", { name: "Done" }));
    expect(h.onMoveCard).toHaveBeenCalledWith(10, 1, 2, 0);
  });
});
