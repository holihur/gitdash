import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { api, type Badge } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { BadgeChip } from "@/components/badge-chip";

/**
 * 目标（用户 / 仓库 / 组织）选择挂出哪些徽章（最多 max 个）。
 * 只列出系统已授予的徽章；没有任何授予时不渲染。
 */
export function BadgeDisplayPicker({
  kind,
  owner,
  repo,
}: {
  kind: "user" | "repo" | "org";
  owner: string;
  repo?: string;
}) {
  const { t, to } = useI18n();
  const [granted, setGranted] = useState<Badge[]>([]);
  const [selected, setSelected] = useState<number[]>([]);
  const [max, setMax] = useState(3);
  const [loaded, setLoaded] = useState(false);
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    setLoaded(false);
    try {
      const r = await api.getOwned(kind, owner, repo);
      setGranted(r.granted ?? []);
      setSelected(r.displayed ?? []);
      setMax(r.max || 3);
    } catch {
      setGranted([]);
    } finally {
      setLoaded(true);
    }
  }, [kind, owner, repo]);
  useEffect(() => {
    void load();
  }, [load]);

  if (!loaded || granted.length === 0) return null;

  const toggle = (id: number) =>
    setSelected((cur) =>
      cur.includes(id) ? cur.filter((x) => x !== id) : cur.length < max ? [...cur, id] : cur,
    );

  const save = async () => {
    setSaving(true);
    try {
      await api.setDisplay(kind, owner, repo, selected);
      toast.success(t("badges.saved"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("badges.manageTitle")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">{t("badges.manageHint", { max })}</p>
        <div className="flex flex-wrap gap-1.5">
          {granted.map((b) => {
            const on = selected.includes(b.id);
            return (
              <button
                key={b.id}
                type="button"
                onClick={() => toggle(b.id)}
                title={b.description || b.label}
                className={cn(
                  "rounded-full outline-none transition-opacity focus-visible:ring-2 focus-visible:ring-ring",
                  on ? "ring-2 ring-ring ring-offset-1" : "opacity-50 hover:opacity-80",
                )}
              >
                <BadgeChip badge={b} />
              </button>
            );
          })}
        </div>
        <div className="flex justify-end">
          <Button size="sm" onClick={() => void save()} disabled={saving}>
            {t("common.save")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
