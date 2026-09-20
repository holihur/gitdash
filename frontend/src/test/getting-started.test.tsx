import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "@/lib/i18n";
import { loadInstanceInfo } from "@/lib/api";
import { GettingStarted } from "@/components/getting-started";
import { vi } from "vitest";

function jsonRes(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

beforeEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
});

describe("GettingStarted", () => {
  it("展示引导步骤与文档入口，可关闭并记住", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/api/instance")) {
          return jsonRes({ version: "dev", ssh_port: "2222", docs_url: "https://docs.example.com/" });
        }
        if (url.includes("/api/keys")) return jsonRes([]);
        return jsonRes({}, 404);
      }),
    );
    await loadInstanceInfo();

    render(
      <MemoryRouter>
        <I18nProvider>
          <GettingStarted hasRepos={false} />
        </I18nProvider>
      </MemoryRouter>,
    );

    expect(await screen.findByText("Getting started")).toBeInTheDocument();
    expect(screen.getByText("Create your first repository")).toBeInTheDocument();
    expect(await screen.findByText("Read the docs")).toBeInTheDocument();

    await userEvent.click(screen.getByTitle("Dismiss"));
    await waitFor(() => expect(screen.queryByText("Getting started")).not.toBeInTheDocument());
    expect(localStorage.getItem("gitdash.gettingStartedDismissed")).toBe("1");
  });

  it("已完成（有仓库且有公钥）时不展示", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/api/keys")) {
          return jsonRes([{ id: 1, name: "k", public_key: "ssh-ed25519 AAAA", fingerprint: "fp", created_at: "" }]);
        }
        return jsonRes({}, 404);
      }),
    );
    render(
      <MemoryRouter>
        <I18nProvider>
          <GettingStarted hasRepos />
        </I18nProvider>
      </MemoryRouter>,
    );
    await waitFor(() => expect(screen.queryByText("Getting started")).not.toBeInTheDocument());
  });
});
