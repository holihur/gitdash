import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Toaster } from "sonner";
import { I18nProvider } from "@/lib/i18n";
import ImportRepoDialog from "@/pages/repos/ImportRepoDialog";
import { vi } from "vitest";

function jsonRes(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
}

const connections = [
  { provider: "gitea", label: "Gitea", enabled: true, connected: true, login: "alice", base_url: "https://gitea.example.com" },
];
const repos = [
  { full_name: "alice/repo1", name: "repo1", owner: "alice", private: false, clone_url: "https://gitea/alice/repo1.git", default_branch: "main", description: "first" },
  { full_name: "alice/repo2", name: "repo2", owner: "alice", private: true, clone_url: "https://gitea/alice/repo2.git", default_branch: "main", description: "second" },
];

function renderDialog(onBatchDone: () => void, onPost: (body: unknown) => void) {
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith("/api/imports/batch")) {
        onPost(JSON.parse(String(init?.body)));
        return jsonRes({ imported: ["alice/repo1"], skipped: {} }, 202);
      }
      if (url.endsWith("/api/connections/gitea/repos")) return jsonRes(repos);
      if (url.endsWith("/api/connections")) return jsonRes(connections);
      return jsonRes({}, 404);
    }),
  );
  return render(
    <I18nProvider>
      <ImportRepoDialog
        open
        onOpenChange={() => undefined}
        url=""
        onUrl={() => undefined}
        name=""
        onName={() => undefined}
        privateRepo
        onPrivate={() => undefined}
        key=""
        onKey={() => undefined}
        busy={false}
        onImport={() => undefined}
        onBatchDone={onBatchDone}
      />
      <Toaster />
    </I18nProvider>,
  );
}

beforeEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
});

describe("ImportRepoDialog (batch import)", () => {
  it("从已绑定账号勾选仓库并批量导入", async () => {
    const done = vi.fn();
    const posted: unknown[] = [];
    renderDialog(done, (b) => posted.push(b));

    await userEvent.click(await screen.findByRole("tab", { name: "From connected account" }));

    // 加载远程仓库列表
    expect(await screen.findByText("alice/repo1")).toBeInTheDocument();
    expect(screen.getByText("alice/repo2")).toBeInTheDocument();
    expect(screen.getByText("private")).toBeInTheDocument();

    // 勾选第一个仓库并导入
    await userEvent.click(screen.getByText("alice/repo1"));
    await userEvent.click(screen.getByRole("button", { name: /Import 1/ }));

    await waitFor(() => expect(posted.length).toBe(1));
    expect(posted[0]).toMatchObject({ provider: "gitea", private: true, repos: ["alice/repo1"] });
    await waitFor(() => expect(done).toHaveBeenCalled());
  });

  it("未绑定任何账号时显示引导", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        if (String(input).endsWith("/api/connections")) return jsonRes([]);
        return jsonRes({}, 404);
      }),
    );
    render(
      <I18nProvider>
        <ImportRepoDialog
          open
          onOpenChange={() => undefined}
          url=""
          onUrl={() => undefined}
          name=""
          onName={() => undefined}
          privateRepo
          onPrivate={() => undefined}
          key=""
          onKey={() => undefined}
          busy={false}
          onImport={() => undefined}
        />
      </I18nProvider>,
    );

    await userEvent.click(await screen.findByRole("tab", { name: "From connected account" }));
    expect(await screen.findByText("No connected accounts yet.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Link an account to import repositories in bulk" })).toBeInTheDocument();
  });
});
