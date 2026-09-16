import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { KeyRound, Plus, Search, Trash2, UserPlus } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { adminList, adminUserReq, toastError, userActionError } from "../api";
import { BanButton } from "../BanButton";
import { PAGE_SIZE, type AdminUser } from "../types";

export function UsersSection() {
  const [refresh, setRefresh] = useState(0);
  return (
    <div className="space-y-4">
      <CreateUserCard onCreated={() => setRefresh((n) => n + 1)} />
      <UsersCard refresh={refresh} />
    </div>
  );
}

export function CreateUserCard({ onCreated }: { onCreated?: () => void }) {
  const { t, to } = useI18n();
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setMsg("");
    const r = await adminUserReq("/users", "POST", {
      username,
      password,
      email: email || undefined,
    });
    setBusy(false);
    if (!r.ok) {
      setMsg(userActionError(to, r, "admin.userCreated"));
      return;
    }
    toast.success(t("admin.userCreated", { name: username }));
    setUsername("");
    setEmail("");
    setPassword("");
    onCreated?.();
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <UserPlus className="h-4 w-4" />
          {t("admin.usersCreateTitle")}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <form onSubmit={submit} className="grid gap-3 md:grid-cols-3">
          <div className="grid gap-2">
            <Label htmlFor="new-user-name">{t("admin.usersCreateUsername")}</Label>
            <Input id="new-user-name" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="off" required />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="new-user-email">{t("admin.usersCreateEmail")}</Label>
            <Input id="new-user-email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="off" />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="new-user-pass">{t("admin.usersCreatePassword")}</Label>
            <Input id="new-user-pass" type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" minLength={8} required />
          </div>
          {msg && <p className="text-sm text-destructive md:col-span-3">{msg}</p>}
          <div className="md:col-span-3">
            <Button type="submit" disabled={busy || !username || password.length < 8}>
              <Plus className="h-4 w-4" />
              {t("admin.usersCreate")}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

export function UsersCard({ refresh = 0 }: { refresh?: number }) {
  const { t, to } = useI18n();
  const [users, setUsers] = useState<AdminUser[] | null>(null);
  const [query, setQuery] = useState("");
  const [activeQuery, setActiveQuery] = useState("");
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);

  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  const load = useCallback(async (q: string, p: number) => {
    const params = new URLSearchParams({ limit: String(PAGE_SIZE), offset: String((p - 1) * PAGE_SIZE) });
    if (q) params.set("q", q);
    try {
      const r = await adminList<AdminUser[]>(`/users?${params}`);
      setUsers(r.items);
      setTotal(r.total);
    } catch (e) {
      toastError(to, e);
    }
  }, [to]);

  useEffect(() => {
    void load(activeQuery, page);
  }, [load, activeQuery, page, refresh]);

  const reload = () => {
    void load(activeQuery, page);
  };

  const search = (e: React.FormEvent) => {
    e.preventDefault();
    setPage(1);
    setActiveQuery(query.trim());
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("admin.usersTitle")}</CardTitle>
        <CardDescription>{t("admin.usersHint")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        <form onSubmit={search} className="flex gap-2">
          <div className="relative flex-1">
            <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input
              className="pl-8"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t("admin.usersSearch")}
            />
          </div>
          <Button type="submit" variant="outline">{t("admin.usersSearchBtn")}</Button>
        </form>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("admin.usersColUsername")}</TableHead>
              <TableHead>{t("admin.usersColEmail")}</TableHead>
              <TableHead>{t("admin.usersColCreated")}</TableHead>
              <TableHead>{t("admin.usersColMfa")}</TableHead>
              <TableHead className="text-right">{t("admin.usersColActions")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {users === null ? null : users.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} className="py-6 text-center text-muted-foreground">
                  {t("admin.usersEmpty")}
                </TableCell>
              </TableRow>
            ) : (
              users.map((u) => (
                <UserRow key={u.id} user={u} onChanged={reload} />
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

export function UserRow({ user, onChanged }: { user: AdminUser; onChanged: () => void }) {
  const { t } = useI18n();
  const [resetOpen, setResetOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const isTemplate = user.username === "template";

  return (
    <TableRow>
      <TableCell className="font-medium">
        <span className="flex items-center gap-2">
          {user.username}
          {user.banned && <Badge variant="destructive">{t("admin.banned")}</Badge>}
        </span>
      </TableCell>
      <TableCell>{user.email || "—"}</TableCell>
      <TableCell>{new Date(user.created_at).toLocaleDateString()}</TableCell>
      <TableCell>
        <Badge variant={user.mfa_enabled ? "default" : "secondary"}>
          {user.mfa_enabled ? t("admin.on") : t("admin.off")}
        </Badge>
      </TableCell>
      <TableCell className="text-right">
        <div className="flex justify-end gap-1">
          {isTemplate ? (
            <Badge variant="outline">{t("admin.templateProtected")}</Badge>
          ) : (
            <BanButton
              path={`/users/${encodeURIComponent(user.username)}/ban`}
              banned={user.banned}
              name={user.username}
              onChanged={onChanged}
            />
          )}
          <Button variant="ghost" size="sm" className="gap-1" onClick={() => setResetOpen(true)}>
            <KeyRound className="h-4 w-4" />
            {t("admin.resetPassword")}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="gap-1 text-destructive hover:text-destructive"
            disabled={isTemplate}
            onClick={() => setDeleteOpen(true)}
          >
            <Trash2 className="h-4 w-4" />
            {t("admin.deleteUser")}
          </Button>
        </div>
      </TableCell>
      <ResetPasswordDialog user={user.username} open={resetOpen} onOpenChange={setResetOpen} onDone={onChanged} />
      <DeleteUserDialog user={user.username} open={deleteOpen} onOpenChange={setDeleteOpen} onDone={onChanged} />
    </TableRow>
  );
}

export function ResetPasswordDialog({
  user,
  open,
  onOpenChange,
  onDone,
}: {
  user: string;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onDone: () => void;
}) {
  const { t, to } = useI18n();
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const submit = async () => {
    setBusy(true);
    setError("");
    const r = await adminUserReq(`/users/${encodeURIComponent(user)}/reset_password`, "POST", { password });
    setBusy(false);
    if (!r.ok) {
      setError(userActionError(to, r, "admin.passwordReset"));
      return;
    }
    toast.success(t("admin.passwordReset", { name: user }));
    setPassword("");
    onOpenChange(false);
    onDone();
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>{t("admin.resetPasswordTitle", { name: user })}</DialogTitle>
        </DialogHeader>
        <div className="grid gap-3">
          <div className="grid gap-2">
            <Label htmlFor={`reset-${user}`}>{t("admin.resetPasswordNew")}</Label>
            <Input
              id={`reset-${user}`}
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="new-password"
            />
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
        </div>
        <DialogFooter>
          <Button onClick={submit} disabled={busy || password.length < 8}>
            {t("admin.resetPasswordConfirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function DeleteUserDialog({
  user,
  open,
  onOpenChange,
  onDone,
}: {
  user: string;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onDone: () => void;
}) {
  const { t, to } = useI18n();
  const [busy, setBusy] = useState(false);

  const confirm = async () => {
    setBusy(true);
    const r = await adminUserReq(`/users/${encodeURIComponent(user)}`, "DELETE");
    setBusy(false);
    if (!r.ok) {
      toast.error(userActionError(to, r, "admin.userDeleted"));
    } else {
      toast.success(t("admin.userDeleted", { name: user }));
    }
    onOpenChange(false);
    onDone();
  };

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("admin.deleteUserTitle", { name: user })}</AlertDialogTitle>
          <AlertDialogDescription>{t("admin.deleteUserHint")}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={busy} />
          <AlertDialogAction onClick={confirm} disabled={busy}>
            {t("admin.deleteUser")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

