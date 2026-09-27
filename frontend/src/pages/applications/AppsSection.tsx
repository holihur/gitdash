import type { ReactNode } from "react";
import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { KeyRound, Plus, Trash2 } from "lucide-react";
import { api, type CreatedOAuthApp, type OAuthApp } from "@/lib/api";
import { Button } from "@/components/ui/button";
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
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import ConfirmDialog from "@/components/confirm-dialog";
import { RelativeTime } from "@/components/relative-time";
import { apiErrorMsg } from "@/lib/errors";
import { SecretDialog } from "./SecretDialog";

type Translator = (key: string, vars?: Record<string, string | number>) => string;

const REDIRECT_URI = "redirect_uri";

/** 把译文里的 redirect_uri 拆出来包进 <code>，其余部分原样输出。 */
function withCodeToken(text: string): ReactNode[] {
  const at = text.indexOf(REDIRECT_URI);
  if (at < 0) return [text];
  return [
    text.slice(0, at),
    <code key={REDIRECT_URI}>{REDIRECT_URI}</code>,
    text.slice(at + REDIRECT_URI.length),
  ];
}

export function AppsSection({
  t,
  to,
  locale,
}: {
  t: Translator;
  to: (k: string, vars?: Record<string, string | number>) => string | undefined;
  locale: string;
}) {
  const [apps, setApps] = useState<OAuthApp[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [homepage, setHomepage] = useState("");
  const [description, setDescription] = useState("");
  const [callbackUrl, setCallbackUrl] = useState("");
  const [busy, setBusy] = useState(false);
  const [created, setCreated] = useState<CreatedOAuthApp | null>(null);
  const [resetSecret, setResetSecret] = useState<{ id: number; secret: string } | null>(null);
  const [pendingDelete, setPendingDelete] = useState<OAuthApp | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setApps(await api.listApps());
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setLoading(false);
    }
  }, [to]);

  useEffect(() => {
    load();
  }, [load]);

  const create = async () => {
    setBusy(true);
    try {
      const app = await api.createApp({
        name: name.trim(),
        homepage: homepage.trim(),
        description: description.trim(),
        callback_url: callbackUrl.trim(),
      });
      setOpen(false);
      setName("");
      setHomepage("");
      setDescription("");
      setCallbackUrl("");
      setCreated(app);
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const reset = async (app: OAuthApp) => {
    try {
      const r = await api.resetSecret(app.id);
      setResetSecret({ id: app.id, secret: r.client_secret });
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const remove = async (app: OAuthApp) => {
    setPendingDelete(null);
    try {
      await api.deleteApp(app.id);
      toast.success(t("oauthApps.deleted", { name: app.name }));
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-sm text-muted-foreground">{t("oauthApps.secretHint")}</p>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button className="gap-2 sm:self-start">
              <Plus className="h-4 w-4" />
              {t("oauthApps.create")}
            </Button>
          </DialogTrigger>
          <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-xl">
            <DialogHeader>
              <DialogTitle>{t("oauthApps.createTitle")}</DialogTitle>
              <DialogDescription>
                {withCodeToken(t("oauthApps.createHint", { uri: REDIRECT_URI }))}
              </DialogDescription>
            </DialogHeader>
            <div className="grid gap-4">
              <div className="grid gap-2">
                <Label htmlFor="app-name">{t("oauthApps.nameLabel")}</Label>
                <Input
                  id="app-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder={t("oauthApps.namePlaceholder")}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="app-home">{t("oauthApps.homepageLabel")}</Label>
                <Input
                  id="app-home"
                  value={homepage}
                  onChange={(e) => setHomepage(e.target.value)}
                  placeholder="https://example.com"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="app-desc">{t("oauthApps.descriptionLabel")}</Label>
                <Input
                  id="app-desc"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="app-cb">{t("oauthApps.callbackLabel")}</Label>
                <Input
                  id="app-cb"
                  value={callbackUrl}
                  onChange={(e) => setCallbackUrl(e.target.value)}
                  placeholder="https://example.com/oauth/callback"
                />
              </div>
            </div>
            <DialogFooter>
              <Button
                onClick={create}
                disabled={busy || !name.trim() || !callbackUrl.trim()}
              >
                {t("oauthApps.register")}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      {loading ? (
        <div className="space-y-3 rounded-lg border p-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-6 w-full" />
          ))}
        </div>
      ) : apps.length === 0 ? (
        <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed py-16 px-4 text-center">
          <KeyRound className="h-10 w-10 text-muted-foreground" />
          <p className="font-medium">{t("oauthApps.empty")}</p>
          <p className="text-sm text-muted-foreground">{t("oauthApps.emptyHint")}</p>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <Table className="min-w-[680px]">
            <TableHeader>
              <TableRow>
                <TableHead>{t("oauthApps.name")}</TableHead>
                <TableHead>{t("oauthApps.clientId")}</TableHead>
                <TableHead>{t("oauthApps.callbackUrl")}</TableHead>
                <TableHead>{t("oauthApps.created")}</TableHead>
                <TableHead className="w-40" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {apps.map((app) => (
                <TableRow key={app.id}>
                  <TableCell className="font-medium">{app.name}</TableCell>
                  <TableCell className="font-mono text-xs">{app.client_id}</TableCell>
                  <TableCell className="max-w-48 truncate text-xs text-muted-foreground">
                    {app.callback_url}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    <RelativeTime iso={app.created_at} locale={locale} />
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center justify-end gap-1">
                      <Button variant="ghost" size="sm" onClick={() => reset(app)}>
                        {t("oauthApps.resetSecret")}
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={t("common.delete")}
                        onClick={() => setPendingDelete(app)}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <SecretDialog
        open={created !== null || resetSecret !== null}
        onOpenChange={() => {
          setCreated(null);
          setResetSecret(null);
        }}
        title={t("oauthApps.secretTitle")}
        description={t("oauthApps.secretBody")}
        secret={created?.client_secret ?? resetSecret?.secret ?? ""}
      />

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(o) => !o && setPendingDelete(null)}
        title={t("oauthApps.deleteTitle")}
        description={t("oauthApps.deleteConfirm", { name: pendingDelete?.name ?? "" })}
        confirmText={t("common.delete")}
        onConfirm={() => pendingDelete && remove(pendingDelete)}
      />
    </div>
  );
}
