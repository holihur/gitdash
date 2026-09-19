import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ChevronLeft, Copy, FileText, Folder, Loader2, Lock, Unlock } from "lucide-react";
import { toast } from "sonner";
import { api, type PackageEntry, type PackageFileEntry } from "@/lib/api";
import { packageFilePath } from "@/lib/api/packages";
import { apiErrorMsg } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
import { copyText, formatSize } from "@/lib/utils";
import { packageSetupCommands, packageUseCommand } from "@/lib/package-command";
import { archiveKind, isPreviewableName } from "@/lib/package-files";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

function CommandBlock({
  text,
  onCopy,
  multiline,
}: {
  text: string;
  onCopy: () => void;
  multiline?: boolean;
}) {
  return (
    <div className="flex items-start gap-2 rounded-md border bg-muted/50 px-3 py-2">
      <code
        className={
          multiline
            ? "min-w-0 flex-1 whitespace-pre-wrap break-all font-mono text-xs"
            : "min-w-0 flex-1 overflow-x-auto whitespace-pre font-mono text-xs"
        }
      >
        {text}
      </code>
      <Button variant="ghost" size="icon" className="h-6 w-6 shrink-0" onClick={onCopy}>
        <Copy className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

/** 单个包详情：一键使用命令 + 制品列表 + 归档内容浏览 / 预览 / 下载。 */
export default function PackageDetail() {
  const { t, to } = useI18n();
  const { type = "", owner = "", "*": splat = "" } = useParams();
  const name = splat.replace(/^\/+/, "").replace(/\/+$/, "");

  const [files, setFiles] = useState<PackageEntry[] | null>(null);
  const [canManage, setCanManage] = useState(false);
  const [error, setError] = useState("");
  const [selected, setSelected] = useState<PackageEntry | null>(null);
  const [entries, setEntries] = useState<PackageFileEntry[] | null>(null);
  const [entriesLoading, setEntriesLoading] = useState(false);
  const [activeEntry, setActiveEntry] = useState("");
  const [preview, setPreview] = useState<string | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);

  useEffect(() => {
    if (!name) return;
    setFiles(null);
    api
      .listPackageFiles(type, owner, name)
      .then(setFiles)
      .catch((e) => setError(apiErrorMsg(to, e)));
  }, [type, owner, name, to]);

  const latest = files?.[0] ?? null;
  const visibility = latest?.visibility ?? "private";

  useEffect(() => {
    api
      .me()
      .then(async (m) => {
        if (m.username === owner) {
          setCanManage(true);
          return;
        }
        const orgs = await api.listOrgs().catch(() => []);
        setCanManage(orgs.some((o) => o.name === owner && o.role === "owner"));
      })
      .catch(() => undefined);
  }, [owner]);

  const setVisibility = async (next: string) => {
    if (!latest) return;
    try {
      await api.setPackageVisibility(type, owner, name, next);
      toast.success(t("packages.visibilityUpdated"));
      setFiles((prev) =>
        prev?.map((f) => ({ ...f, visibility: next, private: next === "private" })) ?? prev,
      );
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };
  const useCommand = useMemo(
    () => (latest ? packageUseCommand(latest) : ""),
    [latest],
  );
  const setupCommands = useMemo(
    () => (latest ? packageSetupCommands(latest) : []),
    [latest],
  );

  const copy = useCallback(
    (text: string) => {
      copyText(text)
        .then(() => toast.success(t("common.copied")))
        .catch(() => toast.error(t("common.copyFailed")));
    },
    [t],
  );

  const openArtifact = async (f: PackageEntry) => {
    setSelected(f);
    setActiveEntry("");
    setPreview(null);
    setEntries(null);
    if (!archiveKind(f.filename)) {
      setEntries([]);
      return;
    }
    setEntriesLoading(true);
    try {
      setEntries(await api.listPackageEntries(type, owner, name, f.version, f.filename));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
      setEntries([]);
    } finally {
      setEntriesLoading(false);
    }
  };

  const openEntry = async (entry: PackageFileEntry) => {
    if (entry.is_dir || !selected) return;
    setActiveEntry(entry.name);
    if (!isPreviewableName(entry.name)) {
      setPreview(null);
      return;
    }
    setPreviewLoading(true);
    try {
      setPreview(await api.readPackageEntry(type, owner, name, selected.version, selected.filename, entry.name));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
      setPreview(null);
    } finally {
      setPreviewLoading(false);
    }
  };

  const fileHref = (f: PackageEntry) =>
    `/api${packageFilePath(type, owner, name, { version: f.version, filename: f.filename, download: "1" })}`;
  const entryHref = (f: PackageEntry, entryName: string, download: boolean) =>
    `/api${packageFilePath(type, owner, name, {
      version: f.version,
      filename: f.filename,
      entry: entryName,
      ...(download ? { download: "1" } : {}),
    })}`;

  return (
    <div className="space-y-4">
      <Button asChild variant="ghost" size="sm" className="-ml-2 gap-1.5">
        <Link to="/packages">
          <ChevronLeft className="h-4 w-4" />
          {t("packages.back")}
        </Link>
      </Button>

      {error ? (
        <Card className="border-destructive">
          <CardContent className="pt-6 text-sm text-destructive">{error}</CardContent>
        </Card>
      ) : !files ? (
        <div className="space-y-3">
          <Skeleton className="h-8 w-64" />
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-40 w-full" />
        </div>
      ) : (
        <>
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="secondary">{type}</Badge>
            <h1 className="break-all font-mono text-xl font-bold">{name}</h1>
            <Badge variant={visibility === "private" ? "secondary" : "outline"} className="gap-1">
              {visibility === "private" ? <Lock className="h-3 w-3" /> : <Unlock className="h-3 w-3" />}
              {t(
                visibility === "private"
                  ? "packages.visPrivate"
                  : visibility === "anonymous"
                    ? "packages.visAnonymous"
                    : "packages.visPublic",
              )}
            </Badge>
            {canManage && latest && (
              <select
                value={visibility}
                onChange={(e) => void setVisibility(e.target.value)}
                aria-label={t("packages.visibility")}
                title={t("packages.visibility")}
                className="h-8 rounded-md border border-input bg-background px-2 text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <option value="private">{t("packages.visPrivate")}</option>
                <option value="public">{t("packages.visPublic")}</option>
                <option value="anonymous">{t("packages.visAnonymous")}</option>
              </select>
            )}
          </div>

          {latest && (
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-base">{t("packages.useCommand")}</CardTitle>
                <p className="text-xs text-muted-foreground">{t("packages.useCommandHint")}</p>
              </CardHeader>
              <CardContent className="space-y-3">
                <CommandBlock text={useCommand} onCopy={() => copy(useCommand)} />
                {setupCommands.length > 0 && (
                  <div className="space-y-1.5">
                    <p className="text-xs font-medium text-muted-foreground">{t("packages.setup")}</p>
                    <CommandBlock
                      text={setupCommands.join("\n")}
                      onCopy={() => copy(setupCommands.join("\n"))}
                      multiline
                    />
                  </div>
                )}
              </CardContent>
            </Card>
          )}

          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-base">{t("packages.files")}</CardTitle>
            </CardHeader>
            <CardContent>
              {files.length === 0 ? (
                <p className="py-6 text-center text-sm text-muted-foreground">{t("packages.noFiles")}</p>
              ) : (
                <div className="overflow-x-auto rounded-md border">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className="whitespace-nowrap">{t("packages.version")}</TableHead>
                        <TableHead className="whitespace-nowrap">{t("common.name")}</TableHead>
                        <TableHead className="w-28 whitespace-nowrap text-right">{t("packages.size")}</TableHead>
                        <TableHead className="w-40" />
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {files.map((f) => (
                        <TableRow key={f.id} className={selected?.id === f.id ? "bg-accent/50" : ""}>
                          <TableCell className="font-mono text-sm">{f.version}</TableCell>
                          <TableCell className="break-all font-mono text-sm">{f.filename}</TableCell>
                          <TableCell className="text-right text-sm">{formatSize(f.size)}</TableCell>
                          <TableCell>
                            <div className="flex justify-end gap-1">
                              {archiveKind(f.filename) && (
                                <Button size="sm" variant="outline" onClick={() => void openArtifact(f)}>
                                  {t("packages.browse")}
                                </Button>
                              )}
                              <Button asChild size="sm" variant="outline">
                                <a href={fileHref(f)} download>
                                  {t("packages.download")}
                                </a>
                              </Button>
                            </div>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              )}
            </CardContent>
          </Card>

          {selected && (
            <div className="grid gap-4 lg:grid-cols-[300px_minmax(0,1fr)]">
              <Card className="self-start">
                <CardHeader className="pb-2">
                  <CardTitle className="break-all font-mono text-sm">{selected.filename}</CardTitle>
                </CardHeader>
                <CardContent className="max-h-[60vh] overflow-auto p-2">
                  {entriesLoading ? (
                    <div className="flex items-center justify-center py-6 text-muted-foreground">
                      <Loader2 className="h-5 w-5 animate-spin" />
                    </div>
                  ) : entries && entries.length > 0 ? (
                    <ul className="space-y-0.5 text-sm">
                      {entries.map((e) => (
                        <li key={e.name}>
                          <button
                            className={`flex w-full items-center gap-2 rounded px-2 py-1 text-left hover:bg-accent ${
                              activeEntry === e.name ? "bg-accent" : ""
                            }`}
                            onClick={() => void openEntry(e)}
                          >
                            {e.is_dir ? (
                              <Folder className="h-3.5 w-3.5 shrink-0 text-blue-500" />
                            ) : (
                              <FileText className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                            )}
                            <span className="min-w-0 flex-1 break-all font-mono text-xs">{e.name}</span>
                            {!e.is_dir && (
                              <span className="shrink-0 text-[10px] text-muted-foreground">
                                {formatSize(e.size)}
                              </span>
                            )}
                          </button>
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <p className="px-2 py-4 text-sm text-muted-foreground">{t("packages.noFiles")}</p>
                  )}
                </CardContent>
              </Card>

              <Card className="min-w-0 self-start">
                <CardHeader className="flex-row items-center justify-between gap-2 space-y-0 pb-2">
                  <CardTitle className="break-all font-mono text-sm">
                    {activeEntry || selected.filename}
                  </CardTitle>
                  {activeEntry && (
                    <Button asChild size="sm" variant="outline" className="shrink-0 gap-1.5">
                      <a href={entryHref(selected, activeEntry, true)} download>
                        {t("packages.download")}
                      </a>
                    </Button>
                  )}
                </CardHeader>
                <CardContent className="min-w-0">
                  {!activeEntry ? (
                    <p className="py-10 text-center text-sm text-muted-foreground">
                      {t("packages.selectFile")}
                    </p>
                  ) : previewLoading ? (
                    <div className="flex items-center justify-center py-10 text-muted-foreground">
                      <Loader2 className="h-5 w-5 animate-spin" />
                    </div>
                  ) : preview !== null ? (
                    <div className="flex items-start gap-2">
                      <pre className="max-h-[60vh] min-w-0 flex-1 overflow-auto rounded-md border bg-muted/30 p-3 text-xs">
                        <code className="whitespace-pre-wrap break-all">{preview}</code>
                      </pre>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7 shrink-0"
                        onClick={() => copy(preview)}
                      >
                        <Copy className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  ) : (
                    <div className="flex flex-col items-center gap-2 py-10 text-sm text-muted-foreground">
                      <p>{t("packages.binaryFile")}</p>
                      <Button asChild size="sm" variant="outline">
                        <a href={entryHref(selected, activeEntry, true)} download>
                          {t("packages.download")}
                        </a>
                      </Button>
                    </div>
                  )}
                </CardContent>
              </Card>
            </div>
          )}
        </>
      )}
    </div>
  );
}
