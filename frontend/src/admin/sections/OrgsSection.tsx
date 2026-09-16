import { useCallback, useEffect, useState } from "react";
import { Search } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { adminList, toastError } from "../api";
import { BanButton } from "../BanButton";
import { PAGE_SIZE, type AdminOrg } from "../types";

export function OrgsSection() {
  const { t, to } = useI18n();
  const [orgs, setOrgs] = useState<AdminOrg[] | null>(null);
  const [query, setQuery] = useState("");
  const [activeQuery, setActiveQuery] = useState("");
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  const load = useCallback(
    async (q: string, p: number) => {
      const params = new URLSearchParams({ limit: String(PAGE_SIZE), offset: String((p - 1) * PAGE_SIZE) });
      if (q) params.set("q", q);
      try {
        const r = await adminList<AdminOrg[]>(`/orgs?${params}`);
        setOrgs(r.items);
        setTotal(r.total);
      } catch (e) {
        toastError(to, e);
      }
    },
    [to],
  );

  useEffect(() => {
    void load(activeQuery, page);
  }, [load, activeQuery, page]);

  const search = (e: React.FormEvent) => {
    e.preventDefault();
    setPage(1);
    setActiveQuery(query.trim());
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("admin.orgsTitle")}</CardTitle>
      </CardHeader>
      <CardContent className="grid gap-4">
        <form onSubmit={search} className="flex gap-2">
          <div className="relative flex-1">
            <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input className="pl-8" value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t("admin.orgsSearch")} />
          </div>
          <Button type="submit" variant="outline">{t("admin.usersSearchBtn")}</Button>
        </form>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("admin.usersColUsername")}</TableHead>
              <TableHead className="text-right">{t("admin.usersColActions")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {orgs === null ? null : orgs.length === 0 ? (
              <TableRow>
                <TableCell colSpan={2} className="py-6 text-center text-muted-foreground">
                  {t("admin.orgsEmpty")}
                </TableCell>
              </TableRow>
            ) : (
              orgs.map((o) => (
                <TableRow key={o.id}>
                  <TableCell className="font-medium">
                    {o.name}
                    {o.banned && <Badge variant="destructive" className="ml-2">{t("admin.banned")}</Badge>}
                  </TableCell>
                  <TableCell className="text-right">
                    <BanButton
                      path={`/orgs/${encodeURIComponent(o.name)}/ban`}
                      banned={o.banned}
                      name={o.name}
                      onChanged={() => void load(activeQuery, page)}
                    />
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
        <div className="flex items-center justify-end gap-2">
          <span className="text-sm text-muted-foreground">{t("admin.usersPageOf", { page, pages })}</span>
          <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
            {t("admin.usersPrev")}
          </Button>
          <Button variant="outline" size="sm" disabled={page >= pages} onClick={() => setPage((p) => p + 1)}>
            {t("admin.usersNext")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

