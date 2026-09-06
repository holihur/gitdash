import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Check, Copy, KeyRound, Plus, Trash2, KeySquare } from "lucide-react";
import { api, type CreatedPAT, type PAT, type SSHKey } from "@/lib/api";
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
import { Textarea } from "@/components/ui/textarea";
import { Skeleton } from "@/components/ui/skeleton";
import ConfirmDialog from "@/components/confirm-dialog";
import { copyText, formatDate } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

const SCOPE_LABEL_KEY: Record<string, string> = {
  repo: "pats.scopeRepo",
  inbox: "pats.scopeInbox",
  keys: "pats.scopeKeys",
};

export default function Keys() {
  const { t, lang, to } = useI18n();
  const locale = lang === "zh-CN" ? "zh-CN" : "en-US";
  const [tab, setTab] = useState("ssh");
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("keys.title")}</h1>
      </div>
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="ssh" className="gap-2">
            <KeyRound className="h-4 w-4" />
            SSH Keys
          </TabsTrigger>
          <TabsTrigger value="pats" className="gap-2">
            <KeySquare className="h-4 w-4" />
            {t("pats.title")}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="ssh" className="mt-4">
          <SSHKeysSection t={t} to={to} locale={locale} />
        </TabsContent>
        <TabsContent value="pats" className="mt-4">
          <PATSection t={t} to={to} locale={locale} />
        </TabsContent>
      </Tabs>
    </div>
  );
}

const SectionProps = { t: {} as (key: string, vars?: Record<string, string | number>) => string, to: {} as (key: string, vars?: Record<string, string | number>) => string | undefined, locale: "" };
type SectionProps = typeof SectionProps;

