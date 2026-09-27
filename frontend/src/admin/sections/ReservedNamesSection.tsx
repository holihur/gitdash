import { useCallback, useEffect, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { adminList, adminUserReq, toastError } from "../api";

/** 注册保留名黑名单：名单内的用户名禁止自助注册（管理员仍可创建）。 */
export function ReservedNamesSection() {
  const { t, to } = useI18n();
  const [names, setNames] = useState<string[] | null>(null);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const r = await adminList<{ names: string[] }>("/reserved-names");
      setNames(r.items.names ?? []);
    } catch (e) {
      toastError(to, e);
    }
  }, [to]);

  useEffect(() => {
    void load();
  }, [load]);

  const add = async (e: React.FormEvent) => {
    e.preventDefault();
    const n = name.trim().toLowerCase();
    if (!n) return;
    setBusy(true);
    try {
      const r = await adminUserReq("/reserved-names", "POST", { name: n });
      if (!r.ok) {
        toast.error(r.error ?? t("admin.reservedAddFailed"));
        return;
      }
      toast.success(t("admin.reservedAdded", { name: n }));
      setName("");
      await load();
    } catch (e) {
      toastError(to, e);
    } finally {
      setBusy(false);
    }
  };

  const remove = async (n: string) => {
    try {
      const r = await adminUserReq(`/reserved-names/${encodeURIComponent(n)}`, "DELETE");
      if (!r.ok) {
        toast.error(r.error ?? t("admin.reservedRemoveFailed"));
        return;
      }
      toast.success(t("admin.reservedRemoved", { name: n }));
      setNames((prev) => (prev ? prev.filter((x) => x !== n) : prev));
    } catch (e) {
      toastError(to, e);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("admin.reservedTitle")}</CardTitle>
        <CardDescription>{t("admin.reservedHint")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        <form onSubmit={add} className="flex flex-wrap items-end gap-2">
          <div className="grid flex-1 gap-1">
            <Input
              aria-label={t("admin.reservedName")}
              placeholder="admin, api, support…"
              value={name}
              maxLength={255}
              className="font-mono"
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <Button type="submit" variant="outline" className="gap-1" disabled={busy || !name.trim()}>
            <Plus className="h-4 w-4" />
            {t("admin.reservedAdd")}
          </Button>
        </form>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("admin.reservedName")}</TableHead>
              <TableHead className="text-right">{t("admin.usersColActions")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {names === null ? null : names.length === 0 ? (
              <TableRow>
                <TableCell colSpan={2} className="py-6 text-center text-muted-foreground">
                  {t("admin.reservedEmpty")}
                </TableCell>
              </TableRow>
            ) : (
              names.map((n) => (
                <TableRow key={n}>
                  <TableCell className="font-mono text-sm">{n}</TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="ghost"
                      size="sm"
                      className="gap-1 text-destructive hover:text-destructive"
                      onClick={() => void remove(n)}
                    >
                      <Trash2 className="h-4 w-4" />
                      {t("admin.reservedRemove")}
                    </Button>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}
