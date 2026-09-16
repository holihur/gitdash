import { useEffect, useState } from "react";
import { ShieldCheck, ShieldOff } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { adminReq, type Settings } from "./api";

export function GithubSettings({ settings, onChange }: { settings: Settings | null; onChange: () => void }) {
  const { t, to } = useI18n();
  const [enabled, setEnabled] = useState(false);
  const [clientId, setClientId] = useState("");
  const [secret, setSecret] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  useEffect(() => {
    if (settings) {
      setEnabled(settings.github_oauth_enabled);
      setClientId(settings.github_client_id);
      setSecret("");
    }
  }, [settings]);

  const save = async () => {
    setBusy(true);
    setMsg("");
    try {
      await adminReq("/settings", {
        github_oauth_enabled: enabled,
        github_client_id: clientId,
        github_client_secret: secret || undefined,
      });
      setMsg(t("admin.saved"));
      setSecret("");
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
          {enabled ? <ShieldCheck className="h-4 w-4 text-green-600" /> : <ShieldOff className="h-4 w-4" />}
          {t("admin.githubTitle")}
          <Badge variant={enabled ? "default" : "secondary"}>{enabled ? t("admin.on") : t("admin.off")}</Badge>
        </CardTitle>
        <CardDescription>{t("admin.githubHint")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} className="h-4 w-4" />
          {t("admin.githubEnable")}
        </label>
        <div className="grid gap-2">
          <Label htmlFor="gh-id">{t("admin.clientId")}</Label>
          <Input id="gh-id" value={clientId} onChange={(e) => setClientId(e.target.value)} autoComplete="off" />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="gh-secret">
            {t("admin.clientSecret")}
            {settings?.github_has_secret && `（${t("admin.secretSet")}）`}
          </Label>
          <Input id="gh-secret" type="password" value={secret} onChange={(e) => setSecret(e.target.value)} autoComplete="new-password" />
        </div>
        <p className="text-xs text-muted-foreground">
          {t("admin.callbackLabel")}:{" "}
          <code className="break-all">{window.location.origin}/api/auth/github/callback</code>
        </p>
        {msg && <p className="text-sm text-muted-foreground">{msg}</p>}
        <div>
          <Button onClick={save} disabled={busy || !clientId}>
            {t("admin.save")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

export function GoogleSettings({ settings, onChange }: { settings: Settings | null; onChange: () => void }) {
  const { t, to } = useI18n();
  const [enabled, setEnabled] = useState(false);
  const [clientId, setClientId] = useState("");
  const [secret, setSecret] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  useEffect(() => {
    if (settings) {
      setEnabled(settings.google_oauth_enabled);
      setClientId(settings.google_client_id);
      setSecret("");
    }
  }, [settings]);

  const save = async () => {
    setBusy(true);
    setMsg("");
    try {
      await adminReq("/settings", {
        google_oauth_enabled: enabled,
        google_client_id: clientId,
        google_client_secret: secret || undefined,
      });
      setMsg(t("admin.saved"));
      setSecret("");
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
          {enabled ? <ShieldCheck className="h-4 w-4 text-green-600" /> : <ShieldOff className="h-4 w-4" />}
          {t("admin.googleTitle")}
          <Badge variant={enabled ? "default" : "secondary"}>{enabled ? t("admin.on") : t("admin.off")}</Badge>
        </CardTitle>
        <CardDescription>{t("admin.googleHint")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} className="h-4 w-4" />
          {t("admin.googleEnable")}
        </label>
        <div className="grid gap-2">
          <Label htmlFor="g-id">{t("admin.clientId")}</Label>
          <Input id="g-id" value={clientId} onChange={(e) => setClientId(e.target.value)} autoComplete="off" />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="g-secret">
            {t("admin.clientSecret")}
            {settings?.google_has_secret && `（${t("admin.secretSet")}）`}
          </Label>
          <Input id="g-secret" type="password" value={secret} onChange={(e) => setSecret(e.target.value)} autoComplete="new-password" />
        </div>
        <p className="text-xs text-muted-foreground">
          {t("admin.callbackLabel")}:{" "}
          <code className="break-all">{window.location.origin}/api/auth/google/callback</code>
        </p>
        {msg && <p className="text-sm text-muted-foreground">{msg}</p>}
        <div>
          <Button onClick={save} disabled={busy || !clientId}>
            {t("admin.save")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

export function OidcSettings({ settings, onChange }: { settings: Settings | null; onChange: () => void }) {
  const { t, to } = useI18n();
  const [enabled, setEnabled] = useState(false);
  const [name, setName] = useState("");
  const [issuer, setIssuer] = useState("");
  const [clientId, setClientId] = useState("");
  const [secret, setSecret] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  useEffect(() => {
    if (settings) {
      setEnabled(settings.oidc_enabled);
      setName(settings.oidc_name);
      setIssuer(settings.oidc_issuer);
      setClientId(settings.oidc_client_id);
      setSecret("");
    }
  }, [settings]);

  const save = async () => {
    setBusy(true);
    setMsg("");
    try {
      await adminReq("/settings", {
        oidc_enabled: enabled,
        oidc_name: name,
        oidc_issuer: issuer,
        oidc_client_id: clientId,
        oidc_client_secret: secret || undefined,
      });
      setMsg(t("admin.saved"));
      setSecret("");
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
          {enabled ? <ShieldCheck className="h-4 w-4 text-green-600" /> : <ShieldOff className="h-4 w-4" />}
          {t("admin.oidcTitle")}
          <Badge variant={enabled ? "default" : "secondary"}>{enabled ? t("admin.on") : t("admin.off")}</Badge>
        </CardTitle>
        <CardDescription>{t("admin.oidcHint")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} className="h-4 w-4" />
          {t("admin.oidcEnable")}
        </label>
        <div className="grid gap-2">
          <Label htmlFor="oidc-name">{t("admin.oidcName")}</Label>
          <Input id="oidc-name" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="oidc-issuer">{t("admin.oidcIssuer")}</Label>
          <Input id="oidc-issuer" value={issuer} onChange={(e) => setIssuer(e.target.value)} placeholder="https://gitlab.com" autoComplete="off" />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="oidc-id">{t("admin.clientId")}</Label>
          <Input id="oidc-id" value={clientId} onChange={(e) => setClientId(e.target.value)} autoComplete="off" />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="oidc-secret">
            {t("admin.clientSecret")}
            {settings?.oidc_has_secret && `（${t("admin.secretSet")}）`}
          </Label>
          <Input id="oidc-secret" type="password" value={secret} onChange={(e) => setSecret(e.target.value)} autoComplete="new-password" />
        </div>
        <p className="text-xs text-muted-foreground">
          {t("admin.callbackLabel")}:{" "}
          <code className="break-all">{window.location.origin}/api/auth/oidc/callback</code>
        </p>
        {msg && <p className="text-sm text-muted-foreground">{msg}</p>}
        <div>
          <Button onClick={save} disabled={busy || !issuer || !clientId}>
            {t("admin.save")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

