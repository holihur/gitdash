import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { I18nProvider } from "@/lib/i18n";
import { BadgesSection } from "@/admin/sections/BadgesSection";

vi.mock("@/admin/api", () => ({
  adminList: vi.fn(),
  adminReq: vi.fn(),
  adminUpload: vi.fn(),
  toastError: vi.fn(),
}));

const { adminList, adminReq, adminUpload } = (await import("@/admin/api")) as unknown as {
  adminList: Mock;
  adminReq: Mock;
  adminUpload: Mock;
};

const badge = {
  id: 1,
  slug: "verified",
  label: "Verified",
  description: "verified account",
  has_image: false,
  created_at: "2026-01-01T00:00:00Z",
};

function renderSection() {
  render(
    <I18nProvider>
      <BadgesSection />
    </I18nProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  adminList.mockResolvedValue({ items: [badge], total: 1 });
  adminReq.mockResolvedValue([]);
  adminUpload.mockResolvedValue(badge);
});

describe("BadgesSection", () => {
  it("列出徽章并创建", async () => {
    const user = userEvent.setup();
    renderSection();

    expect(await screen.findByText("Verified")).toBeInTheDocument();

    await user.type(screen.getByPlaceholderText("Badge name"), "Staff");
    await user.click(screen.getByRole("button", { name: /create badge/i }));

    await waitFor(() => expect(adminUpload).toHaveBeenCalled());
    const [path, form] = adminUpload.mock.calls[0] as [string, FormData];
    expect(path).toBe("/badges");
    expect(form.get("label")).toBe("Staff");
  });

  it("更新 emoji 并移除图标", async () => {
    adminList.mockResolvedValue({
      items: [{ ...badge, emoji: "🏅", has_image: true, image_updated_at: "t" }],
      total: 1,
    });
    const user = userEvent.setup();
    renderSection();
    await screen.findByText("Verified");

    const emojiInput = screen.getByLabelText("Emoji (fallback when no image)");
    await user.clear(emojiInput);
    await user.type(emojiInput, "⭐");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(adminReq).toHaveBeenCalledWith("/badges/1", { emoji: "⭐" }, "PATCH"),
    );

    await user.click(screen.getByRole("button", { name: /remove image/i }));
    await waitFor(() =>
      expect(adminReq).toHaveBeenCalledWith("/badges/1/image", undefined, "DELETE"),
    );
  });

  it("展开授予记录并授予 / 撤销", async () => {
    const user = userEvent.setup();
    renderSection();
    await screen.findByText("Verified");

    // 展开 -> GET grants
    await user.click(screen.getByRole("button", { name: "Grants" }));
    await waitFor(() => expect(adminReq).toHaveBeenCalledWith("/badges/1/grants"));

    await user.type(screen.getByPlaceholderText("User / org / repo owner"), "alice");
    adminReq.mockResolvedValue([
      { badge_id: 1, label: "Verified", kind: "user", owner: "alice", repo: "", created_at: "" },
    ]);
    await user.click(screen.getByRole("button", { name: "Grant" }));
    await waitFor(() =>
      expect(adminReq).toHaveBeenCalledWith("/badges/1/grants", {
        kind: "user",
        owner: "alice",
        repo: "",
      }),
    );

    // 撤销
    await waitFor(() => expect(screen.getByText("alice")).toBeInTheDocument());
    await user.click(screen.getByTitle("Revoke"));
    await waitFor(() =>
      expect(adminReq).toHaveBeenCalledWith(
        expect.stringContaining("/badges/1/grants?"),
        undefined,
        "DELETE",
      ),
    );
  });
});
