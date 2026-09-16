import { useCallback, useEffect, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { adminList, adminUserReq, toastError, type Translator } from "../api";
import { type AdminIPBan } from "../types";

export function ipBanActionError(
  to: Translator,
  r: { ok: false; status: number; code?: string; error?: string },
): string {
  if (r.code) {
    const localized = to(`errors.${r.code}`);
    if (localized) return localized;
  }
  if (r.error) return r.error;
  return to("admin.ipBanFailed") ?? "request failed";
}

export function IPBlacklistSection() {
  const { t, to } = useI18n();
  const [bans, setBans] = useState<AdminIPBan[] | null>(null);
  const [cidr, setCidr] = useState("");
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const r = await adminList<AdminIPBan[]>("/ip-bans");
      setBans(r.items);
    } catch (e) {
      toastError(to, e);
    }
  }, [to]);

  useEffect(() => {
    void load();
  }, [load]);

  const add = async (e: React.FormEvent) => {
    e.preventDefault();
    const c = cidr.trim();
    if (!c) return;
    setBusy(true);
    try {
      const r = await adminUserReq("/ip-bans", "POST", { cidr: c, note: note.trim() });
      if (!r.ok) {
        toast.error(ipBanActionError(to, r));
        return;
      }
      toast.success(t("admin.ipBanAdded", { cidr: c }));
      setCidr("");
      setNote("");
      await load();
    } catch (e) {
      toastError(to, e);
    } finally {
      setBusy(false);
    }
  };

  const remove = async (b: AdminIPBan) => {
    try {
      const r = await adminUserReq(`/ip-bans/${b.id}`, "DELETE");
      if (!r.ok) {
        toast.error(ipBanActionError(to, r));
        return;
      }
      toast.success(t("admin.ipBanRemoved", { cidr: b.cidr }));
      setBans((prev) => (prev ? prev.filter((x) => x.id !== b.id) : prev));
    } catch (e) {
      toastError(to, e);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("admin.ipBanTitle")}</CardTitle>
        <CardDescription>{t("admin.ipBanHint")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        <form onSubmit={add} className="flex flex-wrap items-end gap-2">
          <div className="grid flex-1 gap-1">
            <Label className="text-xs text-muted-foreground" htmlFor="ipban-cidr">
              {t("admin.ipBanCIDR")}
            </Label>
            <Input
              id="ipban-cidr"
              value={cidr}
              onChange={(e) => setCidr(e.target.value)}
              placeholder="203.0.113.0/24"
              className="font-mono"
            />
          </div>
          <div className="grid flex-1 gap-1">
            <Label className="text-xs text-muted-foreground" htmlFor="ipban-note">
              {t("admin.ipBanNote")}
            </Label>
            <Input id="ipban-note" value={note} onChange={(e) => setNote(e.target.value)} />
          </div>
          <Button type="submit" variant="outline" className="gap-1" disabled={busy || !cidr.trim()}>
            <Plus className="h-4 w-4" />
            {t("admin.ipBanAdd")}
          </Button>
        </form>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("admin.ipBanColCIDR")}</TableHead>
              <TableHead>{t("admin.ipBanColNote")}</TableHead>
              <TableHead>{t("admin.usersColCreated")}</TableHead>
              <TableHead className="text-right">{t("admin.usersColActions")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {bans === null ? null : bans.length === 0 ? (
              <TableRow>
                <TableCell colSpan={4} className="py-6 text-center text-muted-foreground">
                  {t("admin.ipBanEmpty")}
                </TableCell>
              </TableRow>
            ) : (
              bans.map((b) => (
                <TableRow key={b.id}>
                  <TableCell className="font-mono text-sm">{b.cidr}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">{b.note || "—"}</TableCell>
                  <TableCell>{new Date(b.created_at).toLocaleDateString()}</TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="ghost"
                      size="sm"
                      className="gap-1 text-destructive hover:text-destructive"
                      onClick={() => void remove(b)}
                    >
                      <Trash2 className="h-4 w-4" />
                      {t("admin.ipBanRemove")}
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

