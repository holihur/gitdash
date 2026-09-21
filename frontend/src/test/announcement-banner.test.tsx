import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "@/lib/i18n";
import { AnnouncementBanner } from "@/components/announcement-banner";
import { vi } from "vitest";

function jsonRes(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

function renderBanner() {
  return render(
    <I18nProvider>
      <AnnouncementBanner />
    </I18nProvider>,
  );
}

beforeEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
});

describe("AnnouncementBanner", () => {
  it("未启用时不展示", async () => {
    vi.stubGlobal("fetch", vi.fn(() => jsonRes({ enabled: false })));
    renderBanner();
    await waitFor(() => expect(fetch).toHaveBeenCalled());
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("展示标题与内容，并可关闭（按 id 记忆）", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        jsonRes({
          enabled: true,
          id: "abc",
          level: "warning",
          title: "Scheduled maintenance",
          message: "Tonight 22:00 UTC",
        }),
      ),
    );
    renderBanner();
    expect(await screen.findByText("Scheduled maintenance")).toBeInTheDocument();
    expect(screen.getByText("Tonight 22:00 UTC")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    await waitFor(() =>
      expect(screen.queryByText("Scheduled maintenance")).not.toBeInTheDocument(),
    );
    expect(localStorage.getItem("gitdash-announcement-dismissed")).toBe("abc");
  });

  it("已关闭的同一公告（相同 id）不再展示", async () => {
    localStorage.setItem("gitdash-announcement-dismissed", "abc");
    vi.stubGlobal(
      "fetch",
      vi.fn(() => jsonRes({ enabled: true, id: "abc", title: "Scheduled maintenance", message: "Tonight" })),
    );
    renderBanner();
    await waitFor(() => expect(fetch).toHaveBeenCalled());
    expect(screen.queryByText("Scheduled maintenance")).not.toBeInTheDocument();
  });

  it("公告内容更新（新 id）后重新展示", async () => {
    localStorage.setItem("gitdash-announcement-dismissed", "old");
    vi.stubGlobal(
      "fetch",
      vi.fn(() => jsonRes({ enabled: true, id: "new", title: "Updated notice", message: "New" })),
    );
    renderBanner();
    expect(await screen.findByText("Updated notice")).toBeInTheDocument();
  });
});
