import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";
import { I18nProvider } from "@/lib/i18n";
import CollaboratorsDialog from "@/components/collabs-dialog";

vi.mock("@/lib/api", () => ({
  api: { listCollabs: vi.fn(), addCollab: vi.fn(), removeCollab: vi.fn() },
}));

const { api } = (await import("@/lib/api")) as unknown as {
  api: { listCollabs: Mock; addCollab: Mock; removeCollab: Mock };
};

beforeEach(() => {
  vi.clearAllMocks();
  api.listCollabs.mockResolvedValue([]);
  api.addCollab.mockResolvedValue({});
});

describe("CollaboratorsDialog roles", () => {
  it("权限下拉提供五档角色", async () => {
    render(
      <I18nProvider>
        <CollaboratorsDialog open onOpenChange={() => {}} owner="alice" repo="demo" />
      </I18nProvider>,
    );
    const select = await screen.findByLabelText("Permission");
    const values = Array.from(select.querySelectorAll("option")).map((o) => o.getAttribute("value"));
    expect(values).toEqual(["read", "triage", "write", "maintain", "admin"]);
  });

  it("以选定角色添加协作者", async () => {
    const user = userEvent.setup();
    render(
      <I18nProvider>
        <CollaboratorsDialog open onOpenChange={() => {}} owner="alice" repo="demo" />
      </I18nProvider>,
    );
    await user.type(screen.getByLabelText("Username"), "bob");
    await user.selectOptions(screen.getByLabelText("Permission"), "maintain");
    await user.click(screen.getByRole("button", { name: /add collaborator/i }));
    await waitFor(() =>
      expect(api.addCollab).toHaveBeenCalledWith("alice", "demo", "bob", "maintain"),
    );
  });
});
