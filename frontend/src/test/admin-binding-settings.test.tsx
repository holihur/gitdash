import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "@/lib/i18n";
import { BindingSettings } from "@/admin/BindingSettings";
import type { Settings } from "@/admin/api";
import { vi } from "vitest";

function jsonRes(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }),
  );
}

const settings = {
  gitlab_enabled: false,
  gitlab_client_id: "",
  gitlab_has_secret: false,
  gitlab_base_url: "https://gitlab.com",
  gitea_enabled: false,
  gitea_client_id: "",
  gitea_has_secret: false,
  gitea_base_url: "",
  bitbucket_enabled: false,
  bitbucket_client_id: "",
  bitbucket_has_secret: false,
} as unknown as Settings;

beforeEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
});

describe("admin BindingSettings", () => {
  it("保存 GitLab 绑定配置并展示回调地址", async () => {
    const posted: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
        posted.push(JSON.parse(String(init?.body)));
        return jsonRes({ ok: true });
      }),
    );
    const onChange = vi.fn();
    render(
      <I18nProvider>
        <BindingSettings settings={settings} onChange={onChange} />
      </I18nProvider>,
    );

    expect(screen.getByText(/api\/connections\/gitlab\/callback/)).toBeInTheDocument();
    expect(screen.getByText(/api\/connections\/gitea\/callback/)).toBeInTheDocument();
    expect(screen.getByText(/api\/connections\/bitbucket\/callback/)).toBeInTheDocument();

    const [enableGitlab] = screen.getAllByText("Enable GitLab binding");
    await userEvent.click(enableGitlab);
    await userEvent.type(screen.getAllByLabelText("Client ID")[0], "gl-id");
    await userEvent.type(screen.getAllByLabelText("Client Secret")[0], "gl-secret");
    await userEvent.type(screen.getAllByLabelText("Base URL")[0], "/extra");

    await userEvent.click(screen.getAllByRole("button", { name: "Save" })[0]);

    await waitFor(() => expect(posted.length).toBe(1));
    expect(posted[0]).toMatchObject({
      gitlab_enabled: true,
      gitlab_client_id: "gl-id",
      gitlab_client_secret: "gl-secret",
    });
    await waitFor(() => expect(onChange).toHaveBeenCalled());
    expect(await screen.findByText("Saved")).toBeInTheDocument();
  });

  it("已设置密钥时留空不覆盖", async () => {
    const posted: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
        posted.push(JSON.parse(String(init?.body)));
        return jsonRes({ ok: true });
      }),
    );
    render(
      <I18nProvider>
        <BindingSettings
          settings={{ ...settings, gitea_enabled: true, gitea_client_id: "gt", gitea_has_secret: true, gitea_base_url: "https://gitea.example.com" } as unknown as Settings}
          onChange={() => undefined}
        />
      </I18nProvider>,
    );

    // 第 2 个 Save 是 Gitea；不改密钥直接保存 → 正文不含 secret
    await userEvent.click(screen.getAllByRole("button", { name: "Save" })[1]);
    await waitFor(() => expect(posted.length).toBe(1));
    expect(posted[0]).toMatchObject({ gitea_enabled: true, gitea_client_id: "gt" });
    expect((posted[0] as Record<string, unknown>).gitea_client_secret).toBeUndefined();
  });
});
