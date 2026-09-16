import { useCallback, useEffect, useState } from "react";
import { GitBranch } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { Card, CardContent } from "@/components/ui/card";
import { adminReq, ApiDisabledError } from "./api";
import { ShellHeader } from "./ShellHeader";
import { LoginView } from "./LoginView";
import { Dashboard } from "./Dashboard";

export default function AdminApp() {
  const { t } = useI18n();
  const [me, setMe] = useState<string | null>(null);
  const [booted, setBooted] = useState(false);
  const [disabled, setDisabled] = useState(false);

  const boot = useCallback(async () => {
    try {
      const m = await adminReq<{ username: string }>("/me");
      setMe(m.username);
    } catch (e) {
      if (e instanceof ApiDisabledError) setDisabled(true);
      else setMe(null);
    } finally {
      setBooted(true);
    }
  }, []);

  useEffect(() => {
    boot();
  }, [boot]);

  if (!booted) return <p className="py-20 text-center text-muted-foreground">…</p>;

  if (disabled) {
    return (
      <ShellHeader>
        <Card className="mx-auto mt-16 max-w-md">
          <CardContent className="py-10 text-center">
            <GitBranch className="mx-auto mb-3 h-8 w-8 text-muted-foreground" />
            <p className="font-medium">{t("admin.disabledTitle")}</p>
            <p className="mt-2 text-sm text-muted-foreground">{t("admin.disabledHint")}</p>
          </CardContent>
        </Card>
      </ShellHeader>
    );
  }

  if (!me) {
    return (
      <ShellHeader>
        <LoginView onLogin={(u) => setMe(u)} />
      </ShellHeader>
    );
  }

  return <Dashboard user={me} onLogout={() => setMe(null)} />;
}

