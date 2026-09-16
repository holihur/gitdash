import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { I18nProvider } from "@/lib/i18n";
import { DiffView, type DiffFileInfo } from "@/components/diff-view";

const files: DiffFileInfo[] = [
  { path: "src/a.ts", status: "M", insertions: 1, deletions: 1 },
  { path: "src/b.ts", status: "A", insertions: 1, deletions: 0 },
];

const patch = [
  "diff --git a/src/a.ts b/src/a.ts",
  "index 1111111..2222222 100644",
  "--- a/src/a.ts",
  "+++ b/src/a.ts",
  "@@ -1 +1 @@",
  "-old value",
  "+new value",
  "diff --git a/src/b.ts b/src/b.ts",
  "new file mode 100644",
  "--- /dev/null",
  "+++ b/src/b.ts",
  "@@ -0,0 +1 @@",
  "+added line",
].join("\n");

function renderDiff() {
  return render(
    <I18nProvider>
      <DiffView files={files} patch={patch} />
    </I18nProvider>,
  );
}

describe("DiffView", () => {
  it("lists changed files and shows the first file diff by default", () => {
    renderDiff();
    const aside = screen.getByRole("complementary");

    expect(within(aside).getByText("src/a.ts")).toBeInTheDocument();
    expect(within(aside).getByText("src/b.ts")).toBeInTheDocument();

    expect(screen.getByText(/new value/)).toBeInTheDocument();
    expect(screen.queryByText(/added line/)).not.toBeInTheDocument();
  });

  it("switches the diff when another file is selected", async () => {
    const user = userEvent.setup();
    renderDiff();
    const aside = screen.getByRole("complementary");

    await user.click(within(aside).getByRole("button", { name: /src\/b\.ts/ }));

    expect(screen.getByText(/added line/)).toBeInTheDocument();
    expect(screen.queryByText(/new value/)).not.toBeInTheDocument();
  });

  it("filters the file list", async () => {
    const user = userEvent.setup();
    renderDiff();
    const aside = screen.getByRole("complementary");

    await user.type(screen.getByLabelText("Filter files…"), "b.ts");

    expect(within(aside).queryByText("src/a.ts")).not.toBeInTheDocument();
    expect(within(aside).getByText("src/b.ts")).toBeInTheDocument();
  });

  it("searches the diff and highlights matches", async () => {
    const user = userEvent.setup();
    renderDiff();

    await user.type(screen.getByLabelText("Search in diff…"), "value");

    // "value" appears in both the removed and the added line.
    expect(screen.getByText("1/2")).toBeInTheDocument();
    expect(document.querySelectorAll("mark").length).toBeGreaterThan(0);
  });

  it("navigates to the next match and switches file", async () => {
    const user = userEvent.setup();
    renderDiff();

    await user.type(screen.getByLabelText("Search in diff…"), "added");
    // Only src/b.ts contains "added", so navigating selects it.
    await user.click(screen.getByLabelText("Next match"));

    expect(screen.getByTitle("src/b.ts")).toBeInTheDocument();
    expect(screen.getByText("added")).toBeInTheDocument();
  });
});
