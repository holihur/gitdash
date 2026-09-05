import { useCallback, useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";
import { Building2, Plus, Trash2 } from "lucide-react";
import { api, type Org, type OrgMember, type Repo } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter,
  DialogHeader, DialogTitle, DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import ConfirmDialog from "@/components/confirm-dialog";
import { formatDate } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

export default function Orgs() {
  const { t, lang, to } = useI18n();
  const nav = useNavigate();
  const { org: paramOrg } = useParams();
  const locale = lang === "zh-CN" ? "zh-CN" : "en-US";
  const [orgs, setOrgs] = useState<Org[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [display, setDisplay] = useState("");
  const [busy, setBusy] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<Org | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try { setOrgs(await api.listOrgs()); }
    catch (e) { toast.error(apiErrorMsg(to, e)); }
    finally { setLoading(false); }
  }, []);
  useEffect(() => { load(); }, [load]);

  const create = async () => {
    setBusy(true);
    try {
      const org = await api.createOrg(name.trim(), display.trim());
      toast.success(t("orgs.created", { name: org.name }));
      setOpen(false); setName(""); setDisplay(""); load();
      nav(`/orgs/${org.name}`);
    } catch (e) { toast.error(apiErrorMsg(to, e)); }
    finally { setBusy(false); }
  };

  const remove = async (org: Org) => {
    setPendingDelete(null);
    try {
      await api.deleteOrg(org.name);
      toast.success(t("orgs.deleted", { name: org.name }));
      if (paramOrg === org.name) nav("/orgs");
      load();
    } catch (e) { toast.error(apiErrorMsg(to, e)); }
  };

  const selected = orgs.find((o) => o.name === paramOrg) ?? null;

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold">{t("orgs.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("orgs.subtitle")}</p>
        </div>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button className="gap-2 sm:self-start"><Plus className="h-4 w-4" />{t("orgs.create")}</Button>
          </DialogTrigger>
          <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-md">
            <DialogHeader>
              <DialogTitle>{t("orgs.createTitle")}</DialogTitle>
              <DialogDescription>{t("orgs.createDescription")}</DialogDescription>
            </DialogHeader>
            <div className="grid gap-4">
              <div className="grid gap-2">
                <Label htmlFor="org-name">{t("orgs.name")}</Label>
                <Input id="org-name" placeholder={t("orgs.namePlaceholder")} value={name}
                  onChange={(e) => setName(e.target.value)} />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="org-display">{t("orgs.displayName")}</Label>
                <Input id="org-display" placeholder={t("orgs.displayPlaceholder")} value={display}
                  onChange={(e) => setDisplay(e.target.value)} />
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
          {Array.from({ length: 3 }).map((_, i) => <Skeleton key={i} className="h-6 w-full" />)}
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
            <TableHeader><TableRow>
              <TableHead>{t("orgs.name")}</TableHead>
              <TableHead>{t("orgs.displayName")}</TableHead>
              <TableHead>{t("orgs.role")}</TableHead>
              <TableHead>{t("orgs.createdAt")}</TableHead>
              <TableHead className="w-12" />
            </TableRow></TableHeader>
            <TableBody>
              {orgs.map((org) => (
                <TableRow key={org.name} className="cursor-pointer"
                  onClick={() => nav(`/orgs/${org.name}`)}>
                  <TableCell className="font-medium">
                    <Link to={`/orgs/${org.name}`} onClick={(e) => e.stopPropagation()}
                      className="hover:underline">{org.name}</Link>
                  </TableCell>
                  <TableCell>{org.display || <span className="text-muted-foreground">—</span>}</TableCell>
                  <TableCell><Badge variant="secondary">{org.role}</Badge></TableCell>
                  <TableCell className="text-sm text-muted-foreground">{formatDate(org.created_at, locale)}</TableCell>
                  <TableCell onClick={(e) => e.stopPropagation()}>
                    {org.role === "owner" && (
                      <Button variant="ghost" size="icon"
                        className="h-8 w-8 text-destructive hover:text-destructive"
                        onClick={() => setPendingDelete(org)}>
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

      {selected && <OrgDetail org={selected} locale={locale} onOrgDeleted={() => { nav("/orgs"); load(); }} />}

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(o) => !o && setPendingDelete(null)}
        description={t("orgs.confirmDelete", { name: pendingDelete?.name ?? "" })}
        onConfirm={() => pendingDelete && remove(pendingDelete)}
      />
    </div>
  );
}

function OrgDetail({ org, locale, onOrgDeleted }: { org: Org; locale: string; onOrgDeleted: () => void }) {
  const { t, to } = useI18n();
  const isOwner = org.role === "owner";
  const [members, setMembers] = useState<OrgMember[]>([]);
  const [repos, setRepos] = useState<Repo[]>([]);
  const [newMember, setNewMember] = useState("");
  const [newRole, setNewRole] = useState("member");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const [ms, rs] = await Promise.all([api.listOrgMembers(org.name), api.listOrgRepos(org.name)]);
      setMembers(ms); setRepos(rs.repos);
    } catch (e) { toast.error(apiErrorMsg(to, e)); }
  }, [org.name]);
  useEffect(() => { load(); }, [load]);

  const addMember = async () => {
    setBusy(true);
    try {
      await api.addOrgMember(org.name, newMember.trim(), newRole);
      toast.success(t("orgs.memberAdded", { name: newMember.trim() }));
      setNewMember(""); setNewRole("member"); load();
    } catch (e) { toast.error(apiErrorMsg(to, e)); }
    finally { setBusy(false); }
  };

  const removeMember = async (username: string) => {
    try {
      await api.removeOrgMember(org.name, username);
      toast.success(t("orgs.memberRemoved", { name: username }));
      load();
    } catch (e) { toast.error(apiErrorMsg(to, e)); }
  };

  return (
    <div className="space-y-6 rounded-lg border p-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Building2 className="h-5 w-5 text-muted-foreground" />
          <h2 className="text-lg font-semibold">{org.name}</h2>
          <Badge variant="secondary">{org.role}</Badge>
        </div>
        {isOwner && (
          <Button variant="outline" size="sm" className="gap-2 text-destructive hover:text-destructive"
            onClick={() => {
              api.deleteOrg(org.name)
                .then(() => { toast.success(t("orgs.deleted", { name: org.name })); onOrgDeleted(); })
                .catch((e) => toast.error(apiErrorMsg(to, e)));
            }}>
            <Trash2 className="h-4 w-4" />{t("orgs.deleteOrg")}
          </Button>
        )}
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <div>
          <h3 className="mb-2 text-sm font-medium">{t("orgs.members")}</h3>
          <div className="overflow-hidden rounded-lg border">
            <Table>
              <TableHeader><TableRow>
                <TableHead>{t("orgs.memberUser")}</TableHead>
                <TableHead>{t("orgs.role")}</TableHead>
                {isOwner && <TableHead className="w-12" />}
              </TableRow></TableHeader>
              <TableBody>
                {members.map((m) => (
                  <TableRow key={m.username}>
                    <TableCell className="font-medium">{m.username}</TableCell>
                    <TableCell><Badge variant="secondary">{m.role}</Badge></TableCell>
                    {isOwner && (
                      <TableCell>
                        <Button variant="ghost" size="icon"
                          className="h-8 w-8 text-destructive hover:text-destructive"
                          onClick={() => removeMember(m.username)}>
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </TableCell>
                    )}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          {isOwner && (
            <div className="mt-3 flex flex-col gap-2 sm:flex-row">
              <Input placeholder={t("orgs.memberUser")} value={newMember}
                onChange={(e) => setNewMember(e.target.value)} />
              <select value={newRole} onChange={(e) => setNewRole(e.target.value)}
                className="h-9 rounded-md border border-input bg-transparent px-2 text-sm">
                <option value="member">member</option>
                <option value="owner">owner</option>
              </select>
              <Button onClick={addMember} disabled={busy || !newMember.trim()} className="sm:self-start">
                {t("common.add")}
              </Button>
            </div>
          )}
        </div>

        <div>
          <h3 className="mb-2 text-sm font-medium">{t("orgs.repos")}</h3>
          {repos.length === 0 ? (
            <div className="rounded-lg border border-dashed py-10 text-center text-sm text-muted-foreground">
              {t("orgs.noRepos")}
            </div>
          ) : (
            <div className="divide-y rounded-lg border">
              {repos.map((r) => (
                <Link key={`${r.owner}/${r.name}`} to={`/repo/${r.owner}/${r.name}`}
                  className="flex items-center justify-between px-3 py-2 text-sm hover:bg-accent">
                  <span className="font-medium">{r.name}</span>
                  <span className="text-xs text-muted-foreground">
                    {t("orgs.updatedAt", { date: formatDate(r.created_at, locale) })}
                  </span>
                </Link>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
