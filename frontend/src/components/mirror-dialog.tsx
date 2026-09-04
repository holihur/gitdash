import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { RefreshCw, Save, Trash2 } from "lucide-react";
import { api } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
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
import { Textarea } from "@/components/ui/textarea";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  owner: string;
  repo: string;
}

export default function MirrorDialog({ open, onOpenChange, owner, repo }: Props) {
  const { t, to } = useI18n();
  const [url, setUrl] = useState("");
  const [key, setKey] = useState("");
  const [configured, setConfigured] = useState(false);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const m = await api.getMirror(owner, repo);
      setUrl(m.url ?? "");
      setConfigured(!!m.url);
    } catch {
      setConfigured(false);
    }
  }, [owner, repo]);

  useEffect(() => {
    if (open) {
      setUrl("");
      setKey("");
      setConfigured(false);
      load();
    }
  }, [open, load]);

  const save = async () => {
    if (!url.trim()) return;
    setBusy(true);
    try {
      await api.setMirror(owner, repo, url.trim(), key.trim() || undefined);
      toast.success(t("mirror.saved"));
      setKey("");
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    setBusy(true);
    try {
      await api.deleteMirror(owner, repo);
      toast.success(t("mirror.removed"));
      setUrl("");
      setKey("");
      setConfigured(false);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const sync = async () => {
    setBusy(true);
    try {
      await api.syncMirror(owner, repo);
      toast.success(t("mirror.synced"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="truncate">
            {owner}/{repo} · {t("mirror.title")}
          </DialogTitle>
          <DialogDescription>{t("mirror.hint")}</DialogDescription>
        </DialogHeader>

        <div className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="mirror-url">{t("mirror.urlLabel")}</Label>
            <Input
              id="mirror-url"
              placeholder="https://github.com/you/repo.git"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="mirror-key">{t("mirror.keyLabel")}</Label>
            <Textarea
              id="mirror-key"
              rows={4}
              placeholder={t("mirror.keyPlaceholder")}
              value={key}
              onChange={(e) => setKey(e.target.value)}
              className="font-mono text-xs"
            />
            <p className="text-xs text-muted-foreground">{t("mirror.keyHint")}</p>
          </div>
        </div>

        <DialogFooter className="gap-2 sm:gap-2">
          {configured && (
            <Button
              variant="ghost"
              className="mr-auto text-destructive hover:text-destructive"
              disabled={busy}
              onClick={remove}
            >
              <Trash2 className="h-4 w-4" />
              {t("mirror.remove")}
            </Button>
          )}
          <Button onClick={save} disabled={busy || !url.trim()}>
            <Save className="h-4 w-4" />
            {t("mirror.save")}
          </Button>
          <Button variant="outline" onClick={sync} disabled={busy || !configured}>
            <RefreshCw className="h-4 w-4" />
            {t("mirror.syncNow")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
