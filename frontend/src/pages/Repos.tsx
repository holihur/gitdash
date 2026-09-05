import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import { Copy, Download, Eye, FolderGit2, MoreVertical, Plus, Star, Trash2, Users, Webhook } from "lucide-react";
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
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import ConfirmDialog from "@/components/confirm-dialog";
import { formatDate } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import CollaboratorsDialog from "@/components/collabs-dialog";
import WebhooksDialog from "@/components/webhooks-dialog";

const NAME_RE = /^[A-Za-z0-9][A-Za-z0-9._-]*$/;

export default function Repos() {
  const { t, lang, to } = useI18n();
  const [repos, setRepos] = useState<Repo[]>([]);
  const [starred, setStarred] = useState<Repo[]>([]);
  const [watched, setWatched] = useState<Repo[]>([]);
  const [tab, setTab] = useState<"repos" | "starred" | "watching">("repos");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [pendingDelete, setPendingDelete] = useState<Repo | null>(null);
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [desc, setDesc] = useState("");
  const [template, setTemplate] = useState<"" | "readme">("");
  const [busy, setBusy] = useState(false);
  const [collabRepo, setCollabRepo] = useState<Repo | null>(null);
  const [hookRepo, setHookRepo] = useState<Repo | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const [importUrl, setImportUrl] = useState("");
  const [importName, setImportName] = useState("");
  const [importPrivate, setImportPrivate] = useState(true);
  const [importKey, setImportKey] = useState("");
  const [importBusy, setImportBusy] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [mine, star, watch] = await Promise.all([
        api.listRepos(),
        api.listStarred(),
        api.listWatched(),
      ]);
      setRepos(mine);
      setStarred(star);
      setWatched(watch);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const show = tab === "repos" ? repos : tab === "starred" ? starred : watched;
  const isMine = tab === "repos";

  const create = async () => {
    if (!NAME_RE.test(name.trim())) {
      toast.error(t("repos.nameInvalid"));
      return;
    }
    setBusy(true);
    try {
      await api.createRepo(name.trim(), desc.trim(), template);
      toast.success(t("repos.created", { name: name.trim() }));
      setOpen(false);
      setName("");
      setDesc("");
      setTemplate("");
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const doImport = async () => {
    if (!importUrl.trim()) {
      toast.error(t("imports.urlRequired"));
      return;
    }
    setImportBusy(true);
    try {
      const repo = await api.importRepo({
        url: importUrl.trim(),
        name: importName.trim() || undefined,
        private: importPrivate,
        private_key: importKey.trim() || undefined,
      });
      toast.success(t("imports.imported", { name: repo.name }));
      setImportOpen(false);
      setImportUrl("");
      setImportName("");
      setImportPrivate(true);
      setImportKey("");
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setImportBusy(false);
    }
  };

  const remove = async (repo: Repo) => {
    setPendingDelete(null);
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
        <div className="flex gap-2 sm:self-start">
          <Button variant="outline" className="gap-2" onClick={() => setImportOpen(true)}>
            <Download className="h-4 w-4" />
            {t("imports.import")}
          </Button>
          <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger asChild>
              <Button className="gap-2">
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
              <div className="grid gap-2">
                <Label htmlFor="repo-template">{t("repos.templateLabel")}</Label>
                <select
                  id="repo-template"
                  value={template}
                  onChange={(e) => setTemplate(e.target.value as "" | "readme")}
                  className="h-10 rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <option value="">{t("repos.templateNone")}</option>
                  <option value="readme">{t("repos.templateReadme")}</option>
                </select>
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
      </div>

      <Tabs value={tab} onValueChange={(v) => setTab(v as "repos" | "starred" | "watching")}>
        <TabsList className="w-full sm:w-auto">
          <TabsTrigger value="repos">{t("social.myRepos")}</TabsTrigger>
          <TabsTrigger value="starred">{t("social.starredRepos")}</TabsTrigger>
          <TabsTrigger value="watching">{t("social.watchingRepos")}</TabsTrigger>
        </TabsList>

        {error && (
          <Card className="mt-4 border-destructive">
            <CardContent className="pt-6 text-sm text-destructive">
              {t("repos.loadFailed", { error })}
            </CardContent>
          </Card>
        )}

        {loading && !error && (
          <div className="mt-4 grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {Array.from({ length: 6 }).map((_, i) => (
              <Card key={i}>
                <CardHeader className="pb-3">
                  <Skeleton className="h-6 w-40" />
                  <Skeleton className="h-4 w-full" />
                  <Skeleton className="h-4 w-2/3" />
                </CardHeader>
                <CardContent className="space-y-3">
                  <div className="flex gap-2">
                    <Skeleton className="h-5 w-28" />
                    <Skeleton className="h-5 w-16" />
                    <Skeleton className="h-5 w-16" />
                  </div>
                  <Skeleton className="h-8 w-full" />
                </CardContent>
              </Card>
            ))}
          </div>
        )}

        {!loading && !error && show.length === 0 && (
          <Card className="mt-4">
            <CardContent className="flex flex-col items-center gap-2 py-12 text-center">
              <FolderGit2 className="h-10 w-10 text-muted-foreground" />
              <p className="font-medium">
                {isMine ? t("repos.empty") : tab === "starred" ? t("social.starredEmpty") : t("social.watchingEmpty")}
              </p>
              <p className="text-sm text-muted-foreground">
                {isMine
                  ? t("repos.emptyHint")
                  : tab === "starred"
                    ? t("social.starredEmptyHint")
                    : t("social.watchingEmptyHint")}
              </p>
            </CardContent>
          </Card>
        )}

        {show.length > 0 && !loading && (
          <div className="mt-4 grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {show.map((repo) => {
              const isOwner = isMine && (repo.role === undefined || repo.role === "owner");
              return (
                <Card key={`${repo.owner}/${repo.name}`} className="flex flex-col">
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
                      {isOwner && (
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button variant="ghost" size="icon" className="h-8 w-8 shrink-0">
                              <MoreVertical className="h-4 w-4" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            <DropdownMenuItem onClick={() => setCollabRepo(repo)}>
                              <Users />
                              {t("collabs.manage")}
                            </DropdownMenuItem>
                            <DropdownMenuItem onClick={() => setHookRepo(repo)}>
                              <Webhook />
                              {t("webhooks.manage")}
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              className="text-destructive focus:text-destructive"
                              onClick={() => setPendingDelete(repo)}
                            >
                              <Trash2 />
                              {t("repos.delete")}
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      )}
                    </div>
                    <CardDescription className="min-h-10">
                      <span className="flex flex-wrap items-center gap-x-1.5 gap-y-1">
                        {!isMine && (
                          <Badge variant="secondary" className="font-normal">
                            {repo.owner}
                          </Badge>
                        )}
                        {isMine && !isOwner && repo.role && (
                          <Badge variant="secondary" className="font-normal">
                            {t("collabs.sharedBy", { owner: repo.owner })} ·
                            {repo.role === "write" ? t("collabs.write") : t("collabs.read")}
                          </Badge>
                        )}
                      </span>
                      <span className="line-clamp-2 block">
                        {repo.description || t("common.noDescription")}
                      </span>
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="mt-auto space-y-3">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge variant="secondary" className="font-normal">
                        {formatDate(repo.created_at, lang === "zh-CN" ? "zh-CN" : "en-US")}
                      </Badge>
                      <Badge variant="secondary" className="gap-1 font-normal">
                        <Star className="h-3 w-3" />
                        {repo.stars ?? 0}
                      </Badge>
                      <Badge variant="secondary" className="gap-1 font-normal">
                        <Eye className="h-3 w-3" />
                        {repo.watchers ?? 0}
                      </Badge>
                    </div>
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
              );
            })}
          </div>
        )}
      </Tabs>

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(o) => !o && setPendingDelete(null)}
        description={t("repos.confirmDelete", { name: pendingDelete?.name ?? "" })}
        onConfirm={() => pendingDelete && remove(pendingDelete)}
      />
      <CollaboratorsDialog
        open={collabRepo !== null}
        onOpenChange={(o) => {
          if (!o) setCollabRepo(null);
        }}
        owner={collabRepo?.owner ?? ""}
        repo={collabRepo?.name ?? ""}
      />
      <WebhooksDialog
        open={hookRepo !== null}
        onOpenChange={(o) => {
          if (!o) setHookRepo(null);
        }}
        owner={hookRepo?.owner ?? ""}
        repo={hookRepo?.name ?? ""}
      />
      <Dialog open={importOpen} onOpenChange={setImportOpen}>
        <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("imports.importTitle")}</DialogTitle>
            <DialogDescription>{t("imports.importDescription")}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="import-url">{t("imports.urlLabel")}</Label>
              <Input
                id="import-url"
                placeholder="https://github.com/owner/repo.git"
                value={importUrl}
                onChange={(e) => setImportUrl(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="import-name">{t("imports.nameLabel")}</Label>
              <Input
                id="import-name"
                placeholder={t("common.optional")}
                value={importName}
                onChange={(e) => setImportName(e.target.value)}
              />
            </div>
            <div className="flex items-center gap-2">
              <input
                id="import-private"
                type="checkbox"
                checked={importPrivate}
                onChange={(e) => setImportPrivate(e.target.checked)}
                className="h-4 w-4"
              />
              <Label htmlFor="import-private" className="text-sm font-normal">
                {t("imports.privateLabel")}
              </Label>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="import-key">{t("imports.keyLabel")}</Label>
              <Textarea
                id="import-key"
                rows={4}
                placeholder={t("imports.keyPlaceholder")}
                value={importKey}
                onChange={(e) => setImportKey(e.target.value)}
                className="font-mono text-xs"
              />
              <p className="text-xs text-muted-foreground">{t("imports.keyHint")}</p>
            </div>
          </div>
          <DialogFooter>
            <Button onClick={doImport} disabled={importBusy || !importUrl.trim()}>
              {t("imports.import")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