function SSHKeysSection({ t, to, locale }: SectionProps) {
  const [keys, setKeys] = useState<SSHKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [publicKey, setPublicKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<SSHKey | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setKeys(await api.listKeys());
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const create = async () => {
    setBusy(true);
    try {
      await api.createKey(name.trim(), publicKey.trim());
      toast.success(t("keys.added", { name: name.trim() }));
      setOpen(false);
      setName("");
      setPublicKey("");
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (key: SSHKey) => {
    setPendingDelete(null);
    try {
      await api.deleteKey(key.id);
      toast.success(t("keys.deleted"));
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-sm text-muted-foreground">{t("keys.subtitle")}</p>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button className="gap-2 sm:self-start">
              <Plus className="h-4 w-4" />
              {t("keys.add")}
            </Button>
          </DialogTrigger>
          <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-xl">
            <DialogHeader>
              <DialogTitle>{t("keys.addTitle")}</DialogTitle>
              <DialogDescription>
                {t("keys.addDescription")}{" "}
                <code>ssh-ed25519 AAAA... user@host</code>
              </DialogDescription>
            </DialogHeader>
            <div className="grid gap-4">
              <div className="grid gap-2">
                <Label htmlFor="key-name">{t("keys.name")}</Label>
                <Input
                  id="key-name"
                  placeholder={t("keys.namePlaceholder")}
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="key-pub">{t("keys.publicKey")}</Label>
                <Textarea
                  id="key-pub"
                  placeholder={t("keys.publicKeyPlaceholder")}
                  className="min-h-32 font-mono text-xs"
                  value={publicKey}
                  onChange={(e) => setPublicKey(e.target.value)}
                />
              </div>
            </div>
            <DialogFooter>
              <Button onClick={create} disabled={busy || !name.trim() || !publicKey.trim()}>
                {t("common.add")}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      {loading ? (
        <div className="space-y-3 rounded-lg border p-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-6 w-full" />
          ))}
        </div>
      ) : keys.length === 0 ? (
        <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed py-16 px-4 text-center">
          <KeyRound className="h-10 w-10 text-muted-foreground" />
          <p className="font-medium">{t("keys.empty")}</p>
          <p className="text-sm text-muted-foreground">{t("keys.emptyHint")}</p>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <Table className="min-w-[680px]">
            <TableHeader>
              <TableRow>
                <TableHead>{t("keys.name")}</TableHead>
                <TableHead>{t("keys.fingerprint")}</TableHead>
                <TableHead>{t("keys.keyType")}</TableHead>
                <TableHead>{t("keys.addedAt")}</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {keys.map((key) => (
                <TableRow key={key.id}>
                  <TableCell className="font-medium">{key.name}</TableCell>
                  <TableCell className="font-mono text-xs">{key.fingerprint}</TableCell>
                  <TableCell>
                    <Badge variant="secondary">{key.public_key.split(" ")[0]}</Badge>
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(key.created_at, locale)}
                  </TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8 text-destructive hover:text-destructive"
                      onClick={() => setPendingDelete(key)}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(o) => !o && setPendingDelete(null)}
        description={t("keys.confirmDelete", { name: pendingDelete?.name ?? "" })}
        onConfirm={() => pendingDelete && remove(pendingDelete)}
      />
    </div>
  );
}

function PATSection({ t, to, locale }: SectionProps) {
  const [pats, setPats] = useState<PAT[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [scopes, setScopes] = useState<string[]>(["repo"]);
  const [busy, setBusy] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<PAT | null>(null);
  const [created, setCreated] = useState<CreatedPAT | null>(null);
  const [copied, setCopied] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setPats(await api.listPATs());
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const toggleScope = (s: string) => {
    setScopes((prev) => (prev.includes(s) ? prev.filter((x) => x !== s) : [...prev, s]));
  };

  const create = async () => {
    setBusy(true);
    try {
      const pat = await api.createPAT(name.trim(), scopes);
      setOpen(false);
      setName("");
      setScopes(["repo"]);
      setCreated(pat);
      setCopied(false);
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (pat: PAT) => {
    setPendingDelete(null);
    try {
      await api.deletePAT(pat.id);
      toast.success(t("keys.deleted"));
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const scopeLabel = (s: string) =>
    SCOPE_LABEL_KEY[s] ? t(SCOPE_LABEL_KEY[s]) : s;

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-sm text-muted-foreground">{t("pats.subtitle")}</p>
        <Dialog
          open={open}
          onOpenChange={(o) => {
            setOpen(o);
            if (o) setCreated(null);
          }}
        >
          <DialogTrigger asChild>
            <Button className="gap-2 sm:self-start">
              <Plus className="h-4 w-4" />
              {t("pats.create")}
            </Button>
          </DialogTrigger>
          <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-xl">
            {created ? (
              <>
                <DialogHeader>
                  <DialogTitle>{t("pats.createdTitle")}</DialogTitle>
                  <DialogDescription>{t("pats.createdHint")}</DialogDescription>
                </DialogHeader>
                <div className="flex items-center gap-2">
                  <code className="min-w-0 flex-1 truncate rounded bg-muted px-2 py-2 font-mono text-xs">
                    {created.token}
                  </code>
                  <Button
                    variant="outline"
                    size="icon"
                    className="shrink-0"
                    onClick={() =>
                      copyText(created.token)
                        .then(() => {
                          setCopied(true);
                          toast.success(t("common.copied"));
                        })
                        .catch(() => toast.error(t("common.copyFailed")))
                    }
                  >
                    {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                  </Button>
                </div>
                <DialogFooter>
                  <Button onClick={() => setCreated(null)}>{t("common.save")}</Button>
                </DialogFooter>
              </>
            ) : (
              <>
                <DialogHeader>
                  <DialogTitle>{t("pats.create")}</DialogTitle>
                </DialogHeader>
                <div className="grid gap-4">
                  <div className="grid gap-2">
                    <Label htmlFor="pat-name">{t("pats.name")}</Label>
                    <Input
                      id="pat-name"
                      placeholder={t("pats.namePlaceholder")}
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                    />
                  </div>
                  <div className="grid gap-2">
                    <Label>{t("pats.scopes")}</Label>
                    <div className="flex flex-wrap gap-4">
                      {["repo", "inbox", "keys"].map((s) => (
                        <label key={s} className="flex items-center gap-2 text-sm">
                          <input
                            type="checkbox"
                            className="h-4 w-4 accent-primary"
                            checked={scopes.includes(s)}
                            onChange={() => toggleScope(s)}
                          />
                          {scopeLabel(s)}
                        </label>
                      ))}
                    </div>
                  </div>
                </div>
                <DialogFooter>
                  <Button onClick={create} disabled={busy || !name.trim() || scopes.length === 0}>
                    {t("pats.create")}
                  </Button>
                </DialogFooter>
              </>
            )}
          </DialogContent>
        </Dialog>
      </div>

      {loading ? (
        <div className="space-y-3 rounded-lg border p-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-6 w-full" />
          ))}
        </div>
      ) : pats.length === 0 ? (
        <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed py-16 px-4 text-center">
          <KeySquare className="h-10 w-10 text-muted-foreground" />
          <p className="font-medium">{t("pats.empty")}</p>
          <p className="text-sm text-muted-foreground">{t("pats.emptyHint")}</p>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <Table className="min-w-[680px]">
            <TableHeader>
              <TableRow>
                <TableHead>{t("pats.name")}</TableHead>
                <TableHead>{t("pats.scopes")}</TableHead>
                <TableHead>{t("pats.created")}</TableHead>
                <TableHead>{t("pats.lastUsed")}</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {pats.map((pat) => (
                <TableRow key={pat.id}>
                  <TableCell className="font-medium">{pat.name}</TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {pat.scopes.map((s) => (
                        <Badge key={s} variant="secondary">
                          {scopeLabel(s)}
                        </Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(pat.created_at, locale)}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {pat.last_used_at ? formatDate(pat.last_used_at, locale) : t("pats.never")}
                  </TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8 text-destructive hover:text-destructive"
                      onClick={() => setPendingDelete(pat)}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(o) => !o && setPendingDelete(null)}
        description={t("pats.confirmDelete", { name: pendingDelete?.name ?? "" })}
        onConfirm={() => pendingDelete && remove(pendingDelete)}
      />
    </div>
  );
}
