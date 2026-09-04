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
  const { name = "" } = useParams();
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

  const copy = useCallback((text: string) => {
    navigator.clipboard.writeText(text).then(() => toast.success("已复制"));
  }, []);

  useEffect(() => {
    (async () => {
      try {
        const [r, bs] = await Promise.all([api.getRepo(name), api.branches(name)]);
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
      const data = await api.tree(name, ref, dir);
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
      .commits(name, ref)
      .then(setCommits)
      .catch((e) => toast.error(e instanceof Error ? e.message : String(e)));
  }, [tab, name, ref]);

  const openEntry = async (entry: TreeEntry) => {
    if (entry.type === "tree") {
      setPath([...path, entry.name]);
      return;
    }
    try {
      setBlob(await api.blob(name, ref, [...path, entry.name].join("/")));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
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
      cloneCommand(name),
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
        <CardContent className="pt-6 text-sm text-destructive">仓库不存在：{error}</CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <h1 className="truncate text-2xl font-bold">{name}</h1>
          <p className="text-sm text-muted-foreground">{repo?.description || "无描述"}</p>
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" className="gap-2 font-mono text-xs">
              <GitBranch className="h-3.5 w-3.5" />
              SSH
              <MoreVertical className="h-3.5 w-3.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-80">
            <DropdownMenuLabel className="font-mono text-xs normal-case">
              {cloneCommand(name)}
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => copy(cloneCommand(name))}>
              <Copy />
              复制 clone 命令
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="code">Code</TabsTrigger>
          <TabsTrigger value="commits">Commits</TabsTrigger>
        </TabsList>

        <TabsContent value="code" className="space-y-4">
          {/* branch selector + breadcrumb */}
          <div className="flex flex-wrap items-center gap-2">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="outline" size="sm" className="gap-2" disabled={emptyRepo}>
                  <GitBranch className="h-4 w-4" />
                  {ref || "no branch"}
                  <ChevronDown className="h-3.5 w-3.5" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" className="max-h-72 overflow-auto">
                {branches.map((b) => (
                  <DropdownMenuItem key={b.name} onClick={() => { setRef(b.name); setPath([]); setBlob(null); }}>
                    <GitBranch />
                    {b.name}
                    {b.is_head && <Badge variant="secondary" className="ml-auto">HEAD</Badge>}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>

            {!emptyRepo && (
              <nav className="flex flex-wrap items-center gap-1 text-sm">
                <button
                  className={cn("font-medium hover:underline", path.length === 0 || blob ? "" : "text-muted-foreground")}
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
                <CardTitle className="text-lg">空仓库</CardTitle>
                <CardDescription>按下面的命令推送第一次提交：</CardDescription>
              </CardHeader>
              <CardContent className="space-y-2">
                {commands.map((cmd) => (
                  <CodeBlock key={cmd} text={cmd} onCopy={() => copy(cmd)} />
                ))}
                <p className="pt-2 text-xs text-muted-foreground">
                  提示：先在「SSH Keys」页面添加你的公钥，才能通过 SSH 访问。
                </p>
              </CardContent>
            </Card>
          )}

          {!emptyRepo && !error && blob && (
            <Card>
              <CardHeader className="pb-2">
                <div className="flex items-center justify-between">
                  <CardTitle className="font-mono text-sm">{blob.path}</CardTitle>
                  <div className="flex items-center gap-2">
                    <Badge variant="secondary">{formatSize(blob.size)}</Badge>
                    {blob.encoding !== "utf-8" && (
                      <Badge variant="destructive">
                        {blob.encoding === "binary" ? "二进制文件" : "文件过大，仅显示大小"}
                      </Badge>
                    )}
                  </div>
                </div>
              </CardHeader>
              <CardContent>
                {blob.encoding === "utf-8" ? (
                  <pre className="max-h-[70vh] overflow-auto rounded-md border bg-muted/30 p-4 text-xs leading-5">
                    {blob.content}
                  </pre>
                ) : (
                  <p className="text-sm text-muted-foreground">该文件无法在浏览器中预览。</p>
                )}
              </CardContent>
            </Card>
          )}

          {!emptyRepo && !error && !blob && (
            <div className="rounded-lg border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>名称</TableHead>
                    <TableHead className="w-24">类型</TableHead>
                    <TableHead className="w-28 text-right">大小</TableHead>
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
                            <Folder className="h-4 w-4 text-blue-500" />
                          ) : (
                            <FileText className="h-4 w-4 text-muted-foreground" />
                          )}
                          <span className={entry.type === "tree" ? "font-medium" : ""}>{entry.name}</span>
                        </button>
                      </TableCell>
                      <TableCell>
                        <Badge variant="secondary">{entry.type === "tree" ? "目录" : "文件"}</Badge>
                      </TableCell>
                      <TableCell className="text-right text-sm text-muted-foreground">
                        {entry.type === "blob" ? formatSize(entry.size) : "-"}
                      </TableCell>
                    </TableRow>
                  ))}
                  {entries.length === 0 && (
                    <TableRow>
                      <TableCell colSpan={3} className="py-10 text-center text-sm text-muted-foreground">
                        该目录为空
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
            <p className="py-10 text-center text-sm text-muted-foreground">暂无提交</p>
          ) : (
            <div className="rounded-lg border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>提交</TableHead>
                    <TableHead className="w-36">作者</TableHead>
                    <TableHead className="w-56">时间</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {commits.map((c) => (
                    <TableRow key={c.sha}>
                      <TableCell>
                        <div className="flex items-center gap-3">
                          <GitCommitHorizontal className="h-4 w-4 shrink-0 text-muted-foreground" />
                          <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{c.sha.slice(0, 7)}</code>
                          <span className="truncate">{c.message}</span>
                        </div>
                      </TableCell>
                      <TableCell className="text-sm">{c.author}</TableCell>
                      <TableCell className="text-sm text-muted-foreground">{formatDate(c.date)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </TabsContent>
      </Tabs>
    </div>
  );
}
