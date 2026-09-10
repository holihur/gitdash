import { describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";

import { I18nProvider } from "@/lib/i18n";
import { CommandPalette } from "@/components/command-palette";

vi.mock("@/lib/api", () => ({
  api: {
    listRepos: vi.fn().mockResolvedValue({
      items: [{ owner: "alice", name: "demo" }],
      total: 1,
    }),
  },
}));

describe("CommandPalette", () => {
  it("展示导航命令与仓库，并按输入过滤", async () => {
    render(
      <I18nProvider>
        <MemoryRouter>
          <CommandPalette open onOpenChange={() => {}} />
        </MemoryRouter>
      </I18nProvider>,
    );

    // 导航命令
    expect(screen.getByText("Repositories")).toBeInTheDocument();
    expect(screen.getByText("Packages")).toBeInTheDocument();

    // 异步加载的仓库命令
    await waitFor(() => expect(screen.getByText("alice/demo")).toBeInTheDocument());
  });
});
