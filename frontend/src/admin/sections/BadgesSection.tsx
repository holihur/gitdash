import { useCallback, useEffect, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import type { Badge, BadgeGrant } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { BadgeChip } from "@/components/badge-chip";
import { adminList, adminReq, adminUpload, toastError } from "../api";

const EMPTY = { label: "", slug: "", description: "" };

/** 管理端徽章：定义（含图标）+ 授予 / 撤销。 */
export function BadgesSection() {
  const { t, to } = useI18n();
  const [badges, setBadges] = useState<Badge[]>([]);
  const [form, setForm] = useState(EMPTY);
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [grants, setGrants] = useState<Record<number, BadgeGrant[] | undefined>>({});
  const [kind, setKind] = useState<"user" | "repo" | "org">("user");
  const [grantOwner, setGrantOwner] = useState("");
  const [grantRepo, setGrantRepo] = useState("");

  const load = useCallback(async () => {
    try {
      setBadges((await adminList<Badge[]>("/badges")).items);
    } catch (e) {
      toastError(to, e);
    }
  }, [to]);
  useEffect(() => {
    void load();
  }, [load]);

  const create = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!form.label.trim()) return;
    setBusy(true);
    try {
      const fd = new FormData();
      fd.set("label", form.label.trim());
      if (form.slug.trim()) fd.set("slug", form.slug.trim());
      fd.set("description", form.description);
      if (file) fd.set("image", file);
      await adminUpload<Badge>("/badges", fd);
      setForm(EMPTY);
      setFile(null);
      await load();
    } catch (err) {
      toastError(to, err);
    } finally {
      setBusy(false);
    }
  };

  const remove = async (id: number) => {
    try {
      await adminReq(`/badges/${id}`, undefined, "DELETE");
      setGrants((s) => ({ ...s, [id]: undefined }));
      await load();
    } catch (e) {
      toastError(to, e);
    }
  };

  const loadGrants = async (id: number) => {
    try {
      const g = await adminReq<BadgeGrant[]>(`/badges/${id}/grants`);
      setGrants((s) => ({ ...s, [id]: g }));
    } catch (e) {
      toastError(to, e);
    }
  };

  const toggleGrants = (id: number) => {
    if (grants[id] === undefined) void loadGrants(id);
    else setGrants((s) => ({ ...s, [id]: undefined }));
  };

  const grant = async (id: number) => {
    if (!grantOwner.trim()) return;
    try {
      await adminReq(`/badges/${id}/grants`, {
        kind,
        owner: grantOwner.trim(),
        repo: kind === "repo" ? grantRepo.trim() : "",
      });
      setGrantOwner("");
      setGrantRepo("");
      await loadGrants(id);
    } catch (e) {
      toastError(to, e);
    }
  };

  const revoke = async (id: number, g: BadgeGrant) => {
    const q = new URLSearchParams({ kind: g.kind, owner: g.owner, repo: g.repo });
    try {
      await adminReq(`/badges/${id}/grants?${q.toString()}`, undefined, "DELETE");
      await loadGrants(id);
    } catch (e) {
      toastError(to, e);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("admin.badgesTitle")}</CardTitle>
      </CardHeader>
      <CardContent className="grid gap-4">
        <form onSubmit={create} className="grid gap-2 sm:grid-cols-2">
          <Input
            placeholder={t("admin.badgeLabel")}
            value={form.label}
            maxLength={64}
            onChange={(e) => setForm({ ...form, label: e.target.value })}
          />
          <Input
            placeholder={t("admin.badgeSlug")}
            value={form.slug}
            maxLength={64}
            onChange={(e) => setForm({ ...form, slug: e.target.value })}
          />
          <Textarea
            className="sm:col-span-2"
            rows={2}
            maxLength={500}
            placeholder={t("admin.badgeDescription")}
            value={form.description}
            onChange={(e) => setForm({ ...form, description: e.target.value })}
          />
          <input
            type="file"
            accept="image/png,image/jpeg,image/gif,image/webp"
            onChange={(e) => setFile(e.target.files?.[0] ?? null)}
            className="text-sm text-muted-foreground"
          />
          <div className="flex items-center justify-end">
            <Button type="submit" className="gap-1.5" disabled={busy || !form.label.trim()}>
              <Plus className="h-4 w-4" />
              {t("admin.badgeCreate")}
            </Button>
          </div>
        </form>

        {badges.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("admin.badgesEmpty")}</p>
        ) : (
          <div className="grid gap-3">
            {badges.map((b) => (
              <div key={b.id} className="rounded-md border p-3">
                <div className="flex items-center gap-2">
                  <BadgeChip badge={b} />
                  <span className="font-mono text-xs text-muted-foreground">{b.slug}</span>
                  <div className="ml-auto flex items-center gap-1">
                    <Button size="sm" variant="outline" onClick={() => toggleGrants(b.id)}>
                      {t("admin.badgeGrants")}
                    </Button>
                    <Button
                      size="icon"
                      variant="ghost"
                      className="h-8 w-8 text-destructive hover:text-destructive"
                      title={t("common.delete")}
                      onClick={() => void remove(b.id)}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
                {b.description && (
                  <p className="mt-1 text-xs text-muted-foreground">{b.description}</p>
                )}

                {grants[b.id] !== undefined && (
                  <div className="mt-2 space-y-2 border-t pt-2">
                    <div className="flex flex-wrap items-center gap-2">
                      <select
                        value={kind}
                        onChange={(e) => setKind(e.target.value as "user" | "repo" | "org")}
                        className="h-9 rounded-md border border-input bg-background px-2 text-sm"
                        aria-label={t("admin.grantKind")}
                      >
                        <option value="user">user</option>
                        <option value="repo">repo</option>
                        <option value="org">org</option>
                      </select>
                      <Input
                        className="h-9 w-40"
                        placeholder={t("admin.grantOwner")}
                        value={grantOwner}
                        onChange={(e) => setGrantOwner(e.target.value)}
                      />
                      {kind === "repo" && (
                        <Input
                          className="h-9 w-40"
                          placeholder={t("admin.grantRepo")}
                          value={grantRepo}
                          onChange={(e) => setGrantRepo(e.target.value)}
                        />
                      )}
                      <Button
                        size="sm"
                        onClick={() => void grant(b.id)}
                        disabled={!grantOwner.trim() || (kind === "repo" && !grantRepo.trim())}
                      >
                        {t("admin.grant")}
                      </Button>
                    </div>
                    {(grants[b.id] ?? []).length === 0 ? (
                      <p className="text-xs text-muted-foreground">{t("admin.noGrants")}</p>
                    ) : (
                      <div className="flex flex-wrap gap-1.5">
                        {(grants[b.id] ?? []).map((g) => (
                          <span
                            key={`${g.kind}:${g.owner}:${g.repo}`}
                            className="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs"
                          >
                            <span className="text-muted-foreground">{g.kind}:</span>
                            {g.owner}
                            {g.repo ? `/${g.repo}` : ""}
                            <button
                              type="button"
                              className="text-destructive"
                              title={t("admin.revoke")}
                              onClick={() => void revoke(b.id, g)}
                            >
                              ×
                            </button>
                          </span>
                        ))}
                      </div>
                    )}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
