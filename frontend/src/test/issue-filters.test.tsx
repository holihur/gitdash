import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "@/lib/i18n";
import { IssueFilters } from "@/components/issues/issue-filters";
import type { Milestone } from "@/lib/api";
import { describe, expect, it, vi } from "vitest";

const milestones: Milestone[] = [
  { id: 1, title: "v1.0", description: "", state: "open", open_issues: 2, closed_issues: 0 },
  { id: 2, title: "v2.0", description: "", state: "closed", open_issues: 0, closed_issues: 3 },
];

function setup(
  over: {
    filterMilestone?: string;
    onFilterMilestone?: (v: string) => void;
    onFilterAssignee?: (v: string) => void;
    onFilterSort?: (v: string) => void;
  } = {},
) {
  const onFilterMilestone = over.onFilterMilestone ?? vi.fn();
  const onFilterAssignee = over.onFilterAssignee ?? vi.fn();
  const onFilterSort = over.onFilterSort ?? vi.fn();
  render(
    <I18nProvider>
      <IssueFilters
        searchInput=""
        onSearchInput={() => {}}
        stateFilter=""
        onStateFilter={() => {}}
        labels={[]}
        filterLabel={null}
        onFilterLabel={() => {}}
        milestones={milestones}
        filterMilestone={over.filterMilestone ?? ""}
        onFilterMilestone={onFilterMilestone}
        filterAssignee=""
        onFilterAssignee={onFilterAssignee}
        filterSort=""
        onFilterSort={onFilterSort}
      />
    </I18nProvider>,
  );
  return { onFilterMilestone, onFilterAssignee, onFilterSort };
}

describe("IssueFilters milestone filter", () => {
  it("列出全部 / 无里程碑 / 各里程碑选项", () => {
    setup();
    const select = screen.getByLabelText("Milestone") as HTMLSelectElement;
    expect(select.value).toBe("");
    expect(screen.getByRole("option", { name: "All milestones" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "No milestone" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "v1.0" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "v2.0" })).toBeInTheDocument();
  });

  it("选择里程碑 / 未指派时回调过滤值", async () => {
    const user = userEvent.setup();
    const { onFilterMilestone } = setup();

    await user.selectOptions(screen.getByLabelText("Milestone"), "1");
    expect(onFilterMilestone).toHaveBeenLastCalledWith("1");

    await user.selectOptions(screen.getByLabelText("Milestone"), "none");
    expect(onFilterMilestone).toHaveBeenLastCalledWith("none");

    await user.selectOptions(screen.getByLabelText("Milestone"), "");
    expect(onFilterMilestone).toHaveBeenLastCalledWith("");
  });

  it("按 filterMilestone 受控显示", () => {
    setup({ filterMilestone: "2" });
    expect((screen.getByLabelText("Milestone") as HTMLSelectElement).value).toBe("2");
  });
});

describe("IssueFilters assignee & sort", () => {
  it("选择负责人与排序时回调", async () => {
    const user = userEvent.setup();
    const { onFilterAssignee, onFilterSort } = setup();

    await user.selectOptions(screen.getByLabelText("Assignee"), "me");
    expect(onFilterAssignee).toHaveBeenLastCalledWith("me");
    await user.selectOptions(screen.getByLabelText("Assignee"), "none");
    expect(onFilterAssignee).toHaveBeenLastCalledWith("none");

    await user.selectOptions(screen.getByLabelText("Sort"), "oldest");
    expect(onFilterSort).toHaveBeenLastCalledWith("oldest");
    await user.selectOptions(screen.getByLabelText("Sort"), "popular");
    expect(onFilterSort).toHaveBeenLastCalledWith("popular");
  });
});
