import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { I18nProvider } from "@/lib/i18n";
import { MemberRoleCard } from "@/pages/repoview/settings/MemberRoleCard";
import type { Repo } from "@/lib/api";

vi.mock("@/lib/api", () => ({
  api: {
    getOrgProfile: vi.fn(),
    setRepoMemberRole: vi.fn(),
  },
}));

const { api } = (await import("@/lib/api")) as unknown as {
  api: {
    getOrgProfile: Mock;
    setRepoMemberRole: Mock;
  };
};

const repo: Repo = {
  id: 1,
  owner: "acme",
  name: "api",
  description: "",
  created_at: "",
  is_org: true,
  role: "admin",
};

function renderCard(setRepo = vi.fn()) {
  render(
    <I18nProvider>
      <MemberRoleCard owner="acme" name="api" repo={repo} setRepo={setRepo} />
    </I18nProvider>,
  );
  return setRepo;
}

beforeEach(() => {
  vi.clearAllMocks();
  api.getOrgProfile.mockResolvedValue({ default_member_role: "read" });
  api.setRepoMemberRole.mockResolvedValue({ ...repo, member_role: "write" });
});

describe("MemberRoleCard", () => {
  it("继承组织默认并保存覆盖角色", async () => {
    const user = userEvent.setup();
    const setRepo = renderCard();

    // 默认展示“继承组织默认（只读）”。
    expect(await screen.findByRole("option", { name: /Inherit organization default.*Read/i })).toBeInTheDocument();

    await user.selectOptions(screen.getByLabelText("Organization member role"), "write");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(api.setRepoMemberRole).toHaveBeenCalledWith("acme", "api", "write"),
    );
    expect(setRepo).toHaveBeenCalledWith(expect.objectContaining({ member_role: "write" }));
  });
});
