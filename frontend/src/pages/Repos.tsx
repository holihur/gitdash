import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Download, FolderGit2, Plus } from "lucide-react";
import { api, type Org, type Repo } from "@/lib/api";
import { useQueryState } from "@/lib/query-state";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import Pagination from "@/components/ui/pagination";
import ConfirmDialog from "@/components/confirm-dialog";
import { dateLocale, useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import CollaboratorsDialog from "@/components/collabs-dialog";
import WebhooksDialog from "@/components/webhooks-dialog";
import RepoCard from "./repos/RepoCard";
import ImportRepoDialog from "./repos/ImportRepoDialog";

const NAME_RE = /^[A-Za-z0-9][A-Za-z0-9._-]*$/;

export default function Repos() {
  const { t, lang, to } = useI18n();
  const [repos, setRepos] = useState<Repo[]>([]);
  const [repoTotal, setRepoTotal] = useState(0);
  // tab/页码/页大小同步进 URL(?tab/?page/?size)
  const { get, getNum, set } = useQueryState();
  const page = getNum("page", 1);
  const setPage = (p: number) => set({ page: p > 1 ? p : null }, { push: true });
  const pageSize = getNum("size", 20);
  const tabRaw = get("tab", "repos");
  const tab = (["repos", "starred", "watching"] as const).includes(tabRaw as "repos")
    ? (tabRaw as "repos" | "starred" | "watching")
    : "repos";
  const setTab = (v: "repos" | "starred" | "watching") =>
    set({ tab: v === "repos" ? null : v, page: null }, { push: true });
  const [starred, setStarred] = useState<Repo[]>([]);
  const [watched, setWatched] = useState<Repo[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [pendingDelete, setPendingDelete] = useState<Repo | null>(null);
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [desc, setDesc] = useState("");
  const [template, setTemplate] = useState<"" | "readme">("");
  const [templateRepos, setTemplateRepos] = useState<Repo[]>([]);
  const [templateRepo, setTemplateRepo] = useState("");
  const [namespace, setNamespace] = useState("");
  const [orgs, setOrgs] = useState<Org[]>([]);
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
        api.listRepos(pageSize, (page - 1) * pageSize),
        api.listStarred(),
        api.listWatched(),
      ]);
      setRepos(mine.items);
      setRepoTotal(mine.total);
      setStarred(star);
      setWatched(watch);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [page, pageSize]);

  useEffect(() => {
    load();
    api.listOrgs().then(setOrgs).catch(() => setOrgs([]));
    api.listTemplateRepos().then(setTemplateRepos).catch(() => setTemplateRepos([]));
  }, [load]);

  const show = tab === "repos" ? repos : tab === "starred" ? starred : watched;
  const isMine = tab === "repos";

  const onPageChange = (p: number) => setPage(p);
  const onPageSizeChange = (s: number) => {
    set({ size: s === 20 ? null : s, page: null });
  };

  const create = async () => {
    if (!NAME_RE.test(name.trim())) {
      toast.error(t("repos.nameInvalid"));
      return;
    }
    setBusy(true);
    try {
      const tplRepo = templateRepos.find((r) => `${r.owner}/${r.name}` === templateRepo);
      await api.createRepo(
        name.trim(),
        desc.trim(),
        template,
        undefined,
        namespace || undefined,
        tplRepo ? { owner: tplRepo.owner, name: tplRepo.name } : undefined,
      );
      toast.success(t("repos.created", { name: namespace ? `${namespace}/${name.trim()}` : name.trim() }));
      setOpen(false);
      setName("");
      setDesc("");
      setTemplate("");
      setTemplateRepo("");
      setNamespace("");
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
      toast.success(t("imports.queued", { name: repo.name }));
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
              {orgs.length > 0 && (
                <div className="grid gap-2">
                  <Label htmlFor="repo-namespace">{t("repos.namespaceLabel")}</Label>
                  <select
                    id="repo-namespace"
                    value={namespace}
                    onChange={(e) => setNamespace(e.target.value)}
                    className="h-10 w-full min-w-0 rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <option value="">{t("repos.namespacePersonal")}</option>
                    {orgs.map((o) => (
                      <option key={o.name} value={o.name}>
                        {o.name}
                      </option>
                    ))}
                  </select>
                </div>
              )}
              <div className="grid gap-2">
                <Label htmlFor="repo-template">{t("repos.templateLabel")}</Label>
                <select
                  id="repo-template"
                  value={template}
                  onChange={(e) => setTemplate(e.target.value as "" | "readme")}
                  className="h-10 w-full min-w-0 rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <option value="">{t("repos.templateNone")}</option>
                  <option value="readme">{t("repos.templateReadme")}</option>
                </select>
              </div>
              {templateRepos.length > 0 && (
                <div className="grid gap-2">
                  <Label htmlFor="repo-template-repo">{t("repos.templateRepoLabel")}</Label>
                  <select
                    id="repo-template-repo"
                    value={templateRepo}
                    onChange={(e) => {
                      setTemplateRepo(e.target.value);
                      if (e.target.value) setTemplate("");
                    }}
                    className="h-10 w-full min-w-0 rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <option value="">{t("repos.templateRepoNone")}</option>
                    {templateRepos.map((r) => (
                      <option key={`${r.owner}/${r.name}`} value={`${r.owner}/${r.name}`}>
                        {r.owner}/{r.name}
                      </option>
                    ))}
                  </select>
                </div>
              )}
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
                <RepoCard
                  key={`${repo.owner}/${repo.name}`}
                  repo={repo}
                  isMine={isMine}
                  isOwner={isOwner}
                  locale={dateLocale(lang)}
                  onManageCollabs={setCollabRepo}
                  onManageWebhooks={setHookRepo}
                  onDelete={setPendingDelete}
                />
              );
            })}
          </div>
        )}

        {isMine && !loading && !error && repoTotal > 0 && (
          <Pagination
            className="mt-4"
            page={page}
            pageSize={pageSize}
            total={repoTotal}
            onPageChange={onPageChange}
            onPageSizeChange={onPageSizeChange}
          />
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
      <ImportRepoDialog
        open={importOpen}
        onOpenChange={setImportOpen}
        url={importUrl}
        onUrl={setImportUrl}
        name={importName}
        onName={setImportName}
        privateRepo={importPrivate}
        onPrivate={setImportPrivate}
        key={importKey}
        onKey={setImportKey}
        busy={importBusy}
        onImport={doImport}
      />
    </div>
  );
}
