import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { CheckCircle2, Download, Loader2, Lock, RefreshCw } from "lucide-react";
import { api, type Connection, type RemoteRepo } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  url: string;
  onUrl: (v: string) => void;
  name: string;
  onName: (v: string) => void;
  privateRepo: boolean;
  onPrivate: (v: boolean) => void;
  key: string;
  onKey: (v: string) => void;
  busy: boolean;
  onImport: () => void;
  /** 批量导入完成（或部分完成）后回调，用于刷新仓库列表。 */
  onBatchDone?: () => void;
}

/** 从第三方远端导入仓库：支持直接填 URL，或选择已绑定账号批量导入。 */
export default function ImportRepoDialog({
  open,
  onOpenChange,
  url,
  onUrl,
  name,
  onName,
  privateRepo,
  onPrivate,
  key,
  onKey,
  busy,
  onImport,
  onBatchDone,
}: Props) {
  const { t } = useI18n();
  const [tab, setTab] = useState("url");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("imports.importTitle")}</DialogTitle>
          <DialogDescription>{t("imports.importDescription")}</DialogDescription>
        </DialogHeader>
        <Tabs value={tab} onValueChange={setTab}>
          <TabsList className="grid w-full grid-cols-2">
            <TabsTrigger value="url">{t("imports.tabUrl")}</TabsTrigger>
            <TabsTrigger value="account">{t("imports.tabAccount")}</TabsTrigger>
          </TabsList>
          <TabsContent value="url" className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="import-url">{t("imports.urlLabel")}</Label>
              <Input
                id="import-url"
                placeholder="https://github.com/owner/repo.git"
                value={url}
                onChange={(e) => onUrl(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="import-name">{t("imports.nameLabel")}</Label>
              <Input
                id="import-name"
                placeholder={t("common.optional")}
                value={name}
                onChange={(e) => onName(e.target.value)}
              />
            </div>
            <div className="flex items-center gap-2">
              <input
                id="import-private"
                type="checkbox"
                checked={privateRepo}
                onChange={(e) => onPrivate(e.target.checked)}
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
                value={key}
                onChange={(e) => onKey(e.target.value)}
                className="font-mono text-xs"
              />
              <p className="text-xs text-muted-foreground">{t("imports.keyHint")}</p>
            </div>
            <DialogFooter>
              <Button onClick={onImport} disabled={busy || !url.trim()}>
                {t("imports.import")}
              </Button>
            </DialogFooter>
          </TabsContent>
          <TabsContent value="account">
            <ConnectedAccountImport
              active={open && tab === "account"}
              privateRepo={privateRepo}
              onPrivate={onPrivate}
              onDone={() => {
                onBatchDone?.();
                onOpenChange(false);
              }}
            />
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}

function ConnectedAccountImport({
  active,
  privateRepo,
  onPrivate,
  onDone,
}: {
  active: boolean;
  privateRepo: boolean;
  onPrivate: (v: boolean) => void;
  onDone: () => void;
}) {
  const { t, to } = useI18n();
  const [connections, setConnections] = useState<Connection[]>([]);
  const [provider, setProvider] = useState("");
  const [loadingConns, setLoadingConns] = useState(false);
  const [repos, setRepos] = useState<RemoteRepo[]>([]);
  const [loadingRepos, setLoadingRepos] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState(false);

  const loadConnections = useCallback(async () => {
    setLoadingConns(true);
    try {
      const all = await api.listConnections();
      const usable = all.filter((c) => c.enabled && c.connected);
      setConnections(usable);
      setProvider((prev) => (prev && usable.some((c) => c.provider === prev) ? prev : usable[0]?.provider ?? ""));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setLoadingConns(false);
    }
  }, [to]);

  const loadRepos = useCallback(
    async (p: string) => {
      if (!p) {
        setRepos([]);
        return;
      }
      setLoadingRepos(true);
      setSelected(new Set());
      try {
        setRepos(await api.listRemoteRepos(p));
      } catch (e) {
        setRepos([]);
        toast.error(apiErrorMsg(to, e));
      } finally {
        setLoadingRepos(false);
      }
    },
    [to],
  );

  useEffect(() => {
    if (active) {
      void loadConnections();
    }
  }, [active, loadConnections]);

  useEffect(() => {
    if (active) {
      void loadRepos(provider);
    }
  }, [active, provider, loadRepos]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return repos;
    return repos.filter(
      (r) => r.full_name.toLowerCase().includes(q) || r.description?.toLowerCase().includes(q),
    );
  }, [repos, query]);

  const toggle = (full: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(full)) next.delete(full);
      else next.add(full);
      return next;
    });
  };

  const toggleAll = () => {
    setSelected((prev) => (prev.size === filtered.length ? new Set() : new Set(filtered.map((r) => r.full_name))));
  };

  const runBatch = async () => {
    if (!provider || selected.size === 0) return;
    setBusy(true);
    try {
      const res = await api.batchImport({
        provider,
        private: privateRepo,
        repos: [...selected],
      });
      const skipped = Object.keys(res.skipped).length;
      if (res.imported.length > 0) {
        toast.success(t("imports.batchQueued", { count: res.imported.length }));
      }
      if (skipped > 0) {
        toast.warning(t("imports.batchSkipped", { count: skipped }));
      }
      onDone();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  if (loadingConns) {
    return <p className="py-8 text-center text-sm text-muted-foreground">…</p>;
  }
  if (connections.length === 0) {
    return (
      <div className="grid gap-3 py-6 text-center text-sm text-muted-foreground">
        <p>{t("imports.noConnections")}</p>
        <a href="/profile" className="text-primary underline-offset-4 hover:underline">
          {t("imports.goToConnections")}
        </a>
      </div>
    );
  }

  const provider_ = connections.find((c) => c.provider === provider);

  return (
    <div className="grid gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <select
          className="h-9 rounded-md border border-input bg-background px-3 text-sm"
          value={provider}
          onChange={(e) => setProvider(e.target.value)}
        >
          {connections.map((c) => (
            <option key={c.provider} value={c.provider}>
              {c.label} ({c.login})
            </option>
          ))}
        </select>
        <Button
          variant="outline"
          size="sm"
          className="gap-2"
          onClick={() => loadRepos(provider)}
          disabled={loadingRepos}
        >
          <RefreshCw className="h-4 w-4" />
          {t("imports.refresh")}
        </Button>
        <Input
          className="h-9 sm:w-56"
          placeholder={t("imports.filterRepos")}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <label className="ml-auto flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={privateRepo}
            onChange={(e) => onPrivate(e.target.checked)}
            className="h-4 w-4"
          />
          {t("imports.privateLabel")}
        </label>
      </div>
      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span>
          {provider_?.base_url} · {t("imports.repoCount", { count: filtered.length })}
        </span>
        <button type="button" className="underline-offset-4 hover:underline" onClick={toggleAll}>
          {selected.size === filtered.length && filtered.length > 0 ? t("imports.clearAll") : t("imports.selectAll")}
        </button>
      </div>
      <div className="max-h-[45vh] min-h-[8rem] overflow-auto rounded-md border">
        {loadingRepos ? (
          <p className="flex items-center justify-center gap-2 py-10 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            {t("imports.loadingRepos")}
          </p>
        ) : filtered.length === 0 ? (
          <p className="py-10 text-center text-sm text-muted-foreground">{t("imports.noRepos")}</p>
        ) : (
          <ul className="divide-y">
            {filtered.map((r) => (
              <li key={r.full_name}>
                <label className="flex cursor-pointer items-center gap-3 px-3 py-2 text-sm hover:bg-accent">
                  <input
                    type="checkbox"
                    className="h-4 w-4"
                    checked={selected.has(r.full_name)}
                    onChange={() => toggle(r.full_name)}
                  />
                  <span className="min-w-0 flex-1">
                    <span className="flex items-center gap-2">
                      <span className="truncate font-medium">{r.full_name}</span>
                      {r.private && (
                        <Badge variant="secondary" className="gap-1">
                          <Lock className="h-3 w-3" />
                          {t("imports.private")}
                        </Badge>
                      )}
                    </span>
                    {r.description && (
                      <span className="block truncate text-xs text-muted-foreground">{r.description}</span>
                    )}
                  </span>
                  {selected.has(r.full_name) && <CheckCircle2 className="h-4 w-4 text-primary" />}
                </label>
              </li>
            ))}
          </ul>
        )}
      </div>
      <DialogFooter>
        <Button onClick={runBatch} disabled={busy || selected.size === 0}>
          <Download className="h-4 w-4" />
          {busy ? t("imports.importing") : t("imports.batchImport", { count: selected.size })}
        </Button>
      </DialogFooter>
    </div>
  );
}
