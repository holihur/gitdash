import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { I18nProvider } from "@/lib/i18n";
import RepoPipeline from "@/pages/RepoPipeline";

vi.mock("@/lib/api", () => ({
  ApiError: class ApiError extends Error {},
  api: {
    getPipeline: vi.fn(),
    listPipelineRuns: vi.fn(),
    getPipelineRun: vi.fn(),
    getPipelineGraph: vi.fn(),
    setPipeline: vi.fn(),
    triggerPipelineRun: vi.fn(),
    rerunPipelineRun: vi.fn(),
    cancelPipelineRun: vi.fn(),
  },
}));

// 图的 Mermaid 渲染与本用例无关，避免加载重型依赖。
vi.mock("@/components/mermaid", () => ({ MermaidDiagram: () => null }));

const { api } = (await import("@/lib/api")) as unknown as {
  api: { getPipeline: Mock; listPipelineRuns: Mock; triggerPipelineRun: Mock };
};

function run(id: number, file: string) {
  return {
    id,
    file,
    sha: "abcdef1234567890abcdef1234567890abcdef12",
    ref: "main",
    trigger_by: "alice",
    status: "success" as const,
    steps_total: 2,
    steps_done: 2,
    event: "push",
    created_at: "2026-01-01T00:00:00Z",
    finished_at: "2026-01-01T00:00:05Z",
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  api.getPipeline.mockResolvedValue({
    enabled: true,
    file: ".gitdash.yml",
    files: [".gitdash.yml", ".gitdash/ci.yml"],
  });
  api.listPipelineRuns.mockResolvedValue([run(2, ".gitdash/ci.yml")]);
  api.triggerPipelineRun.mockResolvedValue({
    runs: [run(3, ".gitdash.yml"), run(4, ".gitdash/ci.yml")],
  });
});

function renderPipeline() {
  return render(
    <I18nProvider>
      <RepoPipeline owner="alice" name="demo" role="owner" />
    </I18nProvider>,
  );
}

describe("RepoPipeline multi-file", () => {
  it("lists pipeline files and shows the run's file in the table", async () => {
    renderPipeline();
    const table = await screen.findByRole("table");
    await waitFor(() =>
      expect(within(table).getByText(".gitdash/ci.yml")).toBeInTheDocument(),
    );
    // 文件下拉包含两个候选文件
    const combo = screen.getByRole("combobox");
    expect(within(combo).getByRole("option", { name: ".gitdash.yml" })).toBeInTheDocument();
    expect(within(combo).getByRole("option", { name: ".gitdash/ci.yml" })).toBeInTheDocument();
  });

  it("triggers the selected file and prepends all returned runs", async () => {
    const user = userEvent.setup();
    renderPipeline();
    await screen.findByRole("table");

    const combo = screen.getByRole("combobox");
    await user.selectOptions(combo, ".gitdash/ci.yml");
    await user.click(screen.getByRole("button", { name: /run now/i }));

    await waitFor(() =>
      expect(api.triggerPipelineRun).toHaveBeenCalledWith("alice", "demo", {
        file: ".gitdash/ci.yml",
      }),
    );
    // 两个新运行都进入列表
    const table = screen.getByRole("table");
    await waitFor(() => expect(within(table).getByText("3")).toBeInTheDocument());
    expect(within(table).getByText("4")).toBeInTheDocument();
  });

  it("offers an 'all pipelines' option that triggers without a file filter", async () => {
    const user = userEvent.setup();
    renderPipeline();
    await screen.findByRole("table");

    const combo = screen.getByRole("combobox");
    expect(within(combo).getByRole("option", { name: /all pipelines/i })).toBeInTheDocument();
    await user.selectOptions(combo, "");
    await user.click(screen.getByRole("button", { name: /run now/i }));

    await waitFor(() =>
      expect(api.triggerPipelineRun).toHaveBeenCalledWith("alice", "demo", {}),
    );
  });

  it("sends the delay and shows a delayed pending run's schedule", async () => {
    api.listPipelineRuns.mockResolvedValue([
      { ...run(9, ".gitdash.yml"), status: "pending", run_at: "2026-01-01T01:00:00Z" },
    ]);
    const user = userEvent.setup();
    renderPipeline();

    const table = await screen.findByRole("table");
    expect(within(table).getByText(/starts/i)).toBeInTheDocument();

    await user.type(screen.getByPlaceholderText(/delay/i), "5m");
    await user.click(screen.getByRole("button", { name: /run now/i }));
    await waitFor(() =>
      expect(api.triggerPipelineRun).toHaveBeenCalledWith("alice", "demo", {
        file: ".gitdash.yml",
        delay: "5m",
      }),
    );
  });

  it("renders the pipeline documentation tabs", async () => {
    const user = userEvent.setup();
    renderPipeline();
    await screen.findByRole("table");

    expect(screen.getByText("Pipeline documentation")).toBeInTheDocument();
    await user.click(screen.getByRole("tab", { name: /dsl fields/i }));
    expect(await screen.findByText("job_timeout")).toBeInTheDocument();

    await user.click(screen.getByRole("tab", { name: /lifecycle/i }));
    expect(await screen.findByText(/cancel stops/i)).toBeInTheDocument();
  });
});
