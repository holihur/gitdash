import { useCallback, useEffect, useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { toast } from "sonner";
import {
  ChevronDown,
  ChevronRight,
  Copy,
  FileText,
  Folder,
  GitBranch,
  GitCommitHorizontal,
  MoreVertical,
} from "lucide-react";
import { api, cloneCommand, type Blob, type Branch, type Commit, type Repo, type TreeEntry } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { cn, formatDate, formatSize } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { MarkdownView, CodeText } from "@/components/markdown";
import RepoIssues from "@/pages/RepoIssues";

// 判断文件名是否需要 Markdown 渲染（README 任意扩展名或 .md/.markdown）
function isMarkdown(path: string): boolean {
  const base = path.split("/").pop() ?? "";
  const lower = base.toLowerCase();
  return /^readme(\.(md|markdown|txt))?$/.test(lower) || /\.(md|markdown)$/.test(lower);
}

// 根据扩展名推断 highlight.js 语言
function langFromPath(path: string): string | undefined {
  const ext = (path.split(".").pop() ?? "").toLowerCase();
  const map: Record<string, string> = {
    ts: "typescript",
    tsx: "typescript",
    js: "javascript",
    jsx: "javascript",
    mjs: "javascript",
    cjs: "javascript",
    py: "python",
    rb: "ruby",
    go: "go",
    rs: "rust",
    java: "java",
    kt: "kotlin",
    c: "c",
    h: "c",
    cpp: "cpp",
    hpp: "cpp",
    cs: "csharp",
    sh: "bash",
    bash: "bash",
    zsh: "bash",
    fish: "bash",
    yml: "yaml",
    yaml: "yaml",
    json: "json",
    toml: "ini",
    ini: "ini",
    sql: "sql",
    html: "xml",
    htm: "xml",
    xml: "xml",
    css: "css",
    scss: "scss",
    less: "less",
    dockerfile: "dockerfile",
    vue: "xml",
    php: "php",
    swift: "swift",
    scala: "scala",
    lua: "lua",
    r: "r",
    dart: "dart",
    elm: "elm",
    ex: "elixir",
    exs: "elixir",
    erl: "erlang",
    hs: "haskell",
    ml: "ocaml",
    pas: "pascal",
    pl: "perl",
    ps1: "powershell",
    proto: "protobuf",
    tex: "latex",
  };
  const name = (path.split("/").pop() ?? "").toLowerCase();
  if (name === "dockerfile") return "dockerfile";
  if (name.startsWith("makefile")) return "makefile";
  return map[ext];
}

function CodeBlock({ text, onCopy }: { text: string; onCopy: () => void }) {
  return (
    <div className="flex items-center gap-2 rounded-md border bg-muted/50 px-3 py-2">
      <code className="flex-1 overflow-x-auto whitespace-pre text-xs">{text}</code>
      <Button variant="ghost" size="icon" className="h-6 w-6 shrink-0" onClick={onCopy}>
        <Copy className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

export default function RepoView() {
  const { t, lang, to } = useI18n();
  const locale = lang === "zh-CN" ? "zh-CN" : "en-US";
  const { owner = "", name = "" } = useParams();
  const [repo, setRepo] = useState<Repo | null>(null);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [ref, setRef] = useState("");
  const [path, setPath] = useState<string[]>([]);
  const [entries, setEntries] = useState<TreeEntry[]>([]);
  const [blob, setBlob] = useState<Blob | null>(null);
  const [commits, setCommits] = useState<Commit[]>([]);
  const [tab, setTab] = useState("code");
  const [error, setError] = useState("");
  const [missing, setMissing] = useState(false);

  const copy = useCallback(
    (text: string) => {
      navigator.clipboard.writeText(text).then(() => toast.success(t("common.copied")));
    },
    [t],
  );

  useEffect(() => {
    (async () => {
      try {
        const [r, bs] = await Promise.all([api.getRepo(owner, name), api.branches(owner, name)]);
        setRepo(r);
        setBranches(bs);
        const head = bs.find((b) => b.is_head) ?? bs[0];
        if (head) setRef(head.name);
      } catch (e) {
        setMissing(true);
        setError(e instanceof Error ? e.message : String(e));
      }
    })();
  }, [name]);

  const loadTree = useCallback(async () => {
    if (!ref) return;
    try {
      const dir = path.join("/");
      const data = await api.tree(owner, name, ref, dir);
      setEntries(data.entries);
      setBlob(null);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [name, ref, path]);

  useEffect(() => {
    loadTree();
  }, [loadTree]);

  useEffect(() => {
    if (tab !== "commits" || !ref) return;
    api
      .commits(owner, name, ref)
      .then(setCommits)
      .catch((e) => toast.error(apiErrorMsg(to, e)));
  }, [tab, name, ref]);

  const openEntry = async (entry: TreeEntry) => {
    if (entry.type === "tree") {
      setPath([...path, entry.name]);
      return;
    }
    try {
      setBlob(await api.blob(owner, name, ref, [...path, entry.name].join("/")));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const jumpTo = (depth: number) => {
    // depth = -1 -> root, otherwise index of segment
    setPath(depth < 0 ? [] : path.slice(0, depth + 1));
    setBlob(null);
  };

  const emptyRepo = branches.length === 0;
  const commands = useMemo(
    () => [
      cloneCommand(owner, name),
      `cd ${name}`,
      `echo "# ${name}" >> README.md`,
      "git add README.md",
      'git commit -m "initial commit"',
      `git push origin ${branches[0]?.name || "main"}`,
    ],
    [name, branches],
  );

  if (missing) {
    return (
      <Card className="border-destructive">
        <CardContent className="pt-6 text-sm text-destructive">
          {t("repo.notFound", { error })}
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
        <div className="min-w-0">
          <h1 className="truncate text-2xl font-bold">
            {owner}/{name}
          </h1>
          <p className="text-sm text-muted-foreground">{repo?.description || t("common.noDescription")}</p>
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" className="w-full gap-2 font-mono text-xs sm:w-auto">
              <GitBranch className="h-3.5 w-3.5" />
              SSH
              <MoreVertical className="ml-auto h-3.5 w-3.5 sm:ml-0" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-80 max-w-[calc(100vw-2rem)]">
            <DropdownMenuLabel className="break-all font-mono text-xs normal-case">
              {cloneCommand(owner, name)}
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => copy(cloneCommand(owner, name))}>
              <Copy />
              {t("repo.copyCloneCommand")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <Tabs value={tab} onValueChange={setTab}>
        <TabsList className="w-full sm:w-auto">
          <TabsTrigger value="code" className="flex-1 sm:flex-none">
            {t("repo.code")}
          </TabsTrigger>
          <TabsTrigger value="commits" className="flex-1 sm:flex-none">
            {t("repo.commits")}
          </TabsTrigger>
          <TabsTrigger value="issues" className="flex-1 sm:flex-none">
            {t("issues.title")}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="code" className="space-y-4">
          {/* branch selector + breadcrumb */}
          <div className="flex flex-wrap items-center gap-2">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="outline" size="sm" className="max-w-full gap-2" disabled={emptyRepo}>
                  <GitBranch className="h-4 w-4 shrink-0" />
                  <span className="truncate">{ref || t("repo.noBranch")}</span>
                  <ChevronDown className="h-3.5 w-3.5 shrink-0" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" className="max-h-72 w-64 overflow-auto">
                {branches.map((b) => (
                  <DropdownMenuItem
                    key={b.name}
                    onClick={() => {
                      setRef(b.name);
                      setPath([]);
                      setBlob(null);
                    }}
                  >
                    <GitBranch className="shrink-0" />
                    <span className="truncate">{b.name}</span>
                    {b.is_head && (
                      <Badge variant="secondary" className="ml-auto shrink-0">
                        HEAD
                      </Badge>
                    )}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>

            {!emptyRepo && (
              <nav className="flex min-w-0 flex-wrap items-center gap-1 text-sm">
                <button
                  className={cn(
                    "font-medium hover:underline",
                    path.length === 0 || blob ? "" : "text-muted-foreground",
                  )}
                  onClick={() => jumpTo(-1)}
                >
                  {name}
                </button>
                {path.map((seg, i) => (
                  <span key={i} className="flex items-center gap-1">
                    <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
                    <button
                      className={cn(
                        "hover:underline",
                        i === path.length - 1 && !blob ? "font-medium" : "text-muted-foreground",
                      )}
                      onClick={() => jumpTo(i)}
                    >
                      {seg}
                    </button>
                  </span>
                ))}
                {blob && (
                  <span className="flex items-center gap-1">
                    <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
                    <span className="font-medium">{blob.path.split("/").pop()}</span>
                  </span>
                )}
              </nav>
            )}
          </div>

          {error && (
            <Card className="border-destructive">
              <CardContent className="pt-6 text-sm text-destructive">{error}</CardContent>
            </Card>
          )}

          {emptyRepo && !error && (
            <Card>
              <CardHeader>
                <CardTitle className="text-lg">{t("repo.emptyRepo")}</CardTitle>
                <CardDescription>{t("repo.emptyRepoHint")}</CardDescription>
              </CardHeader>
              <CardContent className="space-y-2">
                {commands.map((cmd) => (
                  <CodeBlock key={cmd} text={cmd} onCopy={() => copy(cmd)} />
                ))}
                <p className="pt-2 text-xs text-muted-foreground">{t("repo.sshHint")}</p>
              </CardContent>
            </Card>
          )}

          {!emptyRepo && !error && blob && (
            <Card>
              <CardHeader className="pb-2">
                <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                  <CardTitle className="break-all font-mono text-sm">{blob.path}</CardTitle>
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="secondary">{formatSize(blob.size)}</Badge>
                    {blob.encoding !== "utf-8" && (
                      <Badge variant="destructive">
                        {blob.encoding === "binary" ? t("repo.binaryFile") : t("repo.fileTooLarge")}
                      </Badge>
                    )}
                  </div>
                </div>
              </CardHeader>
              <CardContent>
                {blob.encoding === "utf-8" ? (
                  isMarkdown(blob.path) ? (
                    <div className="max-h-[70vh] overflow-auto rounded-md border bg-muted/30 p-4">
                      <MarkdownView text={blob.content} />
                    </div>
                  ) : (
                    <div className="max-h-[70vh] overflow-auto rounded-md border bg-muted/30 p-4">
                      <CodeText text={blob.content} lang={langFromPath(blob.path)} />
                    </div>
                  )
                ) : (
                  <p className="text-sm text-muted-foreground">{t("repo.previewNotAvailable")}</p>
                )}
              </CardContent>
            </Card>
          )}

          {!emptyRepo && !error && !blob && (
            <div className="overflow-x-auto rounded-lg border">
              <Table className="min-w-[560px]">
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("common.name")}</TableHead>
                    <TableHead className="w-24">{t("common.type")}</TableHead>
                    <TableHead className="w-28 text-right">{t("common.size")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {entries.map((entry) => (
                    <TableRow key={entry.name}>
                      <TableCell>
                        <button
                          className="flex items-center gap-2 hover:underline"
                          onClick={() => openEntry(entry)}
                        >
                          {entry.type === "tree" ? (
                            <Folder className="h-4 w-4 shrink-0 text-blue-500" />
                          ) : (
                            <FileText className="h-4 w-4 shrink-0 text-muted-foreground" />
                          )}
                          <span className={cn("truncate", entry.type === "tree" ? "font-medium" : "")}>
                            {entry.name}
                          </span>
                        </button>
                      </TableCell>
                      <TableCell>
                        <Badge variant="secondary">
                          {entry.type === "tree" ? t("common.directory") : t("common.file")}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-right text-sm text-muted-foreground">
                        {entry.type === "blob" ? formatSize(entry.size) : "-"}
                      </TableCell>
                    </TableRow>
                  ))}
                  {entries.length === 0 && (
                    <TableRow>
                      <TableCell
                        colSpan={3}
                        className="py-10 text-center text-sm text-muted-foreground"
                      >
                        {t("repo.emptyDir")}
                      </TableCell>
                    </TableRow>
                  )}
                </TableBody>
              </Table>
            </div>
          )}
        </TabsContent>

        <TabsContent value="commits">
          {emptyRepo ? (
            <p className="py-10 text-center text-sm text-muted-foreground">{t("repo.noCommits")}</p>
          ) : (
            <div className="overflow-x-auto rounded-lg border">
              <Table className="min-w-[640px]">
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("repo.commit")}</TableHead>
                    <TableHead className="w-36">{t("common.author")}</TableHead>
                    <TableHead className="w-56">{t("common.date")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {commits.map((c) => (
                    <TableRow key={c.sha}>
                      <TableCell>
                        <div className="flex items-center gap-3">
                          <GitCommitHorizontal className="h-4 w-4 shrink-0 text-muted-foreground" />
                          <code className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-xs">
                            {c.sha.slice(0, 7)}
                          </code>
                          <span className="truncate">{c.message}</span>
                        </div>
                      </TableCell>
                      <TableCell className="text-sm">{c.author}</TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        {formatDate(c.date, locale)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </TabsContent>

        <TabsContent value="issues">
          <RepoIssues owner={owner} name={name} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
