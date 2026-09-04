import { useEffect, useState } from "react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import CodeMirrorEditor from "@/components/code-editor";

export interface FileOp {
  kind: "create-file" | "create-dir" | "edit";
  path: string; // 完整路径（create 时可为空）
  content: string;
  branch: string;
}

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  owner: string;
  repo: string;
  branches: string[];
  init: FileOp;
  onSaved: (branch: string, path: string) => void;
}

export default function FileOpDialog({ open, onOpenChange, owner, repo, branches, init, onSaved }: Props) {
  const { t, to } = useI18n();
  const [path, setPath] = useState(init.path);
  const [content, setContent] = useState(init.content);
  const [message, setMessage] = useState("");
  const [branch, setBranch] = useState(init.branch || branches[0] || "main");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (open) {
      setPath(init.path);
      setContent(init.content);
      setBranch(init.branch || branches[0] || "main");
      setMessage("");
    }
  }, [open, init, branches]);

  const isDir = init.kind === "create-dir";
  const submitPath = isDir ? path.replace(/\/$/, "") + "/.gitkeep" : path.trim();
  const action: "create" | "update" = init.kind === "edit" ? "update" : "create";

  const submit = async () => {
    if (!submitPath) {
      toast.error(t("fops.pathRequired"));
      return;
    }
    setBusy(true);
    const msg =
      message.trim() ||
      (init.kind === "create-dir"
        ? t("fops.msgCreateDir", { path: path.trim() })
        : init.kind === "edit"
          ? t("fops.msgUpdate", { path: submitPath })
          : t("fops.msgCreate", { path: submitPath }));
    try {
      await api.createCommit(owner, repo, branch, msg, [
        { path: submitPath, action, content },
      ]);
      toast.success(t("fops.saved", { path: submitPath }));
      onOpenChange(false);
      onSaved(branch, submitPath);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const title =
    init.kind === "create-dir"
      ? t("fops.newFolder")
      : init.kind === "edit"
        ? t("fops.editFile")
        : t("fops.newFile");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] max-w-[calc(100vw-2rem)] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="fop-branch">{t("fops.branchLabel")}</Label>
            <select
              id="fop-branch"
              value={branch}
              onChange={(e) => setBranch(e.target.value)}
              className="h-10 rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring sm:max-w-56"
            >
              {branches.length === 0 && <option value="main">main</option>}
              {branches.map((b) => (
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
              readOnly={init.kind === "edit"}
              placeholder="src/main.py"
              autoCapitalize="none"
              autoCorrect="off"
              value={path}
              onChange={(e) => setPath(e.target.value)}
            />
            {isDir && <p className="text-xs text-muted-foreground">{t("fops.folderHint")}</p>}
          </div>
          <div className="grid gap-2">
            <Label>{t("fops.contentLabel")}</Label>
            <div className="max-h-[46vh] overflow-auto rounded-md border bg-background">
              <CodeMirrorEditor value={content} path={submitPath} onDocChange={setContent} className="min-h-64" />
            </div>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="fop-msg">{t("fops.messageLabel")}</Label>
            <Input id="fop-msg" value={message} onChange={(e) => setMessage(e.target.value)} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={busy}>
            {t("login.back")}
          </Button>
          <Button onClick={submit} disabled={busy || !submitPath}>
            {t("fops.commit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
