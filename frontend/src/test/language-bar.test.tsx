import { render, screen } from "@testing-library/react";
import { I18nProvider } from "@/lib/i18n";
import { LanguageBadge, LanguageBar, formatPercent } from "@/components/language-bar";
import { languageColor } from "@/lib/language-color";

describe("formatPercent", () => {
  it("保留一位小数", () => {
    expect(formatPercent(60)).toBe("60.0");
  });
  it("极小值显示 <0.1", () => {
    expect(formatPercent(0.05)).toBe("<0.1");
  });
  it("零显示 0.0", () => {
    expect(formatPercent(0)).toBe("0.0");
  });
});

describe("LanguageBar", () => {
  it("渲染语言与占比，并为 top 之外补 Other", () => {
    render(
      <I18nProvider>
        <LanguageBar
          languages={[
            { language: "Go", bytes: 60, percent: 60 },
            { language: "C++", bytes: 30, percent: 30 },
          ]}
        />
      </I18nProvider>,
    );
    expect(screen.getByText("Go")).toBeInTheDocument();
    expect(screen.getByText("60.0%")).toBeInTheDocument();
    expect(screen.getByText("C++")).toBeInTheDocument();
    // 100 - 90 = 10% -> Other
    expect(screen.getByText("Other")).toBeInTheDocument();
  });

  it("空列表不渲染", () => {
    const { container } = render(
      <I18nProvider>
        <LanguageBar languages={[]} />
      </I18nProvider>,
    );
    expect(container).toBeEmptyDOMElement();
  });
});

describe("LanguageBadge", () => {
  it("无语言时不渲染", () => {
    const { container } = render(<LanguageBadge />);
    expect(container).toBeEmptyDOMElement();
  });

  it("展示语言名", () => {
    render(<LanguageBadge language="Go" />);
    expect(screen.getByText("Go")).toBeInTheDocument();
  });
});

describe("languageColor", () => {
  it("已知语言返回固定色，未知语言稳定回退", () => {
    expect(languageColor("Go")).toBe("#00ADD8");
    expect(languageColor("UnknownLang")).toBe(languageColor("UnknownLang"));
  });
});
