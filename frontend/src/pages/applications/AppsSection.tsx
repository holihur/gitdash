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

export function AppsSection({ to, locale }: { to: (k: string) => string | undefined; locale: string }) {
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
                    <RelativeTime iso={app.created_at} locale={locale} />
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

