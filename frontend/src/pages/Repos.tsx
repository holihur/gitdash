import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import { Copy, FolderGit2, MoreVertical, Plus, Trash2 } from "lucide-react";
import { api, cloneCommand, type Repo } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { formatDate } from "@/lib/utils";

const NAME_RE = /^[A-Za-z0-9][A-Za-z0-9._-]*$/;

export default function Repos() {
  const [repos, setRepos] = useState<Repo[]>([]);
  const [error, setError] = useState("");
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [desc, setDesc] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setRepos(await api.listRepos());
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const create = async () => {
    if (!NAME_RE.test(name.trim())) {
      toast.error("仓库名只能包含字母、数字、._- 且以字母数字开头");
      return;
    }
    setBusy(true);
    try {
      await api.createRepo(name.trim(), desc.trim());
      toast.success(`仓库 ${name.trim()} 已创建`);
      setOpen(false);
      setName("");
      setDesc("");
      load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (repo: Repo) => {
    if (!window.confirm(`确定删除仓库 ${repo.name}？磁盘上的数据也会被删除，不可恢复。`)) return;
    try {
      await api.deleteRepo(repo.name);
      toast.success(`仓库 ${repo.name} 已删除`);
      load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    }
  };

  const copy = (text: string) => {
    navigator.clipboard.writeText(text).then(() => toast.success("已复制"));
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">仓库</h1>
          <p className="text-sm text-muted-foreground">通过 SSH 进行 clone / push，网页端浏览代码。</p>
        </div>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button className="gap-2">
              <Plus className="h-4 w-4" />
              新建仓库
            </Button>
          </DialogTrigger>
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle>新建仓库</DialogTitle>
              <DialogDescription>创建一个 bare 仓库，之后即可通过 SSH push。</DialogDescription>
            </DialogHeader>
            <div className="grid gap-4">
              <div className="grid gap-2">
                <Label htmlFor="repo-name">名称</Label>
                <Input
                  id="repo-name"
                  placeholder="my-repo"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="repo-desc">描述</Label>
                <Input
                  id="repo-desc"
                  placeholder="可选"
                  value={desc}
                  onChange={(e) => setDesc(e.target.value)}
                />
              </div>
            </div>
            <DialogFooter>
              <Button onClick={create} disabled={busy}>
                创建
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      {error && (
        <Card className="border-destructive">
          <CardContent className="pt-6 text-sm text-destructive">
            加载失败：{error}（可在右上角检查 API Token）
          </CardContent>
        </Card>
      )}

      {!error && repos.length === 0 && (
        <Card>
          <CardContent className="flex flex-col items-center gap-2 py-12 text-center">
            <FolderGit2 className="h-10 w-10 text-muted-foreground" />
            <p className="font-medium">还没有仓库</p>
            <p className="text-sm text-muted-foreground">点击右上角「新建仓库」创建第一个仓库。</p>
          </CardContent>
        </Card>
      )}

      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        {repos.map((repo) => (
          <Card key={repo.id} className="flex flex-col">
            <CardHeader className="pb-3">
              <div className="flex items-start justify-between">
                <CardTitle className="text-lg">
                  <Link to={`/repo/${repo.name}`} className="hover:underline">
                    {repo.name}
                  </Link>
                </CardTitle>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="ghost" size="icon" className="h-8 w-8">
                      <MoreVertical className="h-4 w-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem className="text-destructive focus:text-destructive" onClick={() => remove(repo)}>
                      <Trash2 />
                      删除仓库
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
              <CardDescription className="line-clamp-2 min-h-10">
                {repo.description || "无描述"}
              </CardDescription>
            </CardHeader>
            <CardContent className="mt-auto space-y-3">
              <Badge variant="secondary" className="font-normal">
                {formatDate(repo.created_at)}
              </Badge>
              <div className="flex items-center gap-2 rounded-md border bg-muted/40 px-2 py-1.5">
                <code className="flex-1 truncate text-xs text-muted-foreground">
                  {cloneCommand(repo.name)}
                </code>
                <Button variant="ghost" size="icon" className="h-6 w-6" onClick={() => copy(cloneCommand(repo.name))}>
                  <Copy className="h-3.5 w-3.5" />
                </Button>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}
