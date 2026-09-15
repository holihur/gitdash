import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Check, Copy, KeyRound, Plus, Trash2 } from "lucide-react";
import { api, type CreatedOAuthApp, type OAuthApp, type OAuthAuthorization } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import ConfirmDialog from "@/components/confirm-dialog";
import { copyText, formatDate } from "@/lib/utils";
import { dateLocale, useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

export default function Applications() {
  const { to, lang } = useI18n();
  const locale = dateLocale(lang);
  const [tab, setTab] = useState("apps");

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">OAuth Applications</h1>
        <p className="text-sm text-muted-foreground">
          Register OAuth 2.0 applications to let third parties access your gitdash account.
        </p>
      </div>
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="apps" className="gap-2">
            <KeyRound className="h-4 w-4" />
            OAuth Apps
          </TabsTrigger>
          <TabsTrigger value="authorized" className="gap-2">
            Authorized Apps
          </TabsTrigger>
        </TabsList>
        <TabsContent value="apps" className="mt-4">
          <AppsSection to={to} locale={locale} />
        </TabsContent>
        <TabsContent value="authorized" className="mt-4">
          <AuthorizedSection to={to} locale={locale} />
        </TabsContent>
      </Tabs>
    </div>
  );
}

function AppsSection({ to, locale }: { to: (k: string) => string | undefined; locale: string }) {
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
      toast.success(`Deleted "${app.name}"`);
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-sm text-muted-foreground">
          The client secret is only shown once at creation; reset it if it is lost.
        </p>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button className="gap-2 sm:self-start">
              <Plus className="h-4 w-4" />
              New OAuth App
            </Button>
          </DialogTrigger>
          <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-xl">
            <DialogHeader>
              <DialogTitle>Register a new OAuth application</DialogTitle>
              <DialogDescription>
                Callback URL must exactly match the <code>redirect_uri</code> used in the
                authorization request.
              </DialogDescription>
            </DialogHeader>
            <div className="grid gap-4">
              <div className="grid gap-2">
                <Label htmlFor="app-name">Application name</Label>
                <Input
                  id="app-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="My App"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="app-home">Homepage URL</Label>
                <Input
                  id="app-home"
                  value={homepage}
                  onChange={(e) => setHomepage(e.target.value)}
                  placeholder="https://example.com"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="app-desc">Description (optional)</Label>
                <Input
                  id="app-desc"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="app-cb">Callback URL</Label>
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
                Register application
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
          <p className="font-medium">No OAuth applications yet</p>
          <p className="text-sm text-muted-foreground">
            Register an application to get a client ID and secret.
          </p>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <Table className="min-w-[680px]">
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Client ID</TableHead>
                <TableHead>Callback URL</TableHead>
                <TableHead>Created</TableHead>
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
                    {formatDate(app.created_at, locale)}
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center justify-end gap-1">
                      <Button variant="ghost" size="sm" onClick={() => reset(app)}>
                        Reset secret
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
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
        title="Client secret"
        description="Copy this secret now. You will not be able to see it again."
        secret={created?.client_secret ?? resetSecret?.secret ?? ""}
      />

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(o) => !o && setPendingDelete(null)}
        title="Delete OAuth application?"
        description={`This permanently deletes "${pendingDelete?.name ?? ""}" and revokes all tokens issued to it.`}
        confirmText="Delete"
        onConfirm={() => pendingDelete && remove(pendingDelete)}
      />
    </div>
  );
}

function AuthorizedSection({ to, locale }: { to: (k: string) => string | undefined; locale: string }) {
  const [auths, setAuths] = useState<OAuthAuthorization[]>([]);
  const [loading, setLoading] = useState(true);
  const [pendingRevoke, setPendingRevoke] = useState<OAuthAuthorization | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setAuths(await api.listAuthorizations());
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setLoading(false);
    }
  }, [to]);

  useEffect(() => {
    load();
  }, [load]);

  const revoke = async (auth: OAuthAuthorization) => {
    setPendingRevoke(null);
    try {
      await api.revokeAuthorization(auth.id);
      toast.success(`Revoked access for "${auth.app_name}"`);
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  if (loading) {
    return (
      <div className="space-y-3 rounded-lg border p-4">
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} className="h-6 w-full" />
        ))}
      </div>
    );
  }
  if (auths.length === 0) {
    return (
      <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed py-16 px-4 text-center">
        <KeyRound className="h-10 w-10 text-muted-foreground" />
        <p className="font-medium">No authorized applications</p>
        <p className="text-sm text-muted-foreground">
          Applications you authorize will appear here so you can revoke access at any time.
        </p>
      </div>
    );
  }
  return (
    <div className="space-y-6">
      <div className="overflow-x-auto rounded-lg border">
        <Table className="min-w-[680px]">
          <TableHeader>
            <TableRow>
              <TableHead>Application</TableHead>
              <TableHead>Scopes</TableHead>
              <TableHead>Authorized</TableHead>
              <TableHead>Last used</TableHead>
              <TableHead className="w-24" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {auths.map((auth) => (
              <TableRow key={auth.id}>
                <TableCell className="font-medium">{auth.app_name}</TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-1">
                    {auth.scopes.map((s) => (
                      <Badge key={s} variant="secondary">
                        {s}
                      </Badge>
                    ))}
                  </div>
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">
                  {formatDate(auth.created_at, locale)}
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">
                  {auth.last_used_at ? formatDate(auth.last_used_at, locale) : "—"}
                </TableCell>
                <TableCell>
                  <Button variant="ghost" size="sm" onClick={() => setPendingRevoke(auth)}>
                    Revoke
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      <ConfirmDialog
        open={pendingRevoke !== null}
        onOpenChange={(o) => !o && setPendingRevoke(null)}
        title="Revoke access?"
        description={`The access token issued to "${pendingRevoke?.app_name ?? ""}" will stop working immediately.`}
        confirmText="Revoke"
        onConfirm={() => pendingRevoke && revoke(pendingRevoke)}
      />
    </div>
  );
}

function SecretDialog({
  open,
  onOpenChange,
  title,
  description,
  secret,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  secret: string;
}) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    await copyText(secret);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <div className="flex items-center gap-2 rounded-lg border bg-muted/40 p-3">
          <code className="min-w-0 flex-1 break-all font-mono text-xs">{secret}</code>
          <Button variant="outline" size="icon" onClick={copy} title="Copy">
            {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
          </Button>
        </div>
        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>Done</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
