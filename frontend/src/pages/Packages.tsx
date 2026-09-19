import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { Copy, Package as PackageIcon, Search, Terminal, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, type DockerImage, type Org, type PackageAuditEntry, type PackageEntry } from "@/lib/api";
import { packageDetailPath } from "@/lib/api/packages";
import { packageUseCommand } from "@/lib/package-command";
import { copyText } from "@/lib/utils";
import { useQueryState } from "@/lib/query-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
import { Tabs } from "@/components/ui/tabs";
import { TabsListOverflow } from "@/components/ui/tabs-overflow";
import Pagination from "@/components/ui/pagination";
import { Skeleton } from "@/components/ui/skeleton";
import ConfirmDialog from "@/components/confirm-dialog";
import { formatDate } from "@/lib/utils";
import { dateLocale, useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

const PKG_TYPES = ["npm", "composer", "pypi", "rubygems", "go", "cargo", "maven", "docker"] as const;

export default function Packages() {
  const { t, lang } = useI18n();
  const locale = dateLocale(lang);
  // type / 搜索词 / 页码 / 页大小同步进 URL(?type/?q/?page/?size)
  const { get, getNum, set } = useQueryState();
  const type = get("type", "");
  const query = get("q", "");
  const page = getNum("page", 1);
  const pageSize = getNum("size", 20);
  const ownerParam = get("owner", "");

  const [pkgs, setPkgs] = useState<PackageEntry[]>([]);
  const [pkgTotal, setPkgTotal] = useState(0);
  const [dockerImages, setDockerImages] = useState<DockerImage[]>([]);
  const [username, setUsername] = useState("");
  const [orgs, setOrgs] = useState<Org[]>([]);
  const [audit, setAudit] = useState<PackageAuditEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [pendingDelete, setPendingDelete] = useState<PackageEntry | null>(null);

  // 命名空间：默认个人，可切到所属组织（?owner= 同步进 URL）
  const owner = ownerParam || username;
  const selectedOrg = orgs.find((o) => o.name === owner);
  // 组织只有 owner 角色可发布/删除；个人命名空间总能管理
  const canManage = owner !== "" && (owner === username || selectedOrg?.role === "owner");

  // 搜索框本地态 + 300ms 防抖写回 URL（避免每敲一个字符发一次请求）
  const [qInput, setQInput] = useState(query);
  useEffect(() => setQInput(query), [query]);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const onSearch = (v: string) => {
    setQInput(v);
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => set({ q: v || null, page: null }), 300);
  };
  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );

  const setType = (v: string) => set({ type: v || null, page: null }, { push: true });
  const setOwner = (v: string) => set({ owner: v || null, page: null }, { push: true });
  const setPage = (p: number) => set({ page: p > 1 ? p : null }, { push: true });
  const setPageSize = (s: number) => set({ size: s === 20 ? null : s, page: null });

  useEffect(() => {
    api.me().then((m) => setUsername(m.username)).catch(() => undefined);
    api.listOrgs().then(setOrgs).catch(() => setOrgs([]));
  }, []);

  const load = useCallback(async () => {
    if (!owner) return;
    setLoading(true);
    try {
      if (type === "docker") {
        setDockerImages(await api.listDockerImages(owner));
        setPkgs([]);
        setPkgTotal(0);
      } else {
        const [list, log] = await Promise.all([
          api.listPackagesPage(owner, type || undefined, {
            q: query.trim() || undefined,
            limit: pageSize,
            offset: (page - 1) * pageSize,
          }),
          api.listPackageAudit(owner),
        ]);
        setPkgs(list.items);
        setPkgTotal(list.total);
        setAudit(log);
      }
    } catch (e) {
      toast.error(apiErrorMsg(t, e));
    } finally {
      setLoading(false);
    }
  }, [owner, type, query, page, pageSize, t]);

  useEffect(() => {
    void load();
  }, [load]);

  const packageTabs = useMemo(
    () => [
      { value: "", label: t("packages.all") },
      ...PKG_TYPES.map((tp) => ({ value: tp, label: tp })),
    ],
    [t],
  );

  // docker 镜像接口不分页，前端做过滤 + 分页
  const dockerFiltered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return needle ? dockerImages.filter((i) => i.name.toLowerCase().includes(needle)) : dockerImages;
  }, [dockerImages, query]);
  const dockerPageItems = useMemo(() => {
    const start = (page - 1) * pageSize;
    return dockerFiltered.slice(start, start + pageSize);
  }, [dockerFiltered, page, pageSize]);

  const total = type === "docker" ? dockerFiltered.length : pkgTotal;

  const remove = async (p: PackageEntry) => {
    try {
      await api.deletePackage(p.type, p.owner, p.name);
      toast.success(t("packages.deleted"));
      setPendingDelete(null);
      void load();
    } catch (e) {
      toast.error(apiErrorMsg(t, e));
    }
  };

  const copyPull = async (img: DockerImage) => {
    const tag = img.tags[0] ?? "latest";
    const host = typeof window !== "undefined" ? window.location.host : "localhost:8080";
    try {
      await copyText(`docker pull ${host}/${owner}/${img.name}:${tag}`);
      toast.success(t("common.copied"));
    } catch {
      toast.error(t("common.copyFailed"));
    }
  };

  // 一键复制该包的安装 / 依赖命令
  const copyUse = async (p: PackageEntry) => {
    try {
      await copyText(packageUseCommand(p));
      toast.success(t("common.copied"));
    } catch {
      toast.error(t("common.copyFailed"));
    }
  };

  const namespaceSelect =
    orgs.length > 0 ? (
      <div className="flex items-center gap-2">
        <Label htmlFor="pkg-owner" className="shrink-0">
          {t("packages.namespaceLabel")}
        </Label>
        <select
          id="pkg-owner"
          value={ownerParam}
          onChange={(e) => setOwner(e.target.value)}
          className="h-9 w-full max-w-[16rem] rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <option value="">
            {username ? `${t("packages.namespacePersonal")} (${username})` : t("packages.namespacePersonal")}
          </option>
          {orgs.map((o) => (
            <option key={o.name} value={o.name}>
              {o.name}
              {o.role === "owner" ? " · owner" : ""}
            </option>
          ))}
        </select>
      </div>
    ) : null;

  const searchBox = (
    <div className="relative">
      <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
      <Input
        className="pl-9"
        placeholder={t("packages.searchPlaceholder")}
        value={qInput}
        onChange={(e) => onSearch(e.target.value)}
      />
    </div>
  );

  const pagination = (
    <Pagination
      page={page}
      pageSize={pageSize}
      total={total}
      onPageChange={setPage}
      onPageSizeChange={setPageSize}
    />
  );

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("packages.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("packages.subtitle")}</p>
      </div>

      {namespaceSelect}

      <Tabs value={type} onValueChange={setType}>
        <TabsListOverflow tabs={packageTabs} value={type} onValueChange={setType} />
      </Tabs>

      {searchBox}

      {loading ? (
        <div className="space-y-2">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-2/3" />
        </div>
      ) : type === "docker" ? (
        dockerImages.length === 0 ? (
          <div className="flex flex-col items-center gap-2 py-12 text-muted-foreground">
            <PackageIcon className="h-10 w-10" />
            <p className="text-sm">{t("packages.dockerEmpty")}</p>
          </div>
        ) : (
          <>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="whitespace-nowrap">{t("packages.type")}</TableHead>
                  <TableHead className="whitespace-nowrap">{t("common.name")}</TableHead>
                  <TableHead className="whitespace-nowrap">{t("packages.version")}</TableHead>
                  <TableHead className="whitespace-nowrap">{t("packages.dockerPull")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {dockerPageItems.map((img) => {
                  const tag = img.tags[0] ?? "latest";
                  const host = typeof window !== "undefined" ? window.location.host : "localhost:8080";
                  const pull = `docker pull ${host}/${owner}/${img.name}:${tag}`;
                  return (
                    <TableRow key={img.name}>
                      <TableCell>
                        <Badge variant="secondary">docker</Badge>
                      </TableCell>
                      <TableCell className="font-mono text-sm">{img.name}</TableCell>
                      <TableCell className="font-mono text-sm">
                        {img.tags.length ? img.tags.join(", ") : "—"}
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <code className="max-w-[60vw] truncate rounded bg-muted px-2 py-1 text-xs">{pull}</code>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8 shrink-0"
                            onClick={() => void copyPull(img)}
                          >
                            <Copy className="h-4 w-4" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
            {pagination}
          </>
        )
      ) : pkgs.length === 0 ? (
        <div className="flex flex-col items-center gap-2 py-12 text-muted-foreground">
          <PackageIcon className="h-10 w-10" />
          <p className="text-sm">{t("packages.empty")}</p>
        </div>
      ) : (
        <>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="whitespace-nowrap">{t("packages.type")}</TableHead>
                <TableHead className="whitespace-nowrap">{t("common.name")}</TableHead>
                <TableHead className="whitespace-nowrap">{t("packages.version")}</TableHead>
                <TableHead className="whitespace-nowrap">{t("packages.size")}</TableHead>
                <TableHead className="whitespace-nowrap">{t("packages.downloads")}</TableHead>
                <TableHead className="whitespace-nowrap">{t("common.createdAt")}</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {pkgs.map((p) => (
                <TableRow key={p.id}>
                  <TableCell>
                    <Badge variant="secondary">{p.type}</Badge>
                  </TableCell>
                  <TableCell className="font-mono text-sm">
                    <Link to={packageDetailPath(p.type, p.owner, p.name)} className="hover:underline">
                      {p.name}
                    </Link>
                  </TableCell>
                  <TableCell className="font-mono text-sm">{p.version}</TableCell>
                  <TableCell className="text-sm">{(p.size / 1024).toFixed(1)} KB</TableCell>
                  <TableCell className="text-sm">{p.downloads}</TableCell>
                  <TableCell className="text-sm">{formatDate(p.created_at, locale)}</TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-1">
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8"
                        title={t("packages.useCommand")}
                        aria-label={t("packages.useCommand")}
                        onClick={() => void copyUse(p)}
                      >
                        <Terminal className="h-4 w-4" />
                      </Button>
                      {canManage && (
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-8 w-8 text-destructive"
                          onClick={() => setPendingDelete(p)}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          {pagination}
        </>
      )}

      {type !== "docker" && audit.length > 0 && (
        <div>
          <h2 className="mb-2 text-lg font-semibold">{t("packages.audit")}</h2>
          <div className="overflow-hidden rounded-md border p-3 text-sm">
            {audit.slice(0, 20).map((a) => (
              <div
                key={a.id}
                className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 py-0.5 text-muted-foreground"
              >
                <span className="whitespace-nowrap">{formatDate(a.created_at, locale)}</span>
                <span className="whitespace-nowrap font-mono">{a.actor}</span>
                <span className="whitespace-nowrap font-medium">{a.action}</span>
                <span className="min-w-0 break-all font-mono">
                  {a.type}/{a.name}
                  {a.version ? `@${a.version}` : ""}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(o) => !o && setPendingDelete(null)}
        description={t("packages.deleteDesc", { name: pendingDelete?.name ?? "" })}
        onConfirm={() => pendingDelete && void remove(pendingDelete)}
      />
    </div>
  );
}
