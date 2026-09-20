import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Toaster } from "sonner";
import { I18nProvider } from "@/lib/i18n";
import { FeedbackWidget } from "@/components/feedback-widget";
import { vi } from "vitest";

function jsonRes(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

function renderWidget() {
  return render(
    <I18nProvider>
      <FeedbackWidget />
      <Toaster />
    </I18nProvider>,
  );
}

beforeEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
});

describe("FeedbackWidget", () => {
  it("未启用时不显示浮动按钮", async () => {
    vi.stubGlobal("fetch", vi.fn(() => jsonRes({ enabled: false })));
    renderWidget();
    await waitFor(() => expect(fetch).toHaveBeenCalled());
    expect(screen.queryByRole("button", { name: "Feedback" })).not.toBeInTheDocument();
  });

  it("启用后点击按钮输入并提交反馈", async () => {
    const calls: { url: string; init?: RequestInit }[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        calls.push({ url: String(input), init });
        if (init?.method === "POST") return jsonRes({ url: "https://example/issues/7", number: 7 }, 201);
        return jsonRes({ enabled: true });
      }),
    );
    renderWidget();

    const button = await screen.findByRole("button", { name: "Feedback" });
    await userEvent.click(button);
    const textarea = await screen.findByPlaceholderText("Describe your feedback…");
    await userEvent.type(textarea, "The merge button is broken");
    await userEvent.click(screen.getByRole("button", { name: "Submit" }));

    await waitFor(() => {
      const post = calls.find((c) => c.init?.method === "POST");
      expect(post).toBeTruthy();
      expect(post!.url).toBe("/api/feedback");
      expect(JSON.parse(String(post!.init!.body))).toMatchObject({ body: "The merge button is broken" });
    });
    expect(await screen.findByText(/Feedback submitted as issue #7/)).toBeInTheDocument();
  });
});
