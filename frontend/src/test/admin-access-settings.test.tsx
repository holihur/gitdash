import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "@/lib/i18n";
import { ThemeProvider } from "@/lib/theme";
import { AccessSettings } from "@/admin/AccessSettings";
import { vi } from "vitest";
import type { Settings } from "@/admin/api";

function settings(overrides: Partial<Settings> = {}): Settings {
  return {
    password_login_enabled: true,
    swagger_enabled: true,
    ...overrides,
  } as Settings;
}

function renderAccess(overrides: Partial<Settings> = {}, onChange = () => undefined) {
  return render(
    <ThemeProvider>
      <I18nProvider>
        <AccessSettings settings={settings(overrides)} onChange={onChange} />
      </I18nProvider>
    </ThemeProvider>,
  );
}

beforeEach(() => {
  vi.restoreAllMocks();
});

describe("AccessSettings", () => {
  it("按当前设置渲染开关", () => {
    renderAccess({ password_login_enabled: false, swagger_enabled: true });
    expect(screen.getByRole("checkbox", { name: /account & password login/i })).not.toBeChecked();
    expect(screen.getByRole("checkbox", { name: /swagger/i })).toBeChecked();
  });

  it("保存时提交两个开关", async () => {
    const onChange = vi.fn();
    const fetchMock = vi.fn(() =>
      Promise.resolve(new Response(JSON.stringify({ ok: true }), { status: 200 })),
    );
    vi.stubGlobal("fetch", fetchMock);
    renderAccess({ password_login_enabled: true, swagger_enabled: true }, onChange);

    await userEvent.click(screen.getByRole("checkbox", { name: /swagger/i }));
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onChange).toHaveBeenCalled());
    const body = JSON.parse(String((fetchMock.mock.calls[0][1] as RequestInit).body));
    expect(body).toEqual({ password_login_enabled: true, swagger_enabled: false });
  });
});
