import { useCallback, useEffect, useState } from "react";
import { Send } from "lucide-react";
import { toast } from "sonner";
import { api, type Webhook } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import WebhooksDialog from "@/components/webhooks-dialog";

/** 出站 Webhook 设置：复用 Repos 页的 WebhooksDialog 进行增删与投递记录查看。 */
export function OutgoingWebhooksCard({ owner, name }: { owner: string; name: string }) {
  const { t, to } = useI18n();
  const [hooks, setHooks] = useState<Webhook[]>([]);
  const [open, setOpen] = useState(false);

  const load = useCallback(async () => {
    try {
      setHooks(await api.listWebhooks(owner, name));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  }, [owner, name, to]);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Send className="h-4 w-4" />
          {t("repo.outgoingWebhook")}
        </CardTitle>
        <CardDescription>{t("repo.outgoingWebhookDesc")}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex flex-wrap items-center gap-3">
          <Badge variant={hooks.length > 0 ? "secondary" : "outline"}>
            {t("repo.outgoingWebhookCount", { count: hooks.length })}
          </Badge>
          <Button size="sm" variant="outline" onClick={() => setOpen(true)}>
            {t("webhooks.manage")}
          </Button>
        </div>
        <WebhooksDialog
          open={open}
          onOpenChange={(o) => {
            setOpen(o);
            if (!o) void load();
          }}
          owner={owner}
          repo={name}
        />
      </CardContent>
    </Card>
  );
}
