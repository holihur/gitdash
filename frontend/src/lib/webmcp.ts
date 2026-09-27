// WebMCP 支持：把 gitdash 的常用能力以工具形式注册到浏览器的
// navigator.modelContext（WebMCP / Web Model Context Protocol），
// 供内置 AI agent 的浏览器（如启用 WebMCP 的 Chrome/Edge 或扩展 polyfill）直接调用。
//
// 规范/实现仍在演进：这里同时兼容 provideContext({tools}) 与逐个 registerTool(tool)。
// 未检测到该 API 时为 no-op，不影响普通浏览器。

import { api } from "@/lib/api";

interface ModelContextTool {
  name: string;
  description: string;
  inputSchema: Record<string, unknown>;
  execute: (input: Record<string, unknown>) => Promise<ToolResult> | ToolResult;
}

interface ModelContext {
  provideContext?: (ctx: { tools: ModelContextTool[] }) => void;
  registerTool?: (tool: ModelContextTool) => void;
  unregisterTool?: (name: string) => void;
  clearContext?: () => void;
}

interface ToolResult {
  content: { type: "text"; text: string }[];
  isError?: boolean;
}

function ok(value: unknown): ToolResult {
  const text = typeof value === "string" ? value : JSON.stringify(value, null, 2);
  return { content: [{ type: "text", text }] };
}

function fail(err: unknown): ToolResult {
  const msg = err instanceof Error ? err.message : String(err);
  return { content: [{ type: "text", text: msg }], isError: true };
}

const str = (args: Record<string, unknown>, key: string): string => {
  const v = args[key];
  return typeof v === "string" ? v.trim() : "";
};

const num = (args: Record<string, unknown>, key: string, def = 0): number => {
  const v = args[key];
  if (typeof v === "number" && Number.isFinite(v)) return v;
  if (typeof v === "string" && v.trim() !== "") {
    const n = Number(v);
    return Number.isFinite(n) ? n : def;
  }
  return def;
};

const stringProp = (description: string) => ({ type: "string", description });
const objectSchema = (props: Record<string, unknown>, required: string[] = []) => ({
  type: "object",
  properties: props,
  ...(required.length ? { required } : {}),
});
const ownerRepo = (extra: Record<string, unknown> = {}, required: string[] = []) =>
  objectSchema(
    { owner: stringProp("repository owner"), repo: stringProp("repository name"), ...extra },
    ["owner", "repo", ...required],
  );

