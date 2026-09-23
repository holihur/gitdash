import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { I18nProvider } from "@/lib/i18n";
import OrgSettings from "@/pages/OrgSettings";

vi.mock("@/lib/api", () => ({
  ApiError: class ApiError extends Error {},
  api: {
    getOrgProfile: vi.fn(),
    updateOrg: vi.fn(),
    uploadOrgCover: vi.fn(),
    deleteOrgCover: vi.fn(),
  },
}));

const { api } = (await import("@/lib/api")) as unknown as {
  api: {
    getOrgProfile: Mock;
    updateOrg: Mock;
    uploadOrgCover: Mock;
    deleteOrgCover: Mock;
  };
};

const ownerProfile = {
  name: "acme",
  display: "ACME Inc",
  bio: "we build things",
  created_at: "2026-01-01T00:00:00Z",
  role: "owner",
  members: [],
  repos: [],
  followers: 0,
  is_following: false,
};

function renderPage() {
  return render(
    <I18nProvider>
      <MemoryRouter initialEntries={["/orgs/acme/settings"]}>
        <Routes>
          <Route path="/orgs/:org/settings" element={<OrgSettings />} />
        </Routes>
      </MemoryRouter>
    </I18nProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  api.getOrgProfile.mockResolvedValue(ownerProfile);
});

describe("OrgSettings", () => {
  it("渲染并保存显示名与简介", async () => {
    const user = userEvent.setup();
    api.updateOrg.mockResolvedValue({ ...ownerProfile, display: "ACME Corp", bio: "new bio" });
    renderPage();

    const display = await screen.findByLabelText("Display name");
    expect(display).toHaveValue("ACME Inc");
    await user.clear(display);
    await user.type(display, "ACME Corp");

    const bio = screen.getByLabelText("Description / bio");
    await user.clear(bio);
    await user.type(bio, "new bio");

    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(api.updateOrg).toHaveBeenCalledWith("acme", { display: "ACME Corp", bio: "new bio" }),
    );
  });

  it("非 owner 只看到提示，不显示表单", async () => {
    api.getOrgProfile.mockResolvedValue({ ...ownerProfile, role: "member" });
    renderPage();

    expect(
      await screen.findByText("Only the organization owner can edit these settings."),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText("Display name")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
  });

  it("已设置封面时可移除", async () => {
    const user = userEvent.setup();
    api.getOrgProfile.mockResolvedValue({ ...ownerProfile, cover_url: "/orgs/acme/cover" });
    api.deleteOrgCover.mockResolvedValue({ deleted: true });
    renderPage();

    await user.click(await screen.findByRole("button", { name: /remove/i }));
    await waitFor(() => expect(api.deleteOrgCover).toHaveBeenCalledWith("acme"));
  });
});
