import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "@/lib/i18n";
import { ThemeProvider } from "@/lib/theme";
import { LanguageSettings } from "@/admin/LanguageSettings";
import { vi } from "vitest";

function jsonRes(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

function renderComp() {
  return render(
    <ThemeProvider>
      <I18nProvider>
        <LanguageSettings />
      </I18nProvider>
    </ThemeProvider>,
  );
}

beforeEach(() => {
  vi.restoreAllMocks();
});

describe("LanguageSettings", () => {
  it("加载语言列表并保存覆盖色", async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.includes("/api/languages")) {
        return jsonRes({
          languages: ["Go", "C++"],
          defaults: { Go: "#112233", "C++": "#445566" },
          colors: {},
        });
      }
      if (url.includes("/api/admin/language-colors")) {
        return jsonRes({ ok: true });
      }
      void init;
      return jsonRes({}, 404);
    });
    vi.stubGlobal("fetch", fetchMock);
    renderComp();

    await waitFor(() => expect(screen.getByText("Go")).toBeInTheDocument());
    expect(screen.getByText("C++")).toBeInTheDocument();
    // 未覆盖时占位符展示后端默认色
    expect(screen.getByLabelText("Go hex")).toHaveAttribute("placeholder", "#112233");

    const hex = screen.getByLabelText("Go hex");
    await userEvent.clear(hex);
    await userEvent.type(hex, "#abcdef");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      const call = fetchMock.mock.calls.find((c) =>
        String(c[0]).includes("/api/admin/language-colors"),
      );
      expect(call).toBeTruthy();
      const body = JSON.parse(String((call?.[1] as RequestInit | undefined)?.body));
      expect(body.colors.Go).toBe("#abcdef");
    });
  });

  it("过滤语言并按默认色占位", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) =>
        String(input).includes("/api/languages")
          ? jsonRes({ languages: ["Go", "Python"], defaults: {}, colors: {} })
          : jsonRes({}, 404),
      ),
    );
    renderComp();

    await waitFor(() => expect(screen.getByText("Go")).toBeInTheDocument());
    await userEvent.type(screen.getByPlaceholderText("Filter languages"), "py");
    await waitFor(() => expect(screen.queryByText("Go")).not.toBeInTheDocument());
    expect(screen.getByText("Python")).toBeInTheDocument();
  });
});
