import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { I18nProvider } from "@/lib/i18n";
import Keys from "@/pages/Keys";

vi.mock("@/lib/api", () => ({
  ApiError: class ApiError extends Error {},
  api: {
    listKeys: vi.fn(),
    createKey: vi.fn(),
    deleteKey: vi.fn(),
    listPATs: vi.fn(),
    createPAT: vi.fn(),
    deletePAT: vi.fn(),
  },
}));

const { api } = (await import("@/lib/api")) as unknown as {
  api: { listKeys: Mock; listPATs: Mock; createPAT: Mock };
};

const TOKEN = "gd_pat_0123456789abcdef0123456789abcdef";

beforeEach(() => {
  vi.clearAllMocks();
  api.listKeys.mockResolvedValue([]);
  api.listPATs.mockResolvedValue([]);
  api.createPAT.mockResolvedValue({
    id: 1,
    name: "ci",
    scopes: ["repo"],
    cidrs: [],
    expires_at: "",
    created_at: "2026-01-01T00:00:00Z",
    last_used_at: "",
    token: TOKEN,
  });
});

describe("PAT creation", () => {
  it("reveals the plaintext token in a dialog after creating", async () => {
    const user = userEvent.setup();
    render(
      <I18nProvider>
        <Keys />
      </I18nProvider>,
    );

    await user.click(screen.getByRole("tab", { name: /personal access tokens/i }));

    await user.click(await screen.findByRole("button", { name: /create token/i }));

    const dialog = await screen.findByRole("dialog");
    await user.type(within(dialog).getByLabelText(/name/i), "ci");
    await user.click(within(dialog).getByRole("button", { name: /create token/i }));

    // The token must be visible in its own dialog (regression: it used to be
    // rendered inside the create dialog, which was closed on submit).
    await waitFor(() => expect(screen.getByText(TOKEN)).toBeInTheDocument());
    expect(api.createPAT).toHaveBeenCalledWith("ci", ["repo"], [], "");
  });

  it("can dismiss the token dialog", async () => {
    const user = userEvent.setup();
    render(
      <I18nProvider>
        <Keys />
      </I18nProvider>,
    );

    await user.click(screen.getByRole("tab", { name: /personal access tokens/i }));
    await user.click(await screen.findByRole("button", { name: /create token/i }));

    const dialog = await screen.findByRole("dialog");
    await user.type(within(dialog).getByLabelText(/name/i), "ci");
    await user.click(within(dialog).getByRole("button", { name: /create token/i }));

    await waitFor(() => expect(screen.getByText(TOKEN)).toBeInTheDocument());

    await user.click(screen.getByRole("button", { name: /done/i }));

    await waitFor(() => expect(screen.queryByText(TOKEN)).not.toBeInTheDocument());
  });
});
