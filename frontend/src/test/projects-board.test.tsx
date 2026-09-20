import { render, screen } from "@testing-library/react";
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

function renderBoard(cards: ProjectCard[]) {
  const cardsByCell = new Map([[`${lane.id}:${column.id}`, cards]]);
  return render(
    <I18nProvider>
      <ProjectBoardView
        columns={[column]}
        lanes={[lane]}
        cardsByCell={cardsByCell}
        hasUngrouped={false}
        busy={false}
        onEditCard={() => {}}
        onDeleteCard={() => {}}
        onDeleteColumn={() => {}}
        onDeleteLane={() => {}}
        onAddCard={() => {}}
        onMoveCard={() => {}}
      />
    </I18nProvider>,
  );
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
