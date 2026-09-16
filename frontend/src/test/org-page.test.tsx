import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { I18nProvider } from "@/lib/i18n";
import OrgPage from "@/pages/OrgPage";

vi.mock("@/lib/api", () => ({
  ApiError: class ApiError extends Error {},
  api: {
    getOrgProfile: vi.fn(),
    followOrg: vi.fn(),
    unfollowOrg: vi.fn(),
    listOrgFollowers: vi.fn(),
    addOrgMember: vi.fn(),
    removeOrgMember: vi.fn(),
    deleteOrg: vi.fn(),
  },
}));

const { api } = (await import("@/lib/api")) as unknown as {
  api: {
    getOrgProfile: Mock;
    followOrg: Mock;
    unfollowOrg: Mock;
    listOrgFollowers: Mock;
  };
};

const baseProfile = {
  name: "acme",
  display: "ACME Inc",
  created_at: "2026-01-01T00:00:00Z",
  role: "",
  members: [{ org: "acme", username: "alice", role: "owner" }],
  repos: [],
  followers: 0,
  is_following: false,
};

function renderPage() {
  return render(
    <I18nProvider>
      <MemoryRouter initialEntries={["/orgs/acme"]}>
        <Routes>
          <Route path="/orgs/:org" element={<OrgPage />} />
        </Routes>
      </MemoryRouter>
    </I18nProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  api.getOrgProfile.mockResolvedValue(baseProfile);
  api.listOrgFollowers.mockResolvedValue([{ username: "bobby", created_at: "2026-01-02T00:00:00Z" }]);
});

describe("OrgPage", () => {
  it("renders the org profile and follows/unfollows", async () => {
    const user = userEvent.setup();
    api.followOrg.mockResolvedValue({ followers: 1, is_following: true });
    api.unfollowOrg.mockResolvedValue({ followers: 0, is_following: false });

    renderPage();

    await waitFor(() => expect(screen.getByText("ACME Inc")).toBeInTheDocument());

    await user.click(screen.getByRole("button", { name: /^follow$/i }));
    await waitFor(() => expect(api.followOrg).toHaveBeenCalledWith("acme"));
    expect(await screen.findByRole("button", { name: /unfollow/i })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /unfollow/i }));
    await waitFor(() => expect(api.unfollowOrg).toHaveBeenCalledWith("acme"));
  });

  it("lists followers in the followers tab", async () => {
    const user = userEvent.setup();
    renderPage();

    await waitFor(() => expect(screen.getByText("ACME Inc")).toBeInTheDocument());
    await user.click(screen.getByRole("button", { name: /followers/i }));

    await waitFor(() => expect(api.listOrgFollowers).toHaveBeenCalledWith("acme"));
    expect(await screen.findByText("bobby")).toBeInTheDocument();
  });

  it("shows members and management controls for owners", async () => {
    const user = userEvent.setup();
    api.getOrgProfile.mockResolvedValue({ ...baseProfile, role: "owner" });
    renderPage();

    await waitFor(() => expect(screen.getByText("ACME Inc")).toBeInTheDocument());
    // owner sees the delete button
    expect(screen.getByRole("button", { name: /delete organization/i })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /members/i }));
    expect(await screen.findByText("alice")).toBeInTheDocument();
  });
});
