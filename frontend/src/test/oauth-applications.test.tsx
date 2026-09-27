import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { I18nProvider, preloadLang } from "@/lib/i18n";
import Applications from "@/pages/Applications";
import { zhCN } from "@/locales/zh-CN";

vi.mock("@/lib/api", () => ({
  ApiError: class ApiError extends Error {},
  api: {
    listApps: vi.fn(),
    createApp: vi.fn(),
    deleteApp: vi.fn(),
    resetSecret: vi.fn(),
    listAuthorizations: vi.fn(),
    revokeAuthorization: vi.fn(),
  },
}));

const { api } = (await import("@/lib/api")) as unknown as {
  api: {
    listApps: Mock;
    createApp: Mock;
    deleteApp: Mock;
    listAuthorizations: Mock;
  };
};

const APP = {
  id: 7,
  name: "ci-bot",
  client_id: "Iv1.abcdef",
  callback_url: "https://example.com/oauth/callback",
  created_at: "2026-01-01T00:00:00Z",
};

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.removeItem("gitdash-lang");
  api.listApps.mockResolvedValue([]);
  api.listAuthorizations.mockResolvedValue([]);
});

/** 切到指定语言渲染（语言包是动态 import，调用方需先 preloadLang）。 */
async function renderIn(lang: string | null) {
  if (lang) {
    localStorage.setItem("gitdash-lang", lang);
    await preloadLang(lang as "zh-CN");
  }
  return render(
    <I18nProvider>
      <Applications />
    </I18nProvider>,
  );
}

describe("OAuth applications page", () => {
  it("registers an application with trimmed values and shows the client secret", async () => {
    const user = userEvent.setup();
    api.createApp.mockResolvedValue({ ...APP, client_secret: "gd_oauth_secret" });
    await renderIn(null);

    await user.click(await screen.findByRole("button", { name: /new oauth app/i }));

    const dialog = await screen.findByRole("dialog");
    await user.type(within(dialog).getByLabelText(/application name/i), "ci-bot");
    await user.type(within(dialog).getByLabelText(/callback url/i), "https://example.com/oauth/callback");
    await user.click(within(dialog).getByRole("button", { name: /register application/i }));

    expect(api.createApp).toHaveBeenCalledWith({
      name: "ci-bot",
      homepage: "",
      description: "",
      callback_url: "https://example.com/oauth/callback",
    });
    await waitFor(() => expect(screen.getByText("gd_oauth_secret")).toBeInTheDocument());
  });

  it("keeps redirect_uri as a code token inside the translated hint", async () => {
    const user = userEvent.setup();
    await renderIn(null);

    await user.click(await screen.findByRole("button", { name: /new oauth app/i }));

    const dialog = await screen.findByRole("dialog");
    // 令牌必须仍被包在 <code> 里（withCodeToken 按字面量切分译文）。
    expect(within(dialog).getByText("redirect_uri", { selector: "code" })).toBeInTheDocument();
    expect(dialog).toHaveTextContent("Callback URL must exactly match the redirect_uri used");
  });

  it("interpolates the app name into the delete confirmation", async () => {
    const user = userEvent.setup();
    api.listApps.mockResolvedValue([APP]);
    await renderIn(null);

    await user.click(await screen.findByRole("button", { name: "Delete" }));

    const dialog = await screen.findByRole("alertdialog");
    expect(dialog).toHaveTextContent(`Delete OAuth application?`);
    expect(dialog).toHaveTextContent(`This permanently deletes "ci-bot"`);
    await user.click(within(dialog).getByRole("button", { name: /^delete$/i }));

    expect(api.deleteApp).toHaveBeenCalledWith(7);
  });

  it("renders in Chinese instead of falling back to hardcoded English", async () => {
    const user = userEvent.setup();
    api.listApps.mockResolvedValue([APP]);
    await renderIn("zh-CN");

    // 页面标题、说明、表格列头都走 i18n（回归：这些文案曾是硬编码英文）。
    expect(await screen.findByRole("heading", { name: zhCN.oauthApps.title })).toBeInTheDocument();
    expect(screen.getByText(zhCN.oauthApps.subtitle)).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: zhCN.oauthApps.clientId })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: zhCN.oauthApps.resetSecret })).toBeInTheDocument();

    // 删除确认框里的应用名插值
    await user.click(screen.getByRole("button", { name: zhCN.common.delete }));
    const confirm = await screen.findByRole("alertdialog");
    expect(confirm).toHaveTextContent(zhCN.oauthApps.deleteTitle);
    expect(confirm).toHaveTextContent("ci-bot");
    await user.click(within(confirm).getByRole("button", { name: zhCN.common.cancel }));

    // 已授权应用页签
    await user.click(screen.getByRole("tab", { name: zhCN.oauthApps.tabAuthorized }));
    expect(await screen.findByText(zhCN.oauthApps.authorizedEmpty)).toBeInTheDocument();
  });
});
