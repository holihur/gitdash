import { useEffect, useState } from "react";
import { Link2, Link2Off } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { adminReq, type Settings } from "./api";

interface BindingCardProps {
  settings: Settings | null;
  onChange: () => void;
  provider: "gitlab" | "gitea" | "bitbucket";
  titleKey: string;
  hintKey: string;
  enableKey: string;
  needsBaseURL?: boolean;
}

function BindingCard({ settings, onChange, provider, titleKey, hintKey, enableKey, needsBaseURL }: BindingCardProps) {
  const { t, to } = useI18n();
  const [enabled, setEnabled] = useState(false);
  const [clientId, setClientId] = useState("");
  const [secret, setSecret] = useState("");
  const [baseURL, setBaseURL] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  const keys = {
    gitlab: {
      enabled: "gitlab_enabled",
      id: "gitlab_client_id",
      secret: "gitlab_client_secret",
      hasSecret: "gitlab_has_secret",
      base: "gitlab_base_url",
    },
    gitea: {
      enabled: "gitea_enabled",
      id: "gitea_client_id",
      secret: "gitea_client_secret",
      hasSecret: "gitea_has_secret",
      base: "gitea_base_url",
    },
    bitbucket: {
      enabled: "bitbucket_enabled",
      id: "bitbucket_client_id",
      secret: "bitbucket_client_secret",
      hasSecret: "bitbucket_has_secret",
      base: "",
    },
  }[provider];

  useEffect(() => {
    if (settings) {
      setEnabled(Boolean((settings as unknown as Record<string, unknown>)[keys.enabled]));
      setClientId(String((settings as unknown as Record<string, unknown>)[keys.id] ?? ""));
      setBaseURL(keys.base ? String((settings as unknown as Record<string, unknown>)[keys.base] ?? "") : "");
      setSecret("");
    }
    // settings 变化时同步表单
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [settings, provider]);

  const hasSecret = Boolean(settings && (settings as unknown as Record<string, unknown>)[keys.hasSecret]);

  const save = async () => {
    setBusy(true);
    setMsg("");
    try {
      const body: Record<string, unknown> = {
        [keys.enabled]: enabled,
        [keys.id]: clientId,
      };
      if (keys.base) body[keys.base] = baseURL;
      if (secret) body[keys.secret] = secret;
      await adminReq("/settings", body);
      setMsg(t("admin.saved"));
      setSecret("");
      onChange();
    } catch (e) {
      setMsg(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const disabled = busy || (enabled && (!clientId || (needsBaseURL && !baseURL) || (!secret && !hasSecret)));

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          {enabled ? <Link2 className="h-4 w-4 text-green-600" /> : <Link2Off className="h-4 w-4" />}
          {t(titleKey)}
          <Badge variant={enabled ? "default" : "secondary"}>{enabled ? t("admin.on") : t("admin.off")}</Badge>
        </CardTitle>
        <CardDescription>{t(hintKey)}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} className="h-4 w-4" />
          {t(enableKey)}
        </label>
        {needsBaseURL && (
          <div className="grid gap-2">
            <Label htmlFor={`${provider}-base`}>{t("admin.baseUrl")}</Label>
            <Input
              id={`${provider}-base`}
              value={baseURL}
              onChange={(e) => setBaseURL(e.target.value)}
              placeholder={provider === "gitlab" ? "https://gitlab.com" : "https://gitea.example.com"}
              autoComplete="off"
            />
          </div>
        )}
        <div className="grid gap-2">
          <Label htmlFor={`${provider}-id`}>{t("admin.clientId")}</Label>
          <Input id={`${provider}-id`} value={clientId} onChange={(e) => setClientId(e.target.value)} autoComplete="off" />
        </div>
        <div className="grid gap-2">
          <Label htmlFor={`${provider}-secret`}>
            {t("admin.clientSecret")}
            {hasSecret && `（${t("admin.secretSet")}）`}
          </Label>
          <Input
            id={`${provider}-secret`}
            type="password"
            value={secret}
            onChange={(e) => setSecret(e.target.value)}
            autoComplete="new-password"
          />
        </div>
        <p className="text-xs text-muted-foreground">
          {t("admin.callbackLabel")}:{" "}
          <code className="break-all">{window.location.origin}/api/connections/{provider}/callback</code>
        </p>
        {msg && <p className="text-sm text-muted-foreground">{msg}</p>}
        <div>
          <Button onClick={save} disabled={disabled}>
            {t("admin.save")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

/** 第三方账号绑定（批量导入）的 admin 配置：GitLab / Gitea / Bitbucket。 */
export function BindingSettings({ settings, onChange }: { settings: Settings | null; onChange: () => void }) {
  return (
    <>
      <BindingCard
        settings={settings}
        onChange={onChange}
        provider="gitlab"
        titleKey="admin.gitlabBindTitle"
        hintKey="admin.gitlabBindHint"
        enableKey="admin.gitlabEnable"
        needsBaseURL
      />
      <BindingCard
        settings={settings}
        onChange={onChange}
        provider="gitea"
        titleKey="admin.giteaBindTitle"
        hintKey="admin.giteaBindHint"
        enableKey="admin.giteaEnable"
        needsBaseURL
      />
      <BindingCard
        settings={settings}
        onChange={onChange}
        provider="bitbucket"
        titleKey="admin.bitbucketBindTitle"
        hintKey="admin.bitbucketBindHint"
        enableKey="admin.bitbucketEnable"
      />
    </>
  );
}
