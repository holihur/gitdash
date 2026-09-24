import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { I18nProvider } from "@/lib/i18n";
import { RepoTeamAccess } from "@/components/repo-team-access";

vi.mock("@/lib/api", () => ({
  api: {
    listOrgTeams: vi.fn(),
    repoTeamGrants: vi.fn(),
    repoAccess: vi.fn(),
    grantRepoTeam: vi.fn(),
    revokeRepoTeam: vi.fn(),
  },
}));

const { api } = (await import("@/lib/api")) as unknown as {
  api: {
    listOrgTeams: Mock;
    repoTeamGrants: Mock;
    repoAccess: Mock;
    grantRepoTeam: Mock;
    revokeRepoTeam: Mock;
  };
};

function renderCard() {
  render(
    <I18nProvider>
      <RepoTeamAccess owner="acme" name="api" />
    </I18nProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  api.listOrgTeams.mockResolvedValue([
    { id: 1, org: "acme", name: "devs", member_count: 1, created_at: "" },
  ]);
  api.repoTeamGrants.mockResolvedValue([]);
  api.repoAccess.mockResolvedValue([{ subject: "bob", role: "write", sources: ["team:devs"] }]);
  api.grantRepoTeam.mockResolvedValue(null);
});

describe("RepoTeamAccess", () => {
  it("展示权限审计条目并按团队授权", async () => {
    const user = userEvent.setup();
    renderCard();

    expect(await screen.findByText("bob")).toBeInTheDocument();
    expect(screen.getByText("team:devs")).toBeInTheDocument();

    await user.selectOptions(screen.getByLabelText("Select team"), "1");
    await user.selectOptions(screen.getByLabelText("Permission"), "maintain");
    await user.click(screen.getByRole("button", { name: "Grant" }));

    await waitFor(() =>
      expect(api.grantRepoTeam).toHaveBeenCalledWith("acme", "api", 1, "maintain"),
    );
  });
});
