import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { dateLocale, useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { formatDate } from "@/lib/utils";
import { EmailSection } from "./profile/EmailSection";
import { BioSection } from "./profile/BioSection";
import { BadgeDisplayPicker } from "@/components/badge-display-picker";
import { AvatarSection } from "./profile/AvatarSection";
import { CoverSection } from "./profile/CoverSection";
import { ByokSection } from "./profile/ByokSection";
import { PasswordSection } from "./profile/PasswordSection";
import { MFASection } from "./profile/MFASection";
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

export default function ProfilePage() {
  const { t, to, lang } = useI18n();
  const locale = dateLocale(lang);
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

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("profile.title")}</h1>
        <p className="text-sm text-muted-foreground">
          {t("profile.memberSince", { date: formatDate(profile.created_at, locale) })}
        </p>
      </div>

      <EmailSection
        email={profile.email ?? ""}
        verified={profile.email_verified ?? false}
        onChanged={loadProfile}
      />
      <AvatarSection username={profile.username} />
      <CoverSection coverUrl={profile.cover_url} onChanged={loadProfile} />
      <BadgeDisplayPicker kind="user" owner={profile.username} />
      <BioSection />
      <ByokSection />
      <PasswordSection />
      <MFASection mfaEnabled={profile.mfa_enabled} onChanged={loadProfile} />
      <GPGKeySection />
      <ConnectionsSection />
      <RunnersSection />
      <DangerZone />
    </div>
  );
}

