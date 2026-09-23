import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Check, GitBranch, Pencil, Plus, Tag, Trash2 } from "lucide-react";
import { api, type Branch as BranchType, type Tag as TagType } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import Pagination from "@/components/ui/pagination";
import { cn } from "@/lib/utils";

interface Props {
  open: boolean;
  onClose: () => void;
  owner: string;
  repo: string;
  /** 创建分支/标签时的起点引用，默认 HEAD。 */
  current: string;
  /** 是否拥有写权限（只读用户隐藏创建 / 删除 / 备注操作）。 */
  canWrite: boolean;
  onRefresh: () => void;
}

/** 分支与标签管理：独立 tab 分页展示，并支持为每个引用维护备注。 */
export default function RefsDialog({
  open,
  onClose,
  owner,
  repo,
  current,
  canWrite,
  onRefresh,
}: Props) {
  const { t, to } = useI18n();

  // 分支 tab
  const [branches, setBranches] = useState<BranchType[]>([]);
  const [branchTotal, setBranchTotal] = useState(0);
  const [branchPage, setBranchPage] = useState(1);
  const [branchLoading, setBranchLoading] = useState(false);
  const [branchReload, setBranchReload] = useState(0);

  // 标签 tab
  const [tags, setTags] = useState<TagType[]>([]);
  const [tagTotal, setTagTotal] = useState(0);
  const [tagPage, setTagPage] = useState(1);
  const [tagLoading, setTagLoading] = useState(false);
  const [tagReload, setTagReload] = useState(0);

  const [pageSize, setPageSize] = useState(10);
  const [branchName, setBranchName] = useState("");
  const [tagName, setTagName] = useState("");
  const [busy, setBusy] = useState(false);

  // 备注编辑：editing 为 "kind/name"
  const [editing, setEditing] = useState<string | null>(null);
  const [noteDraft, setNoteDraft] = useState("");

  const loadBranches = useCallback(async () => {
    setBranchLoading(true);
    try {
      const data = await api.branchesPage(owner, repo, pageSize, (branchPage - 1) * pageSize);
      const items = data.items ?? [];
      // 删除后当前页可能变空：回退一页重新加载。
      if (items.length === 0 && branchPage > 1) {
        setBranchPage(branchPage - 1);
        return;
      }
      setBranches(items);
      setBranchTotal(data.total ?? 0);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBranchLoading(false);
    }
  }, [owner, repo, pageSize, branchPage, to]);

  const loadTags = useCallback(async () => {
    setTagLoading(true);
    try {
      const data = await api.tagsPage(owner, repo, pageSize, (tagPage - 1) * pageSize);
      const items = data.items ?? [];
      if (items.length === 0 && tagPage > 1) {
        setTagPage(tagPage - 1);
        return;
      }
      setTags(items);
      setTagTotal(data.total ?? 0);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setTagLoading(false);
    }
  }, [owner, repo, pageSize, tagPage, to]);

  // 关闭时重置状态：下次打开直接从第 1 页加载，避免残留页码导致的多余请求。
  useEffect(() => {
    if (open) return;
    setBranchPage(1);
    setTagPage(1);
    setEditing(null);
    setBranchName("");
    setTagName("");
  }, [open]);

  useEffect(() => {
    if (open) void loadBranches();
  }, [open, loadBranches, branchReload]);

  useEffect(() => {
    if (open) void loadTags();
  }, [open, loadTags, tagReload]);

  const reloadBranches = () => setBranchReload((n) => n + 1);
  const reloadTags = () => setTagReload((n) => n + 1);

  const create = async (kind: "branch" | "tag") => {
    const name = kind === "branch" ? branchName.trim() : tagName.trim();
    if (!name) return;
    setBusy(true);
    try {
      await api.createRef(owner, repo, kind, name, current || "HEAD");
      toast.success(t("refs.created", { type: kind, name }));
      if (kind === "branch") {
        setBranchName("");
        setBranchPage(1);
        reloadBranches();
      } else {
        setTagName("");
        setTagPage(1);
        reloadTags();
      }
      onRefresh();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (kind: "branch" | "tag", name: string) => {
    setBusy(true);
    try {
      await api.deleteRef(owner, repo, kind, name);
      toast.success(t("refs.deleted", { type: kind, name }));
      if (kind === "branch") reloadBranches();
      else reloadTags();
      onRefresh();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const saveNote = async (kind: "branch" | "tag", name: string, value: string) => {
    try {
      await api.setRefNote(owner, repo, kind, name, value);
      toast.success(t("refs.noteSaved", { name }));
      setEditing(null);
      if (kind === "branch") reloadBranches();
      else reloadTags();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  // 引用下方的备注区域：编辑态为文本域 + 保存/取消/清除，否则展示已保存备注。
  const renderNote = (kind: "branch" | "tag", name: string, note?: string) => {
    const key = `${kind}/${name}`;
    if (editing === key) {
      return (
        <div className="mt-2 space-y-1.5">
          <Textarea
            value={noteDraft}
            onChange={(e) => setNoteDraft(e.target.value)}
            placeholder={t("refs.notePlaceholder", {
              type: t(kind === "branch" ? "refs.branch" : "refs.tag"),
            })}
            rows={2}
            maxLength={10000}
            className="min-h-[56px] text-sm"
            autoFocus
          />
          <div className="flex flex-wrap items-center gap-1.5">
            <Button size="sm" className="h-7 gap-1" onClick={() => saveNote(kind, name, noteDraft.trim())}>
              <Check className="h-3.5 w-3.5" />
              {t("refs.saveNote")}
            </Button>
            <Button size="sm" variant="ghost" className="h-7" onClick={() => setEditing(null)}>
              {t("common.cancel")}
            </Button>
            {note ? (
              <Button
                size="sm"
                variant="ghost"
                className="h-7 text-destructive hover:text-destructive"
                onClick={() => saveNote(kind, name, "")}
              >
                {t("refs.clearNote")}
              </Button>
            ) : null}
          </div>
        </div>
      );
    }
    if (!note) return null;
    return (
      <p className="mt-1 whitespace-pre-wrap break-words pl-6 text-xs text-muted-foreground">{note}</p>
    );
  };

  const startEditNote = (kind: "branch" | "tag", name: string, note?: string) => {
    setEditing(`${kind}/${name}`);
    setNoteDraft(note ?? "");
  };

  const noteButton = (kind: "branch" | "tag", name: string, note?: string) => {
    if (!canWrite) return null;
    return (
      <Button
        variant="ghost"
        size="icon"
        className="h-7 w-7 shrink-0 text-muted-foreground"
        title={t("refs.editNote")}
        onClick={() => startEditNote(kind, name, note)}
      >
        <Pencil className="h-3.5 w-3.5" />
      </Button>
    );
  };

  const fromRow = (
    label: string,
    placeholder: string,
    value: string,
    setValue: (s: string) => void,
    act: () => void,
  ) => (
    <div className="grid gap-2">
      <Label>{label}</Label>
      <div className="flex flex-col gap-2 sm:flex-row">
        <Input
          placeholder={placeholder}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && act()}
          className="font-mono"
        />
        <Button onClick={act} disabled={busy || !value.trim()} className="gap-1.5">
          <Plus className="h-4 w-4" />
          {t("refs.create")}
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">
        {t("refs.from")}: {current || "HEAD"}
      </p>
    </div>
  );

  return (
    <Dialog open={open} onOpenChange={onClose}>
      <DialogContent className="max-h-[calc(100vh-2rem)] max-w-[calc(100vw-2rem)] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            {owner}/{repo} · {t("refs.manage")}
          </DialogTitle>
        </DialogHeader>

        <Tabs defaultValue="branch" className="space-y-4">
          <TabsList className="w-full justify-start">
            <TabsTrigger value="branch" className="gap-1.5">
              <GitBranch className="h-3.5 w-3.5" />
              {t("refs.branches")}
              <Badge variant="secondary" className="h-5 px-1.5 text-[10px]">
                {branchTotal}
              </Badge>
            </TabsTrigger>
            <TabsTrigger value="tag" className="gap-1.5">
              <Tag className="h-3.5 w-3.5" />
              {t("refs.tags")}
              <Badge variant="secondary" className="h-5 px-1.5 text-[10px]">
                {tagTotal}
              </Badge>
            </TabsTrigger>
          </TabsList>

          <TabsContent value="branch" className="space-y-3">
            {branchLoading && branches.length === 0 ? (
              <p className="py-6 text-center text-sm text-muted-foreground">…</p>
            ) : branches.length === 0 ? (
              <p className="rounded-lg border border-dashed py-6 text-center text-sm text-muted-foreground">
                {t("refs.noBranches")}
              </p>
            ) : (
              <div className="divide-y divide-border rounded-lg border">
                {branches.map((b) => (
                  <div key={b.name} className="px-3 py-2">
                    <div className="flex items-center gap-2">
                      <GitBranch className="h-4 w-4 shrink-0 text-muted-foreground" />
                      <code
                        className={cn(
                          "min-w-0 flex-1 truncate text-sm",
                          b.name === current && "font-medium",
                        )}
                      >
                        {b.name}
                      </code>
                      {b.is_head && (
                        <Badge variant="secondary" className="shrink-0">
                          HEAD
                        </Badge>
                      )}
                      {noteButton("branch", b.name, b.note)}
                      {canWrite && (
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-7 w-7 shrink-0 text-destructive hover:text-destructive"
                          disabled={busy || b.is_head}
                          title={t("refs.deleteBranch")}
                          onClick={() => remove("branch", b.name)}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      )}
                    </div>
                    {renderNote("branch", b.name, b.note)}
                  </div>
                ))}
              </div>
            )}
            {branchTotal > 0 && (
              <Pagination
                page={branchPage}
                pageSize={pageSize}
                total={branchTotal}
                onPageChange={setBranchPage}
                onPageSizeChange={(s) => {
                  setPageSize(s);
                  setBranchPage(1);
                  setTagPage(1);
                }}
              />
            )}
            {canWrite && fromRow(t("refs.branchName"), "feature/xxx", branchName, setBranchName, () => create("branch"))}
          </TabsContent>

          <TabsContent value="tag" className="space-y-3">
            {tagLoading && tags.length === 0 ? (
              <p className="py-6 text-center text-sm text-muted-foreground">…</p>
            ) : tags.length === 0 ? (
              <p className="rounded-lg border border-dashed py-6 text-center text-sm text-muted-foreground">
                {t("refs.noTags")}
              </p>
            ) : (
              <div className="divide-y divide-border rounded-lg border">
                {tags.map((tg) => (
                  <div key={tg.name} className="px-3 py-2">
                    <div className="flex items-center gap-2">
                      <Tag className="h-4 w-4 shrink-0 text-muted-foreground" />
                      <code className="min-w-0 flex-1 truncate text-sm">{tg.name}</code>
                      <span className="shrink-0 font-mono text-xs text-muted-foreground">
                        {tg.sha.slice(0, 7)}
                      </span>
                      {noteButton("tag", tg.name, tg.note)}
                      {canWrite && (
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-7 w-7 shrink-0 text-destructive hover:text-destructive"
                          disabled={busy}
                          title={t("refs.deleteTag")}
                          onClick={() => remove("tag", tg.name)}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      )}
                    </div>
                    {renderNote("tag", tg.name, tg.note)}
                  </div>
                ))}
              </div>
            )}
            {tagTotal > 0 && (
              <Pagination
                page={tagPage}
                pageSize={pageSize}
                total={tagTotal}
                onPageChange={setTagPage}
                onPageSizeChange={(s) => {
                  setPageSize(s);
                  setBranchPage(1);
                  setTagPage(1);
                }}
              />
            )}
            {canWrite && fromRow(t("refs.tagName"), "v1.0.0", tagName, setTagName, () => create("tag"))}
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}
