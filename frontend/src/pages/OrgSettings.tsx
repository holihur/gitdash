import { useCallback, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { toast } from "sonner";
import { ArrowLeft } from "lucide-react";
import { api, type OrgProfile } from "@/lib/api";
import { apiErrorMsg } from "@/lib/errors";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { CoverBanner } from "@/components/cover-banner";
import { BadgeDisplayPicker } from "@/components/badge-display-picker";

/** 组织设置独立页：编辑显示名 / 简介 / 封面（仅 owner）。 */
export default function OrgSettings() {
  const { org = "" } = useParams();
  const { t, to } = useI18n();
  const [profile, setProfile] = useState<OrgProfile | null>(null);
  const [error, setError] = useState("");
  const [display, setDisplay] = useState("");
  const [bio, setBio] = useState("");
  const [saving, setSaving] = useState(false);
  const [coverBusy, setCoverBusy] = useState(false);
  const [coverVersion, setCoverVersion] = useState(0);

  const load = useCallback(async () => {
    setError("");
    try {
      const p = await api.getOrgProfile(org);
      setProfile(p);
      setDisplay(p.display || "");
      setBio(p.bio || "");
    } catch (e) {
      setError(apiErrorMsg(to, e));
    }
  }, [org, to]);
  useEffect(() => {
    load();
  }, [load]);

  const isOwner = profile?.role === "owner";

  const save = async () => {
    if (!profile) return;
    setSaving(true);
    try {
      const o = await api.updateOrg(profile.name, { display, bio });
      setProfile({ ...profile, display: o.display, bio: o.bio ?? "" });
      toast.success(t("orgs.updated"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setSaving(false);
    }
  };

  const uploadCover = async (file: File) => {
    if (!profile) return;
    setCoverBusy(true);
    try {
      const r = await api.uploadOrgCover(profile.name, file);
      setProfile({ ...profile, cover_url: r.cover_url });
      setCoverVersion(Date.now());
      toast.success(t("cover.updated"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setCoverBusy(false);
    }
  };

  const removeCover = async () => {
    if (!profile) return;
    setCoverBusy(true);
    try {
      await api.deleteOrgCover(profile.name);
      setProfile({ ...profile, cover_url: undefined });
      setCoverVersion(Date.now());
      toast.success(t("cover.removed"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setCoverBusy(false);
    }
  };

  const backLink = (
    <Button asChild variant="ghost" size="sm" className="-ml-2 gap-1.5">
      <Link to={`/orgs/${encodeURIComponent(org)}`}>
        <ArrowLeft className="h-4 w-4" />
        {t("orgs.settingsBack")}
      </Link>
    </Button>
  );

  if (error) {
    return (
      <div className="space-y-4">
        {backLink}
        <Card className="border-destructive">
          <CardContent className="pt-6 text-sm text-destructive">{error}</CardContent>
        </Card>
      </div>
    );
  }
  if (!profile) return <p className="py-10 text-center text-sm text-muted-foreground">…</p>;

  if (!isOwner) {
    return (
      <div className="space-y-4">
        {backLink}
        <Card>
          <CardContent className="pt-6 text-sm text-muted-foreground">
            {t("orgs.ownerOnly")}
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-2xl space-y-4">
      {backLink}
      <h1 className="text-xl font-semibold">{t("orgs.settingsTitle")}</h1>

      <CoverBanner
        coverUrl={profile.cover_url}
        version={coverVersion}
        editable
        busy={coverBusy}
        onPick={uploadCover}
        onRemove={removeCover}
      />

      <Card>
        <CardContent className="grid gap-4 pt-6">
          <div className="grid gap-2">
            <label className="text-sm font-medium" htmlFor="org-settings-display">
              {t("orgs.displayLabel")}
            </label>
            <Input
              id="org-settings-display"
              value={display}
              onChange={(e) => setDisplay(e.target.value)}
              placeholder={profile.name}
            />
          </div>
          <div className="grid gap-2">
            <label className="text-sm font-medium" htmlFor="org-settings-bio">
              {t("orgs.bioLabel")}
            </label>
            <Textarea
              id="org-settings-bio"
              value={bio}
              onChange={(e) => setBio(e.target.value)}
              rows={4}
              maxLength={500}
              placeholder={t("orgs.bioPlaceholder")}
            />
          </div>
          <div className="flex justify-end gap-2">
            <Button asChild variant="outline" size="sm">
              <Link to={`/orgs/${encodeURIComponent(org)}`}>{t("common.cancel")}</Link>
            </Button>
            <Button size="sm" onClick={() => void save()} disabled={saving}>
              {t("common.save")}
            </Button>
          </div>
        </CardContent>
      </Card>

      <BadgeDisplayPicker kind="org" owner={profile.name} />
    </div>
  );
}
