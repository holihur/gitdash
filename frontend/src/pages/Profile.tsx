import { useCallback, useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { toast } from "sonner";
import { Code2, Link2, ShieldCheck, TriangleAlert, User } from "lucide-react";
import { api } from "@/lib/api";
import { dateLocale, useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { formatDate } from "@/lib/utils";
import { SettingsLayout, type SettingsNavItem } from "@/components/settings-layout";
import { EmailSection } from "./profile/EmailSection";
import { BioSection } from "./profile/BioSection";
import { BadgeDisplayPicker } from "@/components/badge-display-picker";
import { AvatarSection } from "./profile/AvatarSection";
import { CoverSection } from "./profile/CoverSection";
import { ByokSection } from "./profile/ByokSection";
import { PasswordSection } from "./profile/PasswordSection";
import { MFASection } from "./profile/MFASection";
import { PasskeySection } from "./profile/PasskeySection";
import { GPGKeySection } from "./profile/GPGKeySection";
import { ConnectionsSection } from "./profile/ConnectionsSection";
import { RunnersSection } from "./profile/RunnersSection";
import { DangerZone } from "./profile/DangerZone";

interface Profile {
  username: string;
  email?: string;
  created_at: string;
  mfa_enabled: boolean;
  email_verified?: boolean;
  cover_url?: string;
}

const SECTIONS = ["account", "security", "developer", "connections", "danger"] as const;
type Section = (typeof SECTIONS)[number];

export default function ProfilePage() {
  const { t, to, lang } = useI18n();
  const nav = useNavigate();
  const { section: rawSection } = useParams();
  const section: Section = (SECTIONS as readonly string[]).includes(rawSection ?? "")
    ? (rawSection as Section)
    : "account";
  const [profile, setProfile] = useState<Profile | null>(null);

  const loadProfile = useCallback(async () => {
    try {
      setProfile(await api.me());
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  }, [to]);

  useEffect(() => {
    loadProfile();
  }, [loadProfile]);

  if (!profile) return <p className="py-10 text-center text-sm text-muted-foreground">…</p>;

  const items: SettingsNavItem[] = [
    {
      key: "account",
      label: t("settingsNav.account"),
      icon: User,
      content: (
        <>
          <EmailSection
            email={profile.email ?? ""}
            verified={profile.email_verified ?? false}
            onChanged={loadProfile}
          />
          <AvatarSection username={profile.username} />
          <CoverSection coverUrl={profile.cover_url} onChanged={loadProfile} />
          <BioSection />
          <BadgeDisplayPicker kind="user" owner={profile.username} />
        </>
      ),
    },
    {
      key: "security",
      label: t("settingsNav.security"),
      icon: ShieldCheck,
      content: (
        <>
          <PasswordSection />
          <MFASection mfaEnabled={profile.mfa_enabled} onChanged={loadProfile} />
          <PasskeySection />
        </>
      ),
    },
    {
      key: "developer",
      label: t("settingsNav.developer"),
      icon: Code2,
      content: (
        <>
          <ByokSection />
          <GPGKeySection />
          <RunnersSection />
        </>
      ),
    },
    {
      key: "connections",
      label: t("settingsNav.connections"),
      icon: Link2,
      content: <ConnectionsSection />,
    },
    {
      key: "danger",
      label: t("settingsNav.danger"),
      icon: TriangleAlert,
      content: <DangerZone />,
    },
  ];

  return (
    <SettingsLayout
      title={t("profile.title")}
      description={t("profile.memberSince", { date: formatDate(profile.created_at, dateLocale(lang)) })}
      items={items}
      active={section}
      onSelect={(key) => nav(key === "account" ? "/profile" : `/profile/${key}`)}
    />
  );
}
