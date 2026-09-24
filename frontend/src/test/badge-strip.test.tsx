import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi, type Mock } from "vitest";
import { I18nProvider } from "@/lib/i18n";
import { BadgeStrip } from "@/components/badge-strip";

vi.mock("@/lib/api", () => ({
  badgeImageUrl: () => undefined,
  api: { getBadges: vi.fn() },
}));

const { api } = (await import("@/lib/api")) as unknown as { api: { getBadges: Mock } };

const badge = (id: number, label: string, emoji?: string) => ({
  id,
  slug: label.toLowerCase(),
  label,
  description: `${label} desc`,
  emoji,
  has_image: false,
  created_at: "2026-01-01T00:00:00Z",
});

describe("BadgeStrip", () => {
  it("渲染目标挂出的徽章", async () => {
    api.getBadges.mockResolvedValue([badge(1, "Verified", "🏅"), badge(2, "Staff")]);
    render(
      <I18nProvider>
        <BadgeStrip kind="user" owner="alice" />
      </I18nProvider>,
    );
    expect(await screen.findByText("Verified")).toBeInTheDocument();
    expect(screen.getByText("Staff")).toBeInTheDocument();
    // emoji 兜底在没有图标时展示。
    expect(screen.getByText("🏅")).toBeInTheDocument();
    expect(api.getBadges).toHaveBeenCalledWith("user", "alice", undefined);
  });

  it("没有徽章时不渲染", async () => {
    api.getBadges.mockResolvedValue([]);
    const { container } = render(
      <I18nProvider>
        <BadgeStrip kind="repo" owner="alice" repo="demo" />
      </I18nProvider>,
    );
    await waitFor(() => expect(api.getBadges).toHaveBeenCalled());
    expect(container.querySelector("[title]")).toBeNull();
  });
});
