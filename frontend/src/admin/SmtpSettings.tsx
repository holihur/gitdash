import { useEffect, useState } from "react";
import { Mail, MailCheck, MailX } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { adminReq, type Settings } from "./api";

export function SmtpSettings({ settings, onChange }: { settings: Settings | null; onChange: () => void }) {
  const { t, to } = useI18n();
  const [enabled, setEnabled] = useState(false);
  const [host, setHost] = useState("");
  const [port, setPort] = useState("");
  const [user, setUser] = useState("");
  const [pass, setPass] = useState("");
  const [from, setFrom] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  const [testTo, setTestTo] = useState("");
  const [testBusy, setTestBusy] = useState(false);
  const [testMsg, setTestMsg] = useState("");
  const [testFailed, setTestFailed] = useState(false);

  useEffect(() => {
    if (settings) {
      setEnabled(settings.smtp_enabled);
      setHost(settings.smtp_host);
      setPort(settings.smtp_port);
      setUser(settings.smtp_user);
      setFrom(settings.smtp_from);
      setPass("");
    }
  }, [settings]);

  const save = async () => {
    setBusy(true);
    setMsg("");
    try {
      await adminReq("/settings", {
        smtp_enabled: enabled,
        smtp_host: host,
        smtp_port: port,
        smtp_user: user,
        smtp_from: from,
        smtp_pass: pass || undefined,
      });
      setMsg(t("admin.saved"));
      setPass("");
      onChange();
    } catch (e) {
      setMsg(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const sendTest = async () => {
    setTestBusy(true);
    setTestMsg("");
    setTestFailed(false);
    try {
      await adminReq("/smtp/test", { to: testTo });
      setTestMsg(t("admin.smtpTestSent"));
    } catch (e) {
      setTestFailed(true);
      setTestMsg(apiErrorMsg(to, e));
    } finally {
      setTestBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          {enabled ? <Mail className="h-4 w-4 text-green-600" /> : <MailX className="h-4 w-4" />}
          {t("admin.smtpTitle")}
          <Badge variant={enabled ? "default" : "secondary"}>{enabled ? t("admin.on") : t("admin.off")}</Badge>
        </CardTitle>
        <CardDescription>{t("admin.smtpHint")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} className="h-4 w-4" />
          {t("admin.smtpEnable")}
        </label>
        <div className="grid gap-4 sm:grid-cols-[2fr_1fr]">
          <div className="grid gap-2">
            <Label htmlFor="smtp-host">{t("admin.smtpHost")}</Label>
            <Input id="smtp-host" value={host} onChange={(e) => setHost(e.target.value)} placeholder="smtp.example.com" autoComplete="off" />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="smtp-port">{t("admin.smtpPort")}</Label>
            <Input id="smtp-port" value={port} onChange={(e) => setPort(e.target.value)} placeholder="587" inputMode="numeric" autoComplete="off" />
          </div>
        </div>
        <div className="grid gap-2">
          <Label htmlFor="smtp-user">{t("admin.smtpUser")}</Label>
          <Input id="smtp-user" value={user} onChange={(e) => setUser(e.target.value)} autoComplete="off" />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="smtp-pass">
            {t("admin.smtpPass")}
            {settings?.smtp_has_pass && `（${t("admin.secretSet")}）`}
          </Label>
          <Input id="smtp-pass" type="password" value={pass} onChange={(e) => setPass(e.target.value)} autoComplete="new-password" />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="smtp-from">{t("admin.smtpFrom")}</Label>
          <Input id="smtp-from" value={from} onChange={(e) => setFrom(e.target.value)} placeholder="gitdash@example.com" autoComplete="off" />
        </div>
        {msg && <p className="text-sm text-muted-foreground">{msg}</p>}
        <div>
          <Button onClick={save} disabled={busy || (enabled && !host)}>
            {t("admin.save")}
          </Button>
        </div>

        <div className="mt-2 grid gap-2 border-t pt-4">
          <Label htmlFor="smtp-test-to" className="flex items-center gap-2">
            <MailCheck className="h-4 w-4" />
            {t("admin.smtpTestTitle")}
          </Label>
          <div className="flex flex-col gap-2 sm:flex-row">
            <Input
              id="smtp-test-to"
              type="email"
              value={testTo}
              onChange={(e) => setTestTo(e.target.value)}
              placeholder={t("admin.smtpTestTo")}
              className="sm:flex-1"
            />
            <Button variant="outline" onClick={sendTest} disabled={testBusy || !testTo}>
              {t("admin.smtpTestSend")}
            </Button>
          </div>
          {testMsg && (
            <p className={`text-sm ${testFailed ? "text-destructive" : "text-muted-foreground"}`}>{testMsg}</p>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
