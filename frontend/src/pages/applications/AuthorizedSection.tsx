import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { KeyRound } from "lucide-react";
import { api, type OAuthAuthorization } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import ConfirmDialog from "@/components/confirm-dialog";
import { formatDate } from "@/lib/utils";
import { apiErrorMsg } from "@/lib/errors";

export function AuthorizedSection({ to, locale }: { to: (k: string) => string | undefined; locale: string }) {
  const [auths, setAuths] = useState<OAuthAuthorization[]>([]);
  const [loading, setLoading] = useState(true);
  const [pendingRevoke, setPendingRevoke] = useState<OAuthAuthorization | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setAuths(await api.listAuthorizations());
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setLoading(false);
    }
  }, [to]);

  useEffect(() => {
    load();
  }, [load]);

  const revoke = async (auth: OAuthAuthorization) => {
    setPendingRevoke(null);
    try {
      await api.revokeAuthorization(auth.id);
      toast.success(`Revoked access for "${auth.app_name}"`);
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  if (loading) {
    return (
      <div className="space-y-3 rounded-lg border p-4">
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} className="h-6 w-full" />
        ))}
      </div>
    );
  }
  if (auths.length === 0) {
    return (
      <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed py-16 px-4 text-center">
        <KeyRound className="h-10 w-10 text-muted-foreground" />
        <p className="font-medium">No authorized applications</p>
        <p className="text-sm text-muted-foreground">
          Applications you authorize will appear here so you can revoke access at any time.
        </p>
      </div>
    );
  }
  return (
    <div className="space-y-6">
      <div className="overflow-x-auto rounded-lg border">
        <Table className="min-w-[680px]">
          <TableHeader>
            <TableRow>
              <TableHead>Application</TableHead>
              <TableHead>Scopes</TableHead>
              <TableHead>Authorized</TableHead>
              <TableHead>Last used</TableHead>
              <TableHead className="w-24" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {auths.map((auth) => (
              <TableRow key={auth.id}>
                <TableCell className="font-medium">{auth.app_name}</TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-1">
                    {auth.scopes.map((s) => (
                      <Badge key={s} variant="secondary">
                        {s}
                      </Badge>
                    ))}
                  </div>
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">
                  {formatDate(auth.created_at, locale)}
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">
                  {auth.last_used_at ? formatDate(auth.last_used_at, locale) : "—"}
                </TableCell>
                <TableCell>
                  <Button variant="ghost" size="sm" onClick={() => setPendingRevoke(auth)}>
                    Revoke
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      <ConfirmDialog
        open={pendingRevoke !== null}
        onOpenChange={(o) => !o && setPendingRevoke(null)}
        title="Revoke access?"
        description={`The access token issued to "${pendingRevoke?.app_name ?? ""}" will stop working immediately.`}
        confirmText="Revoke"
        onConfirm={() => pendingRevoke && revoke(pendingRevoke)}
      />
    </div>
  );
}

