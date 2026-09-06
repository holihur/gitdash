import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { FileDown, Package, Plus, Tag as TagIcon, Trash2, Upload } from "lucide-react";
import { api, type Release, type Tag } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import ConfirmDialog from "@/components/confirm-dialog";
import { MarkdownWithToc } from "@/components/markdown";
import { cn, formatDate, formatSize } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

export interface ReleasesTabProps {
  owner: string;
  name: string;
  role?: "owner" | "read" | "write";
}

export default function ReleasesTab({ owner, name, role }: ReleasesTabProps) {
  const { t, lang, to } = useI18n();
  const locale = lang === "zh-CN" ? "zh-CN" : "en-US";
  const canWrite = role === "owner" || role === "write";
  const [releases, setReleases] = useState<Release[]>([]);
  const [tags, setTags] = useState<Tag[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  // create dialog
  const [open, setOpen] = useState(false);
  const [tagMode, setTagMode] = useState<"select" | "input">("select");
  const [tagName, setTagName] = useState("");
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [busy, setBusy] = useState(false);

  const [pendingDelete, setPendingDelete] = useState<string | null>(null);
  const fileInputs = useRef<Record<string, HTMLInputElement | null>>({});

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [rs, ts] = await Promise.all([
        api.listReleases(owner, name),
        api.listTags(owner, name).catch(() => [] as Tag[]),
      ]);
      setReleases(rs);
      setTags(ts);
      setError("");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [owner, name]);

  useEffect(() => {
    load();
  }, [load]);

  const create = async () => {
    if (!tagName.trim()) return;
    setBusy(true);
    try {
      await api.createRelease(owner, name, tagName.trim(), title.trim() || undefined, body.trim() || undefined);
      toast.success(t("releases.created", { tag: tagName.trim() }));
      setOpen(false);
      setTagName("");
      setTitle("");
      setBody("");
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (tag: string) => {
    try {
      await api.deleteRelease(owner, name, tag);
      toast.success(t("releases.deleted", { tag }));
      setPendingDelete(null);
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const upload = async (tag: string, file: File) => {
    try {
      await api.uploadReleaseAsset(owner, name, tag, file);
      toast.success(t("releases.assetUploaded", { name: file.name }));
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const removeAsset = async (tag: string, filename: string) => {
    try {
      await api.deleteReleaseAsset(owner, name, tag, filename);
      toast.success(t("releases.assetDeleted", { name: filename }));
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const usedTags = new Set(releases.map((r) => r.tag_name));

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">{t("releases.count", { count: releases.length })}</p>
        {canWrite && (
          <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger asChild>
              <Button size="sm" className="gap-2">
                <Plus className="h-4 w-4" />
                {t("releases.new")}
              </Button>
            </DialogTrigger>
            <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-lg">
              <DialogHeader>
                <DialogTitle>{t("releases.newTitle")}</DialogTitle>
              </DialogHeader>
              <div className="grid gap-4">
                <div className="grid gap-2">
                  <Label htmlFor="release-tag">{t("releases.tagLabel")}</Label>
                  {tags.filter((tg) => !usedTags.has(tg.name)).length > 0 && tagMode === "select" ? (
                    <div className="flex gap-2">
                      <select
                        id="release-tag"
                        value={tagName}
                        onChange={(e) => setTagName(e.target.value)}
                        className="h-10 flex-1 rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                      >
                        <option value="">-</option>
                        {tags
                          .filter((tg) => !usedTags.has(tg.name))
                          .map((tg) => (
                            <option key={tg.name} value={tg.name}>
                              {tg.name}
                            </option>
                          ))}
                      </select>
                      <Button type="button" variant="outline" size="sm" onClick={() => setTagMode("input")}>
                        {t("releases.tagManual")}
                      </Button>
                    </div>
                  ) : (
                    <div className="flex gap-2">
                      <Input
                        id="release-tag"
                        placeholder="v1.0.0"
                        value={tagName}
                        onChange={(e) => setTagName(e.target.value)}
                      />
                      {tags.length > 0 && (
                        <Button type="button" variant="outline" size="sm" onClick={() => setTagMode("select")}>
                          {t("releases.tagFromList")}
                        </Button>
                      )}
                    </div>
                  )}
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="release-title">{t("releases.nameLabel")}</Label>
                  <Input
                    id="release-title"
                    placeholder={t("releases.namePlaceholder")}
                    maxLength={200}
                    value={title}
                    onChange={(e) => setTitle(e.target.value)}
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="release-body">{t("releases.bodyLabel")}</Label>
                  <Textarea
                    id="release-body"
                    rows={5}
                    value={body}
                    onChange={(e) => setBody(e.target.value)}
                  />
                </div>
              </div>
              <DialogFooter>
                <Button onClick={create} disabled={busy || !tagName.trim()}>
                  {t("common.create")}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        )}
      </div>

      {error && (
        <Card className="border-destructive">
          <CardContent className="pt-6 text-sm text-destructive">{error}</CardContent>
        </Card>
      )}

      {loading && <p className="py-10 text-center text-sm text-muted-foreground">…</p>}

      {!loading && !error && releases.length === 0 && (
        <Card>
          <CardContent className="flex flex-col items-center gap-2 py-12 text-center">
            <Package className="h-10 w-10 text-muted-foreground" />
            <p className="font-medium">{t("releases.empty")}</p>
            <p className="text-sm text-muted-foreground">{t("releases.emptyHint")}</p>
          </CardContent>
        </Card>
      )}

      {!loading && releases.length > 0 && (
        <div className="space-y-3">
          {releases.map((r) => (
            <Card key={r.tag_name}>
              <CardContent className="space-y-3 pt-6">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant="outline" className="gap-1 font-mono">
                    <TagIcon className="h-3 w-3" />
                    {r.tag_name}
                  </Badge>
                  <span className={cn("font-medium", !r.name && "text-muted-foreground")}>
                    {r.name || r.tag_name}
                  </span>
                  <span className="text-xs text-muted-foreground">{formatDate(r.created_at, locale)}</span>
                  {canWrite && (
                    <Button
                      size="icon"
                      variant="ghost"
                      className="ml-auto h-7 w-7 text-muted-foreground hover:text-destructive"
                      title={t("releases.delete")}
                      onClick={() => setPendingDelete(r.tag_name)}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  )}
                </div>
                {r.body && (
                  <div className="rounded-md border bg-muted/20 p-3">
                    <MarkdownWithToc text={r.body} />
                  </div>
                )}
                <div className="space-y-1.5">
                  <p className="text-xs font-medium text-muted-foreground">{t("releases.assets")}</p>
                  {r.assets.length === 0 && (
                    <p className="text-xs text-muted-foreground">{t("releases.noAssets")}</p>
                  )}
                  {r.assets.map((a) => (
                    <div key={a.filename} className="flex items-center gap-2 text-sm">
                      <FileDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                      <a
                        className="min-w-0 truncate font-mono text-xs hover:underline"
                        href={api.releaseAssetUrl(owner, name, r.tag_name, a.filename)}
                        download
                        title={a.filename}
                      >
                        {a.filename}
                      </a>
                      <span className="shrink-0 text-xs text-muted-foreground">{formatSize(a.size)}</span>
                      {canWrite && (
                        <Button
                          size="icon"
                          variant="ghost"
                          className="ml-auto h-6 w-6 text-muted-foreground hover:text-destructive"
                          title={t("releases.assetDelete")}
                          onClick={() => removeAsset(r.tag_name, a.filename)}
                        >
                          <Trash2 className="h-3 w-3" />
                        </Button>
                      )}
                    </div>
                  ))}
                  {canWrite && (
                    <>
                      <input
                        ref={(el) => {
                          fileInputs.current[r.tag_name] = el;
                        }}
                        type="file"
                        className="hidden"
                        onChange={(e) => {
                          const f = e.target.files?.[0];
                          if (f) upload(r.tag_name, f);
                          e.target.value = "";
                        }}
                      />
                      <Button
                        size="sm"
                        variant="outline"
                        className="gap-1.5"
                        onClick={() => fileInputs.current[r.tag_name]?.click()}
                      >
                        <Upload className="h-3.5 w-3.5" />
                        {t("releases.uploadAsset")}
                      </Button>
                    </>
                  )}
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(o) => !o && setPendingDelete(null)}
        description={t("releases.confirmDelete", { tag: pendingDelete ?? "" })}
        onConfirm={() => pendingDelete && remove(pendingDelete)}
      />
    </div>
  );
}
