import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/api", () => ({
  api: {
    listRepos: vi.fn(),
    getRepo: vi.fn(),
    listIssues: vi.fn(),
    getIssue: vi.fn(),
    createIssue: vi.fn(),
    listPulls: vi.fn(),
    postComment: vi.fn(),
    listComments: vi.fn(),
    blob: vi.fn(),
    searchRepo: vi.fn(),
  },
}));

const { api } = (await import("@/lib/api")) as unknown as {
  api: Record<string, ReturnType<typeof vi.fn>>;
};

afterEach(() => {
  vi.clearAllMocks();
  delete (navigator as unknown as { modelContext?: unknown }).modelContext;
});

describe("webmcp", () => {
  it("registers tools via navigator.modelContext.provideContext", async () => {
    const provideContext = vi.fn();
    Object.defineProperty(navigator, "modelContext", {
      configurable: true,
      value: { provideContext },
    });
    vi.resetModules();
    const { registerWebMCP } = await import("@/lib/webmcp");
    registerWebMCP();

    expect(provideContext).toHaveBeenCalledTimes(1);
    const tools = provideContext.mock.calls[0][0].tools as { name: string }[];
    const names = tools.map((t) => t.name);
    expect(names).toContain("list_repos");
    expect(names).toContain("create_issue");
    expect(names).toContain("search_code");
  });

  it("falls back to registerTool when provideContext is absent", async () => {
    const registerTool = vi.fn();
    Object.defineProperty(navigator, "modelContext", {
      configurable: true,
      value: { registerTool },
    });
    vi.resetModules();
    const { registerWebMCP } = await import("@/lib/webmcp");
    registerWebMCP();
    expect(registerTool).toHaveBeenCalled();
  });

  it("create_issue tool calls the API", async () => {
    const provideContext = vi.fn();
    Object.defineProperty(navigator, "modelContext", {
      configurable: true,
      value: { provideContext },
    });
    api.createIssue.mockResolvedValue({ number: 7, title: "hello", state: "open" });
    vi.resetModules();
    const { registerWebMCP } = await import("@/lib/webmcp");
    registerWebMCP();
    const tools = provideContext.mock.calls[0][0].tools as {
      name: string;
      execute: (a: Record<string, unknown>) => Promise<{ isError?: boolean }>;
    }[];
    const tool = tools.find((t) => t.name === "create_issue")!;
    const result = await tool.execute({ owner: "alice", repo: "demo", title: "hello", body: "b" });
    expect(api.createIssue).toHaveBeenCalledWith("alice", "demo", "hello", "b");
    expect(result.isError).toBeFalsy();
  });

  it("is a no-op without navigator.modelContext", async () => {
    vi.resetModules();
    const { registerWebMCP } = await import("@/lib/webmcp");
    expect(() => registerWebMCP()).not.toThrow();
  });
});
