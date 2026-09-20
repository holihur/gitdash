import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Toaster } from "sonner";
import { I18nProvider } from "@/lib/i18n";
import { ConnectionsSection } from "@/pages/profile/ConnectionsSection";
import { vi } from "vitest";

function jsonRes(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(status === 204 ? null : JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

const connections = [
  { provider: "github", label: "GitHub", enabled: false, connected: false },
  { provider: "gitlab", label: "GitLab", enabled: true, connected: false },
  { provider: "gitea", label: "Gitea", enabled: true, connected: true, login: "alice", base_url: "https://gitea.example.com" },
  { provider: "bitbucket", label: "Bitbucket", enabled: true, connected: false },
];

function renderSection(handlers: Record<string, (url: string, init?: RequestInit) => Promise<Response>>) {
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      for (const key of Object.keys(handlers)) {
        if (url.includes(key)) return handlers[key](url, init);
      }
      return jsonRes({}, 404);
    }),
  );
  return render(
    <I18nProvider>
      <Toaster />
      <ConnectionsSection />
    </I18nProvider>,
  );
}

beforeEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
  window.history.replaceState({}, "", "/profile");
});

describe("ConnectionsSection", () => {
  it("展示各平台的绑定状态与禁用状态", async () => {
    renderSection({ "/connections": () => jsonRes(connections) });

    expect(await screen.findByText("Connected accounts")).toBeInTheDocument();
    expect(screen.getByText("GitHub")).toBeInTheDocument();
    expect(screen.getByText("Not configured")).toBeInTheDocument();
    // gitea 已绑定：显示登录名 + 解绑按钮
    expect(screen.getByText("alice")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Disconnect/ })).toBeInTheDocument();
    // github 未配置禁用；gitlab / bitbucket 已启用可绑定 → 3 个 Connect 按钮，1 个禁用
    const connectButtons = screen.getAllByRole("button", { name: /Connect/ });
    expect(connectButtons.length).toBe(3);
    expect(connectButtons.filter((b) => (b as HTMLButtonElement).disabled).length).toBe(1);
  });

  it("解绑调用 DELETE 并刷新", async () => {
    const calls: { url: string; method?: string }[] = [];
    renderSection({
      "/connections": (_url, init) => {
        calls.push({ url: "/connections", method: init?.method });
        if (init?.method === "DELETE") return jsonRes(null, 204);
        return jsonRes(connections);
      },
    });

    const disconnect = await screen.findByRole("button", { name: /Disconnect/ });
    await userEvent.click(disconnect);
    await waitFor(() => {
      const del = calls.find((c) => c.method === "DELETE");
      expect(del).toBeTruthy();
    });
    expect(await screen.findByText("gitea disconnected")).toBeInTheDocument();
  });

  it("OAuth 回调参数触发提示并清理 URL", async () => {
    window.history.replaceState({}, "", "/profile?connected=gitlab");
    renderSection({ "/connections": () => jsonRes(connections) });

    expect(await screen.findByText("gitlab connected")).toBeInTheDocument();
    await waitFor(() => expect(window.location.search).toBe(""));
  });
});
