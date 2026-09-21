import { render, screen } from "@testing-library/react";
import { vi } from "vitest";
import { I18nProvider } from "@/lib/i18n";
import ErrorBoundary from "@/components/error-boundary";

function Boom(): never {
  throw new Error("boom");
}

describe("ErrorBoundary", () => {
  it("捕获渲染错误并显示可恢复的提示，而不是白屏", () => {
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    render(
      <I18nProvider>
        <ErrorBoundary>
          <Boom />
        </ErrorBoundary>
      </I18nProvider>,
    );
    expect(screen.getByText(/Something went wrong/i)).toBeInTheDocument();
    expect(screen.getByText("boom")).toBeInTheDocument();
    spy.mockRestore();
  });
});
