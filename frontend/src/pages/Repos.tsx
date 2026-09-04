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
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

const NAME_RE = /^[A-Za-z0-9][A-Za-z0-9._-]*$/;

export default function Repos() {
  const { t, lang, to } = useI18n();
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
      toast.error(t("repos.nameInvalid"));
      return;
    }
    setBusy(true);
    try {
      await api.createRepo(name.trim(), desc.trim());
      toast.success(t("repos.created", { name: name.trim() }));
      setOpen(false);
      setName("");
      setDesc("");
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (repo: Repo) => {
    if (!window.confirm(t("repos.confirmDelete", { name: repo.name }))) return;
    try {
      await api.deleteRepo(repo.owner, repo.name);
      toast.success(t("repos.deleted", { name: repo.name }));
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const copy = (text: string) => {
    navigator.clipboard.writeText(text).then(() => toast.success(t("common.copied")));
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold">{t("repos.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("repos.subtitle")}</p>
        </div>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button className="gap-2 sm:self-start">
              <Plus className="h-4 w-4" />
              {t("repos.new")}
            </Button>
          </DialogTrigger>
          <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-md">
            <DialogHeader>
              <DialogTitle>{t("repos.new")}</DialogTitle>
              <DialogDescription>{t("repos.newDialogDescription")}</DialogDescription>
            </DialogHeader>
            <div className="grid gap-4">
              <div className="grid gap-2">
                <Label htmlFor="repo-name">{t("common.name")}</Label>
                <Input
                  id="repo-name"
                  placeholder="my-repo"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="repo-desc">{t("common.description")}</Label>
                <Input
                  id="repo-desc"
                  placeholder={t("common.optional")}
                  value={desc}
                  onChange={(e) => setDesc(e.target.value)}
                />
              </div>
            </div>
            <DialogFooter>
              <Button onClick={create} disabled={busy}>
                {t("common.create")}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      {error && (
        <Card className="border-destructive">
          <CardContent className="pt-6 text-sm text-destructive">
            {t("repos.loadFailed", { error })}
          </CardContent>
        </Card>
      )}

      {!error && repos.length === 0 && (
        <Card>
          <CardContent className="flex flex-col items-center gap-2 py-12 text-center">
            <FolderGit2 className="h-10 w-10 text-muted-foreground" />
            <p className="font-medium">{t("repos.empty")}</p>
            <p className="text-sm text-muted-foreground">{t("repos.emptyHint")}</p>
          </CardContent>
        </Card>
      )}

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {repos.map((repo) => (
          <Card key={repo.id} className="flex flex-col">
            <CardHeader className="pb-3">
              <div className="flex items-start justify-between gap-2">
                <CardTitle className="min-w-0 text-lg">
                  <Link
                    to={`/repo/${repo.owner}/${repo.name}`}
                    className="block truncate hover:underline"
                  >
                    {repo.name}
                  </Link>
                </CardTitle>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="ghost" size="icon" className="h-8 w-8 shrink-0">
                      <MoreVertical className="h-4 w-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem
                      className="text-destructive focus:text-destructive"
                      onClick={() => remove(repo)}
                    >
                      <Trash2 />
                      {t("repos.delete")}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
              <CardDescription className="line-clamp-2 min-h-10">
                {repo.description || t("common.noDescription")}
              </CardDescription>
            </CardHeader>
            <CardContent className="mt-auto space-y-3">
              <Badge variant="secondary" className="font-normal">
                {formatDate(repo.created_at, lang === "zh-CN" ? "zh-CN" : "en-US")}
              </Badge>
              <div className="flex items-center gap-2 rounded-md border bg-muted/40 px-2 py-1.5">
                <code className="flex-1 truncate text-xs text-muted-foreground">
                  {cloneCommand(repo.owner, repo.name)}
                </code>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-6 w-6 shrink-0"
                  onClick={() => copy(cloneCommand(repo.owner, repo.name))}
                >
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
