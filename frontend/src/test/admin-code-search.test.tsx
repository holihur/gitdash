import { render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "@/lib/i18n";
import { ThemeProvider } from "@/lib/theme";
import { CodeSearchSection } from "@/admin/sections/CodeSearchSection";
import { vi } from "vitest";

function renderComp() {
  return render(
    <ThemeProvider>
      <I18nProvider>
        <CodeSearchSection />
      </I18nProvider>
    </ThemeProvider>,
  );
}

describe("CodeSearchSection", () => {
  it("渲染代码搜索指标", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              backend: "bleve",
              remote: false,
              search_requests: { index: 12, grep: 3 },
              index_runs: { full: 1, incremental: 9, ok: 10, error: 0 },
              index: { repos: 4, documents: 500, dirty: 1 },
              index_avg_duration_ms: 42.5,
              last_index_at: 1700000000,
            }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
        ),
      ),
    );
    renderComp();

    await waitFor(() => expect(screen.getByText("Code search index")).toBeInTheDocument());
    expect(screen.getByText("bleve")).toBeInTheDocument();
    expect(screen.getByText("500")).toBeInTheDocument(); // documents
    expect(screen.getByText("43 ms")).toBeInTheDocument(); // avg duration
    expect(screen.getByText("Indexed documents")).toBeInTheDocument();
    expect(screen.getByText("Pending rebuild")).toBeInTheDocument();
    vi.restoreAllMocks();
  });
});
