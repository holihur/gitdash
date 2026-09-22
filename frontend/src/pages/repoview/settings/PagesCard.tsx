import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { ExternalLink, Globe } from "lucide-react";
import { api } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

/** 静态网站托管（Pages）配置（仅 owner）。默认关闭。 */
export function PagesCard({ owner, name }: { owner: string; name: string }) {
  const { t, to } = useI18n();
  const [enabled, setEnabled] = useState(false);
  const [branch, setBranch] = useState("");
  const [dir, setDir] = useState("");
  const [url, setUrl] = useState("");
  const [branches, setBranches] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [loaded, setLoaded] = useState(false);

  const load = useCallback(async () => {
    try {
      const [cfg, bs] = await Promise.all([
        api.getRepoPages(owner, name),
        api.branches(owner, name),
      ]);
      setEnabled(cfg.enabled);
      setBranch(cfg.branch);
      setDir(cfg.dir);
      setUrl(cfg.url);
      setBranches((bs ?? []).map((b) => b.name));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setLoaded(true);
    }
  }, [owner, name, to]);

  useEffect(() => {
    void load();
  }, [load]);

  const save = async (nextEnabled: boolean) => {
    setBusy(true);
    try {
      const cfg = await api.setRepoPages(owner, name, {
        enabled: nextEnabled,
        branch: branch.trim(),
        dir: dir.trim(),
      });
      setEnabled(cfg.enabled);
      setBranch(cfg.branch);
      setDir(cfg.dir);
      setUrl(cfg.url);
      toast.success(t("repo.pagesSaved"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Globe className="h-4 w-4" />
          {t("repo.pagesTitle")}
        </CardTitle>
        <CardDescription>{t("repo.pagesDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {!loaded ? (
          <p className="text-sm text-muted-foreground">…</p>
        ) : (
          <>
            <div className="flex flex-wrap items-center gap-3">
              <Badge variant={enabled ? "secondary" : "outline"}>
                {enabled ? t("repo.pagesEnabled") : t("repo.pagesDisabled")}
              </Badge>
              {enabled && url && (
                <a
                  href={url}
                  target="_blank"
                  rel="noreferrer noopener"
                  className="inline-flex items-center gap-1 text-sm text-primary hover:underline"
                >
                  <ExternalLink className="h-3.5 w-3.5" />
                  {url}
                </a>
              )}
            </div>

            <div className="grid gap-3 sm:grid-cols-2">
              <div className="grid gap-1.5">
                <Label htmlFor="pages-branch">{t("repo.pagesBranch")}</Label>
                <select
                  id="pages-branch"
                  value={branch}
                  onChange={(e) => setBranch(e.target.value)}
                  className="h-9 rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <option value="">{t("repo.pagesDefaultBranch")}</option>
                  {branches.map((b) => (
                    <option key={b} value={b}>
                      {b}
                    </option>
                  ))}
                </select>
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="pages-dir">{t("repo.pagesDir")}</Label>
                <Input
                  id="pages-dir"
                  value={dir}
                  onChange={(e) => setDir(e.target.value)}
                  placeholder="/"
                />
              </div>
            </div>
            <p className="text-xs text-muted-foreground">{t("repo.pagesDirHint")}</p>

            <div className="flex flex-wrap gap-2">
              <Button size="sm" disabled={busy} onClick={() => save(!enabled)}>
                {enabled ? t("repo.pagesDisable") : t("repo.pagesEnable")}
              </Button>
              {enabled && (
                <Button size="sm" variant="outline" disabled={busy} onClick={() => save(true)}>
                  {t("repo.pagesSave")}
                </Button>
              )}
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}
