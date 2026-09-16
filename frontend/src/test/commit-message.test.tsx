import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { COMMIT_MESSAGE_MAX_LENGTH, CommitMessage } from "@/components/commit-message";

describe("CommitMessage", () => {
  it("renders short messages in full without a toggle", () => {
    render(<CommitMessage message="short message" />);
    expect(screen.getByText("short message")).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("truncates long messages and expands on click", async () => {
    const user = userEvent.setup();
    const long = "a".repeat(COMMIT_MESSAGE_MAX_LENGTH + 20);
    render(<CommitMessage message={long} />);

    const button = screen.getByRole("button");
    expect(button).toHaveTextContent("…");
    expect(button).not.toHaveTextContent(long);
    expect(button).toHaveAttribute("aria-expanded", "false");

    await user.click(button);
    expect(button).toHaveTextContent(long);
    expect(button).toHaveAttribute("aria-expanded", "true");

    await user.click(button);
    expect(button).not.toHaveTextContent(long);
  });

  it("exposes the full message on hover via title", () => {
    const long = "b".repeat(COMMIT_MESSAGE_MAX_LENGTH + 1);
    render(<CommitMessage message={long} />);
    expect(screen.getByRole("button")).toHaveAttribute("title", long);
  });

  it("renders nothing without a message", () => {
    const { container } = render(<CommitMessage />);
    expect(container).toBeEmptyDOMElement();
  });
});
