import { useCallback, useEffect, useState } from "react";
import { Package as PackageIcon, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api, type PackageAuditEntry, type PackageEntry } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import ConfirmDialog from "@/components/confirm-dialog";
import { formatDate } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

const PKG_TYPES = ["npm", "composer", "pypi", "rubygems", "go", "cargo", "maven"] as const;

export default function Packages() {
  const [type, setType] = useState<string>("");
  const [pkgs, setPkgs] = useState<PackageEntry[]>([]);
  const [audit, setAudit] = useState<PackageAuditEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [pendingDelete, setPendingDelete] = useState<PackageEntry | null>(null);
  const { t, lang } = useI18n();
  const locale = lang === "zh-CN" ? "zh-CN" : undefined;

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const me = await api.me();
      const [list, log] = await Promise.all([
        api.listPackages(me.username, type || undefined),
        api.listPackageAudit(me.username),
      ]);
      setPkgs(list);
      setAudit(log);
    } catch (e) {
      toast.error(apiErrorMsg(t, e));
    } finally {
      setLoading(false);
    }
  }, [type, t]);

  useEffect(() => {
    void load();
  }, [load]);

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

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("packages.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("packages.subtitle")}</p>
      </div>

      <Tabs value={type} onValueChange={setType}>
        <TabsList>
          <TabsTrigger value="">{t("packages.all")}</TabsTrigger>
          {PKG_TYPES.map((tp) => (
            <TabsTrigger key={tp} value={tp}>
              {tp}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>

      {loading ? (
        <div className="space-y-2">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-2/3" />
        </div>
      ) : pkgs.length === 0 ? (
        <div className="flex flex-col items-center gap-2 py-12 text-muted-foreground">
          <PackageIcon className="h-10 w-10" />
          <p className="text-sm">{t("packages.empty")}</p>
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("packages.type")}</TableHead>
              <TableHead>{t("common.name")}</TableHead>
              <TableHead>{t("packages.version")}</TableHead>
              <TableHead>{t("packages.size")}</TableHead>
              <TableHead>{t("packages.downloads")}</TableHead>
              <TableHead>{t("common.createdAt")}</TableHead>
              <TableHead className="w-12" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {pkgs.map((p) => (
              <TableRow key={p.id}>
                <TableCell>
                  <Badge variant="secondary">{p.type}</Badge>
                </TableCell>
                <TableCell className="font-mono text-sm">{p.name}</TableCell>
                <TableCell className="font-mono text-sm">{p.version}</TableCell>
                <TableCell className="text-sm">{(p.size / 1024).toFixed(1)} KB</TableCell>
                <TableCell className="text-sm">{p.downloads}</TableCell>
                <TableCell className="text-sm">{formatDate(p.created_at, locale)}</TableCell>
                <TableCell>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 text-destructive"
                    onClick={() => setPendingDelete(p)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      {audit.length > 0 && (
        <div>
          <h2 className="mb-2 text-lg font-semibold">{t("packages.audit")}</h2>
          <div className="rounded-md border p-3 text-sm">
            {audit.slice(0, 20).map((a) => (
              <div key={a.id} className="flex gap-2 py-0.5 text-muted-foreground">
                <span>{formatDate(a.created_at, locale)}</span>
                <span className="font-mono">{a.actor}</span>
                <span className="font-medium">{a.action}</span>
                <span className="font-mono">
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
