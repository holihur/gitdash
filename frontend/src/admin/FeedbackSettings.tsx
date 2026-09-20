import { useEffect, useState } from "react";
import { MessageSquareOff, MessageSquarePlus } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { adminReq, type Settings } from "./api";

export function FeedbackSettings({ settings, onChange }: { settings: Settings | null; onChange: () => void }) {
  const { t, to } = useI18n();
  const [enabled, setEnabled] = useState(false);
  const [repo, setRepo] = useState("");
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  useEffect(() => {
    if (settings) {
      setEnabled(!!settings.feedback_enabled);
      setRepo(settings.feedback_repo ?? "");
      setToken("");
    }
  }, [settings]);

  const save = async () => {
    setBusy(true);
    setMsg("");
    try {
      await adminReq("/settings", {
        feedback_enabled: enabled,
        feedback_repo: repo,
        feedback_token: token || undefined,
      });
      setMsg(t("admin.saved"));
      setToken("");
      onChange();
    } catch (e) {
      setMsg(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          {enabled ? (
            <MessageSquarePlus className="h-4 w-4 text-green-600" />
          ) : (
            <MessageSquareOff className="h-4 w-4" />
          )}
          {t("admin.feedbackTitle")}
          <Badge variant={enabled ? "default" : "secondary"}>{enabled ? t("admin.on") : t("admin.off")}</Badge>
        </CardTitle>
        <CardDescription>{t("admin.feedbackHint")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} className="h-4 w-4" />
          {t("admin.feedbackEnable")}
        </label>
        <div className="grid gap-2">
          <Label htmlFor="feedback-repo">{t("admin.feedbackRepo")}</Label>
          <Input
            id="feedback-repo"
            value={repo}
            onChange={(e) => setRepo(e.target.value)}
            placeholder={t("admin.feedbackRepoPlaceholder")}
            autoComplete="off"
          />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="feedback-token">
            {t("admin.feedbackToken")}
            {settings?.feedback_has_token && `（${t("admin.secretSet")}）`}
          </Label>
          <Input
            id="feedback-token"
            type="password"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            autoComplete="new-password"
          />
        </div>
        {msg && <p className="text-sm text-muted-foreground">{msg}</p>}
        <div>
          <Button
            onClick={save}
            disabled={busy || (enabled && (!repo || (!token && !settings?.feedback_has_token)))}
          >
            {t("admin.save")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
