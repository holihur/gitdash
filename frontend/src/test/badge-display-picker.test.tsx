import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { I18nProvider } from "@/lib/i18n";
import { BadgeDisplayPicker } from "@/components/badge-display-picker";

vi.mock("@/lib/api", () => ({
  badgeImageUrl: () => undefined,
  api: { getOwned: vi.fn(), setDisplay: vi.fn() },
}));

const { api } = (await import("@/lib/api")) as unknown as {
  api: { getOwned: Mock; setDisplay: Mock };
};

const badge = (id: number, label: string) => ({
  id,
  slug: `b${id}`,
  label,
  description: "",
  has_image: false,
  created_at: "2026-01-01T00:00:00Z",
});

function renderPicker() {
  render(
    <I18nProvider>
      <BadgeDisplayPicker kind="user" owner="alice" />
    </I18nProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  api.getOwned.mockResolvedValue({
    granted: [badge(1, "A"), badge(2, "B"), badge(3, "C"), badge(4, "D")],
    displayed: [1],
    max: 3,
  });
  api.setDisplay.mockResolvedValue({ displayed: [] });
});

describe("BadgeDisplayPicker", () => {
  it("只列出已授予徽章并可保存挂出选择", async () => {
    const user = userEvent.setup();
    renderPicker();

    const b = await screen.findByRole("button", { name: /B$/ });
    await user.click(b);
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(api.setDisplay).toHaveBeenCalledWith("user", "alice", undefined, [1, 2]),
    );
  });

  it("超过上限时不再选中", async () => {
    const user = userEvent.setup();
    api.getOwned.mockResolvedValue({
      granted: [badge(1, "A"), badge(2, "B"), badge(3, "C"), badge(4, "D")],
      displayed: [1, 2, 3],
      max: 3,
    });
    renderPicker();

    await screen.findByRole("button", { name: /A$/ });
    await user.click(screen.getByRole("button", { name: /D$/ }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(api.setDisplay).toHaveBeenCalledWith("user", "alice", undefined, [1, 2, 3]),
    );
  });

  it("没有获得徽章时不渲染", async () => {
    api.getOwned.mockResolvedValue({ granted: [], displayed: [], max: 3 });
    const { container } = render(
      <I18nProvider>
        <BadgeDisplayPicker kind="org" owner="acme" />
      </I18nProvider>,
    );
    await waitFor(() => expect(api.getOwned).toHaveBeenCalled());
    expect(container.firstChild).toBeNull();
  });
});
