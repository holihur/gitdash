import { useCallback, useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { Building2, Plus, Trash2 } from "lucide-react";
import { api, type Org } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import ConfirmDialog from "@/components/confirm-dialog";
import { formatDate } from "@/lib/utils";
import { dateLocale, useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

export default function Orgs() {
  const { t, lang, to } = useI18n();
  const nav = useNavigate();
  const locale = dateLocale(lang);
  const [orgs, setOrgs] = useState<Org[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [display, setDisplay] = useState("");
  const [busy, setBusy] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<Org | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setOrgs(await api.listOrgs());
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setLoading(false);
    }
  }, [to]);
  useEffect(() => {
    load();
  }, [load]);

  const create = async () => {
    setBusy(true);
    try {
      const org = await api.createOrg(name.trim(), display.trim());
      toast.success(t("orgs.created", { name: org.name }));
      setOpen(false);
      setName("");
      setDisplay("");
      nav(`/orgs/${org.name}`);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (org: Org) => {
    setPendingDelete(null);
    try {
      await api.deleteOrg(org.name);
      toast.success(t("orgs.deleted", { name: org.name }));
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold">{t("orgs.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("orgs.subtitle")}</p>
        </div>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button className="gap-2 sm:self-start">
              <Plus className="h-4 w-4" />
              {t("orgs.create")}
            </Button>
          </DialogTrigger>
          <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-md">
            <DialogHeader>
              <DialogTitle>{t("orgs.createTitle")}</DialogTitle>
              <DialogDescription>{t("orgs.createDescription")}</DialogDescription>
            </DialogHeader>
            <div className="grid gap-4">
              <div className="grid gap-2">
                <Label htmlFor="org-name">{t("orgs.name")}</Label>
                <Input
                  id="org-name"
                  placeholder={t("orgs.namePlaceholder")}
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="org-display">{t("orgs.displayName")}</Label>
                <Input
                  id="org-display"
                  placeholder={t("orgs.displayPlaceholder")}
                  value={display}
                  onChange={(e) => setDisplay(e.target.value)}
                />
              </div>
            </div>
            <DialogFooter>
              <Button onClick={create} disabled={busy || !name.trim()}>
                {t("common.create")}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      {loading ? (
        <div className="space-y-3 rounded-lg border p-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-6 w-full" />
          ))}
        </div>
      ) : orgs.length === 0 ? (
        <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed py-16 px-4 text-center">
          <Building2 className="h-10 w-10 text-muted-foreground" />
          <p className="font-medium">{t("orgs.empty")}</p>
          <p className="text-sm text-muted-foreground">{t("orgs.emptyHint")}</p>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <Table className="min-w-[560px]">
            <TableHeader>
              <TableRow>
                <TableHead>{t("orgs.name")}</TableHead>
                <TableHead>{t("orgs.displayName")}</TableHead>
                <TableHead>{t("orgs.role")}</TableHead>
                <TableHead>{t("orgs.createdAt")}</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {orgs.map((org) => (
                <TableRow
                  key={org.name}
                  className="cursor-pointer"
                  onClick={() => nav(`/orgs/${org.name}`)}
                >
                  <TableCell className="font-medium">
                    <Link
                      to={`/orgs/${org.name}`}
                      onClick={(e) => e.stopPropagation()}
                      className="hover:underline"
                    >
                      {org.name}
                    </Link>
                  </TableCell>
                  <TableCell>{org.display || <span className="text-muted-foreground">—</span>}</TableCell>
                  <TableCell>
                    <Badge variant="secondary">{org.role}</Badge>
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(org.created_at, locale)}
                  </TableCell>
                  <TableCell onClick={(e) => e.stopPropagation()}>
                    {org.role === "owner" && (
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 text-destructive hover:text-destructive"
                        onClick={() => setPendingDelete(org)}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(o) => !o && setPendingDelete(null)}
        description={t("orgs.confirmDelete", { name: pendingDelete?.name ?? "" })}
        onConfirm={() => pendingDelete && remove(pendingDelete)}
      />
    </div>
  );
}
