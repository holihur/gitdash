import { render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "@/lib/i18n";
import { ThemeProvider } from "@/lib/theme";
import AdminApp from "@/admin/App";
import { vi } from "vitest";

function jsonRes(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

function fetchMock(handlers: Record<string, (url: string) => Promise<Response>>) {
  return vi.fn((input: RequestInfo | URL) => {
    const url = String(input);
    for (const key of Object.keys(handlers)) {
      if (url.includes(key)) return handlers[key](url);
    }
    return jsonRes({}, 404);
  });
}

function renderAdmin() {
  return render(
    <ThemeProvider>
      <I18nProvider>
        <AdminApp />
      </I18nProvider>
    </ThemeProvider>,
  );
}

beforeEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
});

describe("AdminApp", () => {
  it("未登录：显示登录表单", async () => {
    vi.stubGlobal(
      "fetch",
      fetchMock({ "/me": () => jsonRes({ error: "unauthorized" }, 401) }),
    );
    renderAdmin();
    await waitFor(() =>
      expect(screen.getByText("Sign in")).toBeInTheDocument(),
    );
  });

  it("管理面板未启用：显示 disabled 提示", async () => {
    vi.stubGlobal("fetch", fetchMock({ "/me": () => jsonRes({}, 404) }));
    renderAdmin();
    await waitFor(() =>
      expect(screen.getByText("Admin panel is not enabled")).toBeInTheDocument(),
    );
  });

  it("已登录：显示 Dashboard（signed in as）", async () => {
    const handlers: Record<string, (url: string) => Promise<Response>> = {
      "/me": () => jsonRes({ username: "root" }),
      "/settings": () => jsonRes({ github_enabled: false, oidc_enabled: false }),
      "/users": () => jsonRes([]),
      "/runners": () => jsonRes([]),
    };
    vi.stubGlobal("fetch", fetchMock(handlers));
    renderAdmin();
    await waitFor(() =>
      expect(screen.getByText("Signed in as root")).toBeInTheDocument(),
    );
  });

  it("已登录：展示 IP 黑名单条目", async () => {
    const handlers: Record<string, (url: string) => Promise<Response>> = {
      "/me": () => jsonRes({ username: "root" }),
      "/settings": () => jsonRes({ github_enabled: false, oidc_enabled: false }),
      "/users": () => jsonRes([]),
      "/runners": () => jsonRes([]),
      "/ip-bans": () =>
        Promise.resolve(
          new Response(
            JSON.stringify([
              {
                id: 1,
                cidr: "203.0.113.0/24",
                note: "abuse",
                created_by: "admin",
                created_at: "2026-01-01T00:00:00Z",
              },
            ]),
            { status: 200, headers: { "Content-Type": "application/json", "X-Total-Count": "1" } },
          ),
        ),
    };
    vi.stubGlobal("fetch", fetchMock(handlers));
    renderAdmin();
    await waitFor(() =>
      expect(screen.getByText("203.0.113.0/24")).toBeInTheDocument(),
    );
    expect(screen.getByText("IP blacklist")).toBeInTheDocument();
    expect(screen.getByText("abuse")).toBeInTheDocument();
  });
});
