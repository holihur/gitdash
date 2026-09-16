import { useEffect, useState } from "react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export function PipelineCard({ owner, name }: { owner: string; name: string }) {
  const { t, to } = useI18n();
  const [enabled, setEnabled] = useState<boolean | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api
      .getPipeline(owner, name)
      .then((p) => setEnabled(p.enabled))
      .catch(() => setEnabled(false));
  }, [owner, name]);

  const toggle = async () => {
    setBusy(true);
    try {
      const res = await api.setPipeline(owner, name, !enabled);
      setEnabled(res.enabled);
      toast.success(t(res.enabled ? "pipeline.enabled" : "pipeline.disabled"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("pipeline.title")}</CardTitle>
        <CardDescription>{t("pipeline.hint")}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex flex-wrap items-center gap-3">
          <Badge variant={enabled ? "secondary" : "outline"}>
            {t(enabled ? "pipeline.statusOn" : "pipeline.statusOff")}
          </Badge>
          <Button size="sm" variant="outline" disabled={busy || enabled === null} onClick={toggle}>
            {t(enabled ? "pipeline.turnOff" : "pipeline.turnOn")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
