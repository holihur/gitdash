import { useCallback, useEffect, useState } from "react";
import { LogOut } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { adminReq, toastError, type Settings } from "./api";
import { GithubSettings, GoogleSettings, OidcSettings } from "./OAuthSettings";
import { BindingSettings } from "./BindingSettings";
import { DocsSettings } from "./DocsSettings";
import { SmtpSettings } from "./SmtpSettings";
import { FeedbackSettings } from "./FeedbackSettings";
import { PasswordCard } from "./PasswordCard";
import { UsersSection } from "./sections/UsersSection";
import { ReposSection } from "./sections/ReposSection";
import { OrgsSection } from "./sections/OrgsSection";
import { IPBlacklistSection } from "./sections/IPBlacklistSection";
import { QuotaSection } from "./sections/QuotaSection";

export function Dashboard({ user, onLogout }: { user: string; onLogout: () => void }) {
  const { t, to } = useI18n();
  const [settings, setSettings] = useState<Settings | null>(null);

  const load = useCallback(async () => {
    try {
      setSettings(await adminReq<Settings>("/settings"));
    } catch (e) {
      toastError(to, e);
    }
  }, [to]);

  useEffect(() => {
    void load();
  }, [load]);

  const logout = async () => {
    try {
      await adminReq("/logout", {});
    } catch {
      /* ignore */
    }
    onLogout();
  };

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold">{t("admin.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("admin.signedInAs", { user })}</p>
        </div>
        <Button variant="outline" size="sm" className="gap-2" onClick={logout}>
          <LogOut className="h-4 w-4" />
          {t("admin.signOut")}
        </Button>
      </div>

      <GithubSettings settings={settings} onChange={load} />
      <GoogleSettings settings={settings} onChange={load} />
      <OidcSettings settings={settings} onChange={load} />
      <BindingSettings settings={settings} onChange={load} />
      <DocsSettings settings={settings} onChange={load} />
      <SmtpSettings settings={settings} onChange={load} />
      <FeedbackSettings settings={settings} onChange={load} />
      <PasswordCard />
      <UsersSection />
      <ReposSection />
      <OrgsSection />
      <IPBlacklistSection />
      <QuotaSection />
    </div>
  );
}

