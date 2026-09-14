import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MermaidDiagram } from "@/components/mermaid";

const { renderMock } = vi.hoisted(() => ({ renderMock: vi.fn() }));

vi.mock("mermaid", () => ({
  default: {
    initialize: vi.fn(),
    render: renderMock,
  },
}));

describe("MermaidDiagram", () => {
  beforeEach(() => {
    renderMock.mockReset();
    renderMock.mockImplementation(async (_id: string) => ({
      svg: `<svg></svg>`,
    }));
  });

  // 回归：mermaid 按需异步加载，加载/渲染完成前不能把源码当文本展示（issue #7）
  it("渲染期间不展示源码，完成后注入 SVG", async () => {
    const chart = 'flowchart TD\n  nstart(["start"]) --> nstep_0["build"]';
    const { container } = render(<MermaidDiagram chart={chart} />);

    // 初始（mermaid 尚未 resolve）：源码不得出现在 DOM 中
    expect(container.textContent).not.toContain("flowchart TD");
    expect(container.querySelector("svg")).toBeNull();

    await waitFor(() => expect(container.querySelector("svg")).toBeTruthy());
    expect(renderMock).toHaveBeenCalledWith(expect.any(String), chart);
    expect(container.textContent).not.toContain("flowchart TD");
  });

  it("渲染失败时回退展示源码", async () => {
    renderMock.mockRejectedValueOnce(new Error("parse error"));
    const chart = "this is not a diagram";
    render(<MermaidDiagram chart={chart} />);

    await waitFor(() => expect(screen.getByText(chart)).toBeTruthy());
  });
});
