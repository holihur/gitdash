import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";

import { NavOverflow, type NavOverflowItem } from "@/components/nav-overflow";

const items: NavOverflowItem[] = [
  { key: "repos", to: "/", icon: <span>R</span>, label: "Repositories" },
  { key: "explore", to: "/explore", icon: <span>E</span>, label: "Explore" },
  { key: "inbox", to: "/inbox", icon: <span>I</span>, label: "Inbox", badge: <span>3</span> },
];

describe("NavOverflow", () => {
  it("无溢出时渲染全部导航项", () => {
    render(
      <MemoryRouter initialEntries={["/explore"]}>
        <NavOverflow items={items} />
      </MemoryRouter>,
    );

    expect(screen.getByText("Repositories")).toBeInTheDocument();
    expect(screen.getByText("Explore")).toBeInTheDocument();
    expect(screen.getByText("Inbox")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
  });
});
