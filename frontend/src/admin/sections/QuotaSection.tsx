import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Plus, Trash2 } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { adminReq, toastError } from "../api";
import { EMPTY_QUOTA, QUOTA_FIELDS, type Quota, type QuotaOverride } from "../types";

export function QuotaFields({ value, onChange }: { value: Quota; onChange: (q: Quota) => void }) {
  const { t } = useI18n();
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
      {QUOTA_FIELDS.map((f) => (
        <div key={f.key} className="grid gap-1">
          <Label className="text-xs text-muted-foreground" htmlFor={`q-${f.key}`}>
            {t(`admin.${f.label}`)}
          </Label>
          <Input
            id={`q-${f.key}`}
            type="number"
            min={0}
            value={value[f.key]}
            onChange={(e) => onChange({ ...value, [f.key]: Math.max(0, Number(e.target.value) || 0) })}
          />
        </div>
      ))}
    </div>
  );
}

export function QuotaSection() {
  const { t, to } = useI18n();
  const [def, setDef] = useState<Quota>(EMPTY_QUOTA);
  const [overrides, setOverrides] = useState<QuotaOverride[]>([]);
  const [scope, setScope] = useState<"user" | "org">("user");
  const [name, setName] = useState("");
  const [draft, setDraft] = useState<Quota>(EMPTY_QUOTA);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const r = await adminReq<{ default: Quota; overrides: QuotaOverride[] }>("/quota");
      setDef({ ...EMPTY_QUOTA, ...r.default });
      setOverrides(r.overrides ?? []);
    } catch (e) {
      toastError(to, e);
    }
  }, [to]);

  useEffect(() => {
    void load();
  }, [load]);

  const saveDefault = async () => {
    setBusy(true);
    try {
      const r = await adminReq<Quota>("/quota", def);
      setDef({ ...EMPTY_QUOTA, ...r });
      toast.success(t("admin.saved"));
    } catch (e) {
      toastError(to, e);
    } finally {
      setBusy(false);
    }
  };

  const addOverride = async () => {
    const n = name.trim();
    if (!n) {
      toast.error(t("admin.quotaNameRequired"));
      return;
    }
    setBusy(true);
    try {
      await adminReq(`/quota/${scope}/${encodeURIComponent(n)}`, draft, "PUT");
      toast.success(t("admin.quotaAdded", { name: n }));
      setName("");
      setDraft(EMPTY_QUOTA);
      await load();
    } catch (e) {
      toastError(to, e);
    } finally {
      setBusy(false);
    }
  };

  const removeOverride = async (o: QuotaOverride) => {
    try {
      await adminReq(`/quota/${o.scope}/${encodeURIComponent(o.name)}`, undefined, "DELETE");
      toast.success(t("admin.quotaRemoved", { name: o.name }));
      setOverrides((prev) => prev.filter((x) => !(x.scope === o.scope && x.name === o.name)));
    } catch (e) {
      toastError(to, e);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("admin.quotaTitle")}</CardTitle>
        <CardDescription>{t("admin.quotaHint")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        <div className="space-y-3">
          <p className="text-sm font-medium">{t("admin.quotaDefault")}</p>
          <QuotaFields value={def} onChange={setDef} />
          <Button size="sm" onClick={saveDefault} disabled={busy}>
            {t("admin.save")}
          </Button>
        </div>

        <div className="space-y-3 border-t pt-4">
          <p className="text-sm font-medium">{t("admin.quotaOverrides")}</p>
          {overrides.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("admin.quotaEmpty")}</p>
          ) : (
            <div className="space-y-2">
              {overrides.map((o) => (
                <div
                  key={`${o.scope}:${o.name}`}
                  className="flex flex-wrap items-center gap-2 rounded-md border px-3 py-2 text-sm"
                >
                  <Badge variant="outline">
                    {o.scope === "user" ? t("admin.quotaUser") : t("admin.quotaOrg")}
                  </Badge>
                  <span className="font-medium">{o.name}</span>
                  <span className="text-xs text-muted-foreground">
                    {QUOTA_FIELDS.map((f) => `${t(`admin.${f.label}`)}=${o.quota[f.key]}`).join(" · ")}
                  </span>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="ml-auto h-7 gap-1 text-xs"
                    onClick={() => removeOverride(o)}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                    {t("admin.quotaRemove")}
                  </Button>
                </div>
              ))}
            </div>
          )}

          <div className="space-y-2 rounded-md border p-3">
            <div className="flex flex-wrap items-end gap-2">
              <div className="grid gap-1">
                <Label className="text-xs text-muted-foreground" htmlFor="q-scope">
                  {t("admin.quotaScope")}
                </Label>
                <select
                  id="q-scope"
                  className="h-9 rounded-md border border-input bg-background px-2 text-sm"
                  value={scope}
                  onChange={(e) => setScope(e.target.value as "user" | "org")}
                >
                  <option value="user">{t("admin.quotaUser")}</option>
                  <option value="org">{t("admin.quotaOrg")}</option>
                </select>
              </div>
              <div className="grid flex-1 gap-1">
                <Label className="text-xs text-muted-foreground" htmlFor="q-name">
                  {t("admin.quotaName")}
                </Label>
                <Input id="q-name" value={name} onChange={(e) => setName(e.target.value)} />
              </div>
            </div>
            <QuotaFields value={draft} onChange={setDraft} />
            <Button size="sm" variant="outline" className="gap-1" onClick={addOverride} disabled={busy}>
              <Plus className="h-3.5 w-3.5" />
              {t("admin.quotaAdd")}
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

