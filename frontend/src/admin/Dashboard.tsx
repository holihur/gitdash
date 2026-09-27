import { useCallback, useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { FileText, KeyRound, LayoutDashboard, LogOut, Server, ShieldAlert } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { SettingsLayout, type SettingsNavItem } from "@/components/settings-layout";
import { adminReq, toastError, type Settings } from "./api";
import { GithubSettings, GoogleSettings, OidcSettings } from "./OAuthSettings";
import { BindingSettings } from "./BindingSettings";
import { AccessSettings } from "./AccessSettings";
import { LanguageSettings } from "./LanguageSettings";
import { DocsSettings } from "./DocsSettings";
import { AnnouncementSettings } from "./AnnouncementSettings";
import { SmtpSettings } from "./SmtpSettings";
import { FeedbackSettings } from "./FeedbackSettings";
import { PasswordCard } from "./PasswordCard";
import { UsersSection } from "./sections/UsersSection";
import { ReposSection } from "./sections/ReposSection";
import { OrgsSection } from "./sections/OrgsSection";
import { IPBlacklistSection } from "./sections/IPBlacklistSection";
import { QuotaSection } from "./sections/QuotaSection";
import { CodeSearchSection } from "./sections/CodeSearchSection";
import { BadgesSection } from "./sections/BadgesSection";
import { ReservedNamesSection } from "./sections/ReservedNamesSection";

const SECTIONS = ["general", "authentication", "content", "moderation", "system"] as const;
type Section = (typeof SECTIONS)[number];

export function Dashboard({ user, onLogout }: { user: string; onLogout: () => void }) {
  const { t, to } = useI18n();
  const nav = useNavigate();
  const { section: rawSection } = useParams();
  const section: Section = (SECTIONS as readonly string[]).includes(rawSection ?? "")
    ? (rawSection as Section)
    : "general";
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

  const items: SettingsNavItem[] = [
    {
      key: "general",
      label: t("settingsNav.general"),
      icon: LayoutDashboard,
      content: (
        <>
          <AccessSettings settings={settings} onChange={load} />
          <LanguageSettings />
          <AnnouncementSettings settings={settings} onChange={load} />
        </>
      ),
    },
    {
      key: "authentication",
      label: t("settingsNav.authentication"),
      icon: KeyRound,
      content: (
        <>
          <GithubSettings settings={settings} onChange={load} />
          <GoogleSettings settings={settings} onChange={load} />
          <OidcSettings settings={settings} onChange={load} />
          <BindingSettings settings={settings} onChange={load} />
          <SmtpSettings settings={settings} onChange={load} />
        </>
      ),
    },
    {
      key: "content",
      label: t("settingsNav.content"),
      icon: FileText,
      content: (
        <>
          <DocsSettings settings={settings} onChange={load} />
          <FeedbackSettings settings={settings} onChange={load} />
        </>
      ),
    },
    {
      key: "moderation",
      label: t("settingsNav.moderation"),
      icon: ShieldAlert,
      content: (
        <>
          <UsersSection />
          <ReposSection />
          <OrgsSection />
          <BadgesSection />
          <ReservedNamesSection />
        </>
      ),
    },
    {
      key: "system",
      label: t("settingsNav.system"),
      icon: Server,
      content: (
        <>
          <CodeSearchSection />
          <IPBlacklistSection />
          <QuotaSection />
          <PasswordCard />
        </>
      ),
    },
  ];

  return (
    <SettingsLayout
      title={t("admin.title")}
      description={t("settingsNav.adminSubtitle")}
      actions={
        <div className="flex shrink-0 items-center gap-2">
          <span className="hidden text-xs text-muted-foreground sm:inline">
            {t("admin.signedInAs", { user })}
          </span>
          <Button variant="outline" size="sm" className="gap-2" onClick={logout}>
            <LogOut className="h-4 w-4" />
            {t("admin.signOut")}
          </Button>
        </div>
      }
      items={items}
      active={section}
      onSelect={(key) => nav(key === "general" ? "/" : `/${key}`)}
    />
  );
}
