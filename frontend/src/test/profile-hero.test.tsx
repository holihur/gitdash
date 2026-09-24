import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { I18nProvider } from "@/lib/i18n";
import { ProfileHero } from "@/components/profile-hero";

function renderHero() {
  return render(
    <I18nProvider>
      <ProfileHero
        coverUrl="/cover.png"
        avatar={<span data-testid="avatar">A</span>}
        title="ACME Inc"
        subtitle="acme"
        badge={<span>owner</span>}
        bio="we build things"
        meta={<p>meta line</p>}
        actions={<button type="button">Follow</button>}
      />
    </I18nProvider>,
  );
}

describe("ProfileHero", () => {
  it("渲染封面、悬浮头像与信息 / 操作", () => {
    const { container } = renderHero();
    expect(container.querySelector("img")).toHaveAttribute("src", "/cover.png");
    // 头像容器绝对定位在封面下沿，形成叠加而不是割裂的顶栏
    const avatarBox = screen.getByTestId("avatar").parentElement;
    expect(avatarBox?.className).toContain("absolute");
    expect(avatarBox?.className).toContain("-bottom-10");
    expect(screen.getByRole("heading", { level: 1, name: "ACME Inc" })).toBeInTheDocument();
    expect(screen.getByText("acme")).toBeInTheDocument();
    expect(screen.getByText("owner")).toBeInTheDocument();
    expect(screen.getByText("we build things")).toBeInTheDocument();
    expect(screen.getByText("meta line")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Follow" })).toBeInTheDocument();
  });

  it("无封面时仍渲染占位与头像", () => {
    render(
      <I18nProvider>
        <ProfileHero avatar={<span data-testid="avatar">A</span>} title="Solo" />
      </I18nProvider>,
    );
    expect(screen.getByTestId("avatar")).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 1, name: "Solo" })).toBeInTheDocument();
  });
});
