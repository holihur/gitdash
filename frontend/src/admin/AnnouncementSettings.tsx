import { useEffect, useState } from "react";
import { Megaphone } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { adminReq, type Settings } from "./api";

/** 全站通知配置：发布的公告条展示在所有页面顶部，用户可关闭。 */
export function AnnouncementSettings({
  settings,
  onChange,
}: {
  settings: Settings | null;
  onChange: () => void;
}) {
  const { t, to } = useI18n();
  const [enabled, setEnabled] = useState(false);
  const [level, setLevel] = useState("info");
  const [title, setTitle] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  useEffect(() => {
    if (settings) {
      setEnabled(settings.announcement_enabled === true);
      setLevel(settings.announcement_level || "info");
      setTitle(settings.announcement_title ?? "");
      setMessage(settings.announcement_message ?? "");
    }
  }, [settings]);

  const save = async () => {
    setBusy(true);
    setMsg("");
    try {
      await adminReq("/settings", {
        announcement_enabled: enabled,
        announcement_level: level,
        announcement_title: title.trim(),
        announcement_message: message.trim(),
      });
      setMsg(t("admin.saved"));
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
          <Megaphone className="h-4 w-4" />
          {t("admin.announcementTitle")}
        </CardTitle>
        <CardDescription>{t("admin.announcementHint")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        <label className="flex items-start gap-3 text-sm">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
            className="mt-0.5 h-4 w-4"
          />
          <span>
            <span className="font-medium">{t("admin.announcementEnable")}</span>
            <span className="block text-xs text-muted-foreground">{t("admin.announcementEnableHint")}</span>
          </span>
        </label>
        <div className="grid gap-2">
          <Label htmlFor="announcement-level">{t("admin.announcementLevel")}</Label>
          <select
            id="announcement-level"
            value={level}
            onChange={(e) => setLevel(e.target.value)}
            className="h-10 w-full min-w-0 rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <option value="info">{t("admin.announcementLevelInfo")}</option>
            <option value="warning">{t("admin.announcementLevelWarning")}</option>
            <option value="critical">{t("admin.announcementLevelCritical")}</option>
          </select>
        </div>
        <div className="grid gap-2">
          <Label htmlFor="announcement-title">{t("admin.announcementTitleField")}</Label>
          <Input
            id="announcement-title"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            autoComplete="off"
          />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="announcement-message">{t("admin.announcementMessage")}</Label>
          <Textarea
            id="announcement-message"
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            className="min-h-[100px]"
          />
          <p className="text-xs text-muted-foreground">{t("admin.announcementMessageHint")}</p>
        </div>
        {msg && <p className="text-sm text-muted-foreground">{msg}</p>}
        <div>
          <Button onClick={save} disabled={busy}>
            {t("admin.save")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
