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
