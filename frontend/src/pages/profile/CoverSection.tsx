import { useState } from "react";
import { toast } from "sonner";
import { ImagePlus } from "lucide-react";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { CoverBanner } from "@/components/cover-banner";

export function CoverSection({
  coverUrl,
  onChanged,
}: {
  coverUrl?: string;
  onChanged: () => void;
}) {
  const { t, to } = useI18n();
  const [version, setVersion] = useState(0);
  const [busy, setBusy] = useState(false);

  const upload = async (file: File) => {
    setBusy(true);
    try {
      await api.uploadCover(file);
      setVersion(Date.now());
      onChanged();
      toast.success(t("cover.updated"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    setBusy(true);
    try {
      await api.deleteCover();
      setVersion(Date.now());
      onChanged();
      toast.success(t("cover.removed"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <ImagePlus className="h-4 w-4" />
          {t("cover.title")}
        </CardTitle>
        <CardDescription>{t("cover.hint")}</CardDescription>
      </CardHeader>
      <CardContent>
        <CoverBanner
          coverUrl={coverUrl}
          version={version}
          editable
          busy={busy}
          onPick={upload}
          onRemove={remove}
        />
      </CardContent>
    </Card>
  );
}
