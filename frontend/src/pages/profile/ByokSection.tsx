import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Bot, Loader2, Pencil, PlugZap, Plus, Trash2 } from "lucide-react";
import { api, type ByokKey } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";

export const PROVIDERS = [
  { value: "anthropic", keyRequired: true, baseUrl: "https://api.anthropic.com", model: "claude-sonnet-4-5" },
  { value: "compatible", keyRequired: true, baseUrl: "", model: "" },
  { value: "ollama", keyRequired: false, baseUrl: "http://127.0.0.1:11434", model: "" },
] as const;

export type ByokProvider = (typeof PROVIDERS)[number]["value"];

export function providerLabel(t: (k: string) => string, provider: string): string {
  const key = `byok.provider.${provider}`;
  const label = t(key);
  return label === key ? provider : label;
}

export function ByokSection() {
  const { t, to } = useI18n();
  const [keys, setKeys] = useState<ByokKey[] | null>(null);
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<ByokKey | null>(null);
  const [name, setName] = useState("");
  const [provider, setProvider] = useState("anthropic");
  const [apiKey, setApiKey] = useState("");
  const [baseUrl, setBaseUrl] = useState("");
  const [model, setModel] = useState("");
  const [busy, setBusy] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ ok: boolean; msg: string } | null>(null);

  const load = useCallback(async () => {
    try {
      setKeys(await api.listByok());
    } catch {
      setKeys([]);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const openAdd = () => {
    setEditing(null);
    setName("");
    setProvider("anthropic");
    setApiKey("");
    setBaseUrl("");
    setModel("");
    setTestResult(null);
    setOpen(true);
  };

  const openEdit = (k: ByokKey) => {
    setEditing(k);
    setName(k.name);
    setProvider(k.provider || "anthropic");
    setApiKey("");
    setBaseUrl(k.base_url ?? "");
    setModel(k.model ?? "");
    setTestResult(null);
    setOpen(true);
  };

  const specOf = (p: string) => PROVIDERS.find((x) => x.value === p) ?? PROVIDERS[0];
  const spec = specOf(provider);

  const changeProvider = (p: string) => {
    setProvider(p);
    setTestResult(null);
    const next = specOf(p);
    if (!baseUrl.trim()) setBaseUrl(next.baseUrl);
    if (!model.trim()) setModel(next.model);
  };

  const save = async () => {
    if (!name.trim()) return;
    setBusy(true);
    try {
      const body = {
        name: name.trim(),
        provider,
        api_key: apiKey.trim(),
        base_url: baseUrl.trim(),
        model: model.trim(),
      };
      if (editing) {
        await api.updateByok(editing.id, body);
      } else {
        await api.createByok(body);
      }
      toast.success(t("byok.saved"));
      setOpen(false);
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const test = async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const res = await api.testByok({
        id: editing?.id,
        provider,
        api_key: apiKey.trim(),
        base_url: baseUrl.trim(),
        model: model.trim(),
      });
      setTestResult(res.ok ? { ok: true, msg: t("byok.testOk") } : { ok: false, msg: res.error || t("byok.testFail") });
    } catch (e) {
      setTestResult({ ok: false, msg: apiErrorMsg(to, e) });
    } finally {
      setTesting(false);
    }
  };

  const remove = async (k: ByokKey) => {
    try {
      await api.deleteByok(k.id);
      toast.success(t("byok.deleted"));
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between gap-3">
          <div className="min-w-0">
            <CardTitle className="flex items-center gap-2 text-base">
              <Bot className="h-4 w-4" />
              {t("byok.title")}
            </CardTitle>
            <CardDescription className="mt-1">{t("byok.hint")}</CardDescription>
          </div>
          <Button size="sm" variant="outline" onClick={openAdd}>
            <Plus className="h-4 w-4" />
            {t("byok.add")}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-2">
        {keys === null ? null : keys.length === 0 ? (
          <p className="py-2 text-sm text-muted-foreground">{t("byok.empty")}</p>
        ) : (
          keys.map((k) => (
            <div key={k.id} className="flex items-center justify-between rounded-lg border p-3">
              <div className="min-w-0">
                <p className="flex items-center gap-2 text-sm font-medium">
                  {k.name}
                  <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
                    {providerLabel(t, k.provider)}
                  </span>
                </p>
                <p className="mt-0.5 truncate text-xs text-muted-foreground">
                  {k.model || "claude"}
                  {k.key_set
                    ? ` · ${t("byok.keySet")}`
                    : ` · ${t("byok.keyMissing")}`}
                </p>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <Button size="icon" variant="ghost" onClick={() => openEdit(k)}>
                  <Pencil className="h-4 w-4" />
                </Button>
                <Button size="icon" variant="ghost" onClick={() => remove(k)}>
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            </div>
          ))
        )}
      </CardContent>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{editing ? t("byok.edit") : t("byok.add")}</DialogTitle>
            <DialogDescription>{t("byok.hint")}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="byok-name">{t("byok.name")}</Label>
              <Input id="byok-name" value={name} onChange={(e) => setName(e.target.value)} placeholder={t("byok.namePlaceholder")} />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="byok-provider">{t("byok.providerLabel")}</Label>
              <select
                id="byok-provider"
                className="rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                value={provider}
                onChange={(e) => changeProvider(e.target.value as ByokProvider)}
              >
                {PROVIDERS.map((p) => (
                  <option key={p.value} value={p.value}>
                    {providerLabel(t, p.value)}
                  </option>
                ))}
              </select>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="byok-key">
                {t("byok.apiKey")}
                {!spec.keyRequired && <span className="ml-1 text-xs text-muted-foreground">{t("byok.optional")}</span>}
              </Label>
              <Input id="byok-key" type="password" value={apiKey} onChange={(e) => setApiKey(e.target.value)} placeholder={editing ? t("byok.apiKeyKeep") : t("byok.apiKeyPlaceholder")} />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="byok-base">{t("byok.baseUrl")}</Label>
              <Input id="byok-base" value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} placeholder={spec.baseUrl || "https://your-gateway.example.com"} />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="byok-model">{t("byok.model")}</Label>
              <Input id="byok-model" value={model} onChange={(e) => setModel(e.target.value)} placeholder={spec.model || t("byok.modelPlaceholder")} />
            </div>
            {provider !== "anthropic" && (
              <p className="text-xs text-muted-foreground">{t("byok.compatibleHint")}</p>
            )}
            {testResult && (
              <p className={cn("text-xs", testResult.ok ? "text-green-600" : "text-destructive")}>
                {testResult.msg}
              </p>
            )}
          </div>
          <DialogFooter className="sm:justify-between">
            <Button variant="outline" onClick={test} disabled={testing || busy}>
              {testing ? <Loader2 className="h-4 w-4 animate-spin" /> : <PlugZap className="h-4 w-4" />}
              {t("byok.test")}
            </Button>
            <div className="flex items-center gap-2">
              <Button variant="ghost" onClick={() => setOpen(false)} disabled={busy}>
                {t("login.back")}
              </Button>
              <Button onClick={save} disabled={busy || !name.trim() || (!editing && spec.keyRequired && !apiKey.trim())}>
                {t("common.save")}
              </Button>
            </div>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}

