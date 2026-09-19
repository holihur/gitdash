import { useEffect, useState } from "react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Textarea } from "@/components/ui/textarea";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

/** 个人简介（显示在用户主页）。 */
export function BioSection() {
  const { t, to } = useI18n();
  const [bio, setBio] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    api
      .me()
      .then((m) => setBio(m.bio ?? ""))
      .catch(() => undefined)
      .finally(() => setLoaded(true));
  }, []);

  const save = async () => {
    setSaving(true);
    try {
      await api.setBio(bio.trim());
      toast.success(t("profile.bioSaved"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("profile.bio")}</CardTitle>
        <CardDescription>{t("profile.bioDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3">
        <Textarea
          value={bio}
          onChange={(e) => setBio(e.target.value)}
          rows={3}
          maxLength={500}
          placeholder={t("profile.bioPlaceholder")}
          disabled={!loaded}
        />
        <div className="flex justify-end">
          <Button size="sm" onClick={() => void save()} disabled={saving || !loaded}>
            {t("common.save")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
