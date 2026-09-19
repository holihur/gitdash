import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { ChevronLeft, FilePlus2, FolderPlus, Pencil } from "lucide-react";
import { toast } from "sonner";
import { api, type Branch } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
import { buildRepoPath, type FileOpMode } from "@/lib/repo-url";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import CodeMirrorEditor from "@/components/code-editor-lazy";

export type FileOpKind = "create-file" | "create-dir" | "edit";

/** 新建 / 编辑文件的整页表单（替代原弹框，URL 可分享、可刷新、可回退）。 */
export default function FileOpPage({ mode }: { mode: FileOpMode }) {
  const { t, to } = useI18n();
  const navigate = useNavigate();
  const { owner = "", name = "" } = useParams();
  const [searchParams] = useSearchParams();

  const editPath = mode === "edit" ? (searchParams.get("path") ?? "") : "";
  const dir = searchParams.get("dir") ?? "";
  const urlRef = searchParams.get("ref") ?? "";
  const kind: FileOpKind =
    mode === "edit" ? "edit" : searchParams.get("kind") === "dir" ? "create-dir" : "create-file";
  const isDir = kind === "create-dir";

  const [branches, setBranches] = useState<Branch[]>([]);
  const [branch, setBranch] = useState(urlRef);
  // 新建时路径可编辑；编辑时直接跟随 URL 里的 path，避免组件复用导致的陈旧值。
  const [newPath, setNewPath] = useState(dir ? `${dir.replace(/\/+$/, "")}/` : "");
  const path = kind === "edit" ? editPath : newPath;
  const [content, setContent] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(mode === "edit");
  const [loadError, setLoadError] = useState("");

  // 分支列表：未显式指定 ref 时回退到默认（head）分支。
  useEffect(() => {
    let alive = true;
    api
      .branches(owner, name)
      .then((bs) => {
        if (!alive) return;
        setBranches(bs);
        setBranch((prev) => prev || bs.find((b) => b.is_head)?.name || bs[0]?.name || "main");
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, [owner, name]);

  // 编辑模式：按 branch + path 拉取文件内容（刷新页面也能恢复）。
  useEffect(() => {
    if (mode !== "edit") return;
    if (!editPath) {
      setLoading(false);
      setLoadError(t("fops.pathRequired"));
      return;
    }
    if (!branch) return;
    let alive = true;
    setLoading(true);
    api
      .blob(owner, name, branch, editPath)
      .then((b) => {
        if (!alive) return;
        if (b.encoding !== "utf-8") {
          setLoadError(t("fops.notEditable"));
          setContent("");
        } else {
          setContent(b.content);
          setLoadError("");
        }
      })
      .catch((e) => {
        if (alive) setLoadError(apiErrorMsg(to, e));
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [mode, editPath, branch, owner, name, t, to]);

  const submitPath = isDir ? `${path.replace(/\/+$/, "")}/.gitkeep` : path.trim();
  const action: "create" | "update" = kind === "edit" ? "update" : "create";

  // ref 可能是 tag（不在 branches 里），保留为可选项以免受控值与显示不一致。
  const branchOptions = useMemo(() => {
    const names = branches.map((b) => b.name);
    return branch && !names.includes(branch) ? [branch, ...names] : names;
  }, [branch, branches]);

  const codeUrl = useCallback(
    (targetPath: string) => {
      const p = buildRepoPath(owner, name, { tab: "code", kind: "tree", path: targetPath });
      return branch ? `${p}?ref=${encodeURIComponent(branch)}` : p;
    },
    [owner, name, branch],
  );

  // 取消：编辑回到文件，新建回到当前目录。
  const cancelUrl = useMemo(() => {
    if (mode === "edit" && editPath) {
      const p = buildRepoPath(owner, name, { tab: "code", kind: "blob", path: editPath });
      return branch ? `${p}?ref=${encodeURIComponent(branch)}` : p;
    }
    return codeUrl(dir);
  }, [mode, editPath, owner, name, branch, codeUrl, dir]);

  const submit = async () => {
    if (!submitPath) {
      toast.error(t("fops.pathRequired"));
      return;
    }
    if (!branch) return;
    setBusy(true);
    const msg =
      message.trim() ||
      (kind === "create-dir"
        ? t("fops.msgCreateDir", { path: path.trim().replace(/\/+$/, "") })
        : kind === "edit"
          ? t("fops.msgUpdate", { path: submitPath })
          : t("fops.msgCreate", { path: submitPath }));
    try {
      await api.createCommit(owner, name, branch, msg, [
        // 新建文件夹通过提交 <dir>/.gitkeep 占位实现，其内容无意义，固定为空。
        { path: submitPath, action, content: isDir ? "" : content },
      ]);
      toast.success(t("fops.saved", { path: submitPath }));
      // 提交后回到父目录（避免把文件名当目录加载）
      const i = submitPath.lastIndexOf("/");
      navigate(codeUrl(i > 0 ? submitPath.slice(0, i) : ""));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const title =
    kind === "create-dir" ? t("fops.newFolder") : kind === "edit" ? t("fops.editFile") : t("fops.newFile");
  const Icon = isDir ? FolderPlus : kind === "edit" ? Pencil : FilePlus2;

  return (
    <div className="mx-auto max-w-4xl space-y-4">
      <Button asChild variant="ghost" size="sm" className="-ml-2 gap-1.5">
        <Link to={cancelUrl}>
          <ChevronLeft className="h-4 w-4" />
          {t("login.back")}
        </Link>
      </Button>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-lg">
            <Icon className="h-5 w-5" />
            {title}
          </CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="fop-branch">{t("fops.branchLabel")}</Label>
            <select
              id="fop-branch"
              value={branch}
              onChange={(e) => setBranch(e.target.value)}
              className="h-10 rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring sm:max-w-56"
            >
              {branchOptions.length === 0 && <option value={branch || "main"}>{branch || "main"}</option>}
              {branchOptions.map((b) => (
                <option key={b} value={b}>
                  {b}
                </option>
              ))}
            </select>
          </div>

          <div className="grid gap-2">
            <Label htmlFor="fop-path">{t("fops.pathLabel")}</Label>
            <Input
              id="fop-path"
              readOnly={kind === "edit"}
              placeholder="src/main.py"
              autoCapitalize="none"
              autoCorrect="off"
              value={path}
              onChange={(e) => setNewPath(e.target.value)}
            />
            {isDir && <p className="text-xs text-muted-foreground">{t("fops.folderHint")}</p>}
          </div>

          {!isDir && (
            <div className="grid gap-2">
              <Label>{t("fops.contentLabel")}</Label>
              {loadError ? (
                <p className="rounded-md border border-destructive/50 bg-destructive/5 px-3 py-2 text-sm text-destructive">
                  {loadError}
                </p>
              ) : loading ? (
                <Skeleton className="min-h-64 w-full" />
              ) : (
                <div className="max-h-[60vh] overflow-auto rounded-md border bg-background">
                  <CodeMirrorEditor
                    value={content}
                    path={submitPath}
                    onDocChange={setContent}
                    className="min-h-64"
                  />
                </div>
              )}
            </div>
          )}

          <div className="grid gap-2">
            <Label htmlFor="fop-msg">{t("fops.messageLabel")}</Label>
            <Input id="fop-msg" value={message} onChange={(e) => setMessage(e.target.value)} />
          </div>

          <div className="flex justify-end gap-2">
            <Button variant="outline" asChild disabled={busy}>
              <Link to={cancelUrl}>{t("common.cancel")}</Link>
            </Button>
            <Button onClick={submit} disabled={busy || !submitPath || !!loadError}>
              {t("fops.commit")}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