function buildTools(): ModelContextTool[] {
  return [
    {
      name: "list_repos",
      description: "List repositories accessible to the signed-in user.",
      inputSchema: objectSchema({}),
      execute: async () => {
        try {
          const page = await api.listRepos(200, 0);
          return ok(
            page.items.map((r) => ({
              owner: r.owner,
              name: r.name,
              description: r.description,
              private: r.private,
              default_branch: r.default_branch,
              updated_at: r.updated_at,
              commit_count: r.commit_count,
            })),
          );
        } catch (e) {
          return fail(e);
        }
      },
    },
    {
      name: "get_repo",
      description: "Get details of a repository.",
      inputSchema: ownerRepo(),
      execute: async (args) => {
        try {
          const r = await api.getRepo(str(args, "owner"), str(args, "repo"));
          return ok(r);
        } catch (e) {
          return fail(e);
        }
      },
    },
    {
      name: "list_issues",
      description: "List issues in a repository, optionally filtered by state.",
      inputSchema: ownerRepo({
        state: { type: "string", enum: ["open", "closed", "all"], description: "default open" },
      }),
      execute: async (args) => {
        try {
          const state = str(args, "state") || "open";
          const page = await api.listIssues(str(args, "owner"), str(args, "repo"), 50, 0, {
            state: state === "all" ? undefined : state,
          });
          return ok(page.items);
        } catch (e) {
          return fail(e);
        }
      },
    },
    {
      name: "get_issue",
      description: "Get a single issue with its comments.",
      inputSchema: ownerRepo({ number: { type: "integer", description: "issue number" } }, ["number"]),
      execute: async (args) => {
        try {
          const owner = str(args, "owner");
          const repo = str(args, "repo");
          const number = num(args, "number");
          const issue = await api.getIssue(owner, repo, number);
          const comments = await api.listComments(owner, repo, number, "issues");
          return ok({ issue, comments });
        } catch (e) {
          return fail(e);
        }
      },
    },
    {
      name: "create_issue",
      description: "Create an issue in a repository.",
      inputSchema: ownerRepo(
        {
          title: stringProp("issue title"),
          body: stringProp("issue body (Markdown)"),
        },
        ["title"],
      ),
      execute: async (args) => {
        try {
          const issue = await api.createIssue(
            str(args, "owner"),
            str(args, "repo"),
            str(args, "title"),
            str(args, "body"),
          );
          return ok({ number: issue.number, title: issue.title, state: issue.state });
        } catch (e) {
          return fail(e);
        }
      },
    },
    {
      name: "list_pulls",
      description: "List pull requests in a repository.",
      inputSchema: ownerRepo({
        state: { type: "string", enum: ["open", "merged", "closed", "all"], description: "default open" },
      }),
      execute: async (args) => {
        try {
          const raw = str(args, "state") || "open";
          const state = raw === "all" ? undefined : (raw as "open" | "merged" | "closed");
          const page = await api.listPulls(str(args, "owner"), str(args, "repo"), state, 50, 0);
          return ok(page.items);
        } catch (e) {
          return fail(e);
        }
      },
    },
    {
      name: "create_comment",
      description: "Add a comment to an issue or pull request.",
      inputSchema: ownerRepo(
        {
          number: { type: "integer", description: "issue or pull request number" },
          body: stringProp("comment body (Markdown)"),
          kind: { type: "string", enum: ["issue", "pull"], description: "default issue" },
        },
        ["number", "body"],
      ),
      execute: async (args) => {
        try {
          const kind = str(args, "kind") === "pull" ? "pulls" : "issues";
          const c = await api.postComment(
            str(args, "owner"),
            str(args, "repo"),
            num(args, "number"),
            str(args, "body"),
            kind,
          );
          return ok({ id: c.id, body: c.body });
        } catch (e) {
          return fail(e);
        }
      },
    },
    {
      name: "get_file",
      description: "Read a file from a repository (default branch when ref is omitted).",
      inputSchema: ownerRepo(
        {
          path: stringProp("file path relative to the repository root"),
          ref: stringProp("branch, tag or commit (default: repository default branch)"),
        },
        ["path"],
      ),
      execute: async (args) => {
        try {
          const owner = str(args, "owner");
          const repo = str(args, "repo");
          let ref = str(args, "ref");
          if (!ref) {
            const info = await api.getRepo(owner, repo);
            ref = info.default_branch || "main";
          }
          const blob = await api.blob(owner, repo, ref, str(args, "path"));
          if (blob.encoding !== "utf-8") {
            return ok({ path: blob.path, size: blob.size, encoding: blob.encoding });
          }
          return ok({ path: blob.path, content: blob.content });
        } catch (e) {
          return fail(e);
        }
      },
    },
    {
      name: "search_code",
      description: "Search code within a repository.",
      inputSchema: ownerRepo(
        {
          query: stringProp("search query"),
          ref: stringProp("branch, tag or commit (default branch when omitted)"),
        },
        ["query"],
      ),
      execute: async (args) => {
        try {
          const hits = await api.searchRepo(
            str(args, "owner"),
            str(args, "repo"),
            str(args, "query"),
            str(args, "ref") || undefined,
          );
          return ok(hits);
        } catch (e) {
          return fail(e);
        }
      },
    },
  ];
}

let registered = false;

/** 注册 WebMCP 工具；无 navigator.modelContext 时为 no-op，且只注册一次。 */
export function registerWebMCP(): void {
  if (registered) return;
  const mc = (navigator as unknown as { modelContext?: ModelContext }).modelContext;
  if (!mc) return;
  const tools = buildTools();
  if (typeof mc.provideContext === "function") {
    mc.provideContext({ tools });
  } else if (typeof mc.registerTool === "function") {
    for (const tool of tools) mc.registerTool(tool);
  } else {
    return;
  }
  registered = true;
  console.info(`[webmcp] registered ${tools.length} gitdash tools`);
}
