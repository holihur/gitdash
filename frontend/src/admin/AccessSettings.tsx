import { useEffect, useState } from "react";
import { KeyRound } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { adminReq, type Settings } from "./api";

/**
 * 访问控制：开关账号密码登录与匿名 Swagger/OpenAPI 文档。
 * 两项默认开启（未设置时按开启处理），仅当显式保存为关闭后生效。
 */
export function AccessSettings({ settings, onChange }: { settings: Settings | null; onChange: () => void }) {
  const { t, to } = useI18n();
  const [passwordLogin, setPasswordLogin] = useState(true);
  const [swagger, setSwagger] = useState(true);
  const [versionVisible, setVersionVisible] = useState(true);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  useEffect(() => {
    if (settings) {
      setPasswordLogin(settings.password_login_enabled !== false);
      setSwagger(settings.swagger_enabled !== false);
      setVersionVisible(settings.version_visible !== false);
    }
  }, [settings]);

  const save = async () => {
    setBusy(true);
    setMsg("");
    try {
      await adminReq("/settings", {
        password_login_enabled: passwordLogin,
        swagger_enabled: swagger,
        version_visible: versionVisible,
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
          <KeyRound className="h-4 w-4" />
          {t("admin.accessTitle")}
        </CardTitle>
        <CardDescription>{t("admin.accessHint")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        <label className="flex items-start gap-3 text-sm">
          <input
            type="checkbox"
            checked={passwordLogin}
            onChange={(e) => setPasswordLogin(e.target.checked)}
            className="mt-0.5 h-4 w-4"
          />
          <span>
            <span className="font-medium">{t("admin.passwordLoginEnable")}</span>
            <span className="block text-xs text-muted-foreground">{t("admin.passwordLoginHint")}</span>
          </span>
        </label>
        <label className="flex items-start gap-3 text-sm">
          <input
            type="checkbox"
            checked={swagger}
            onChange={(e) => setSwagger(e.target.checked)}
            className="mt-0.5 h-4 w-4"
          />
          <span>
            <span className="font-medium">{t("admin.swaggerEnable")}</span>
            <span className="block text-xs text-muted-foreground">{t("admin.swaggerHint")}</span>
          </span>
        </label>
        <label className="flex items-start gap-3 text-sm">
          <input
            type="checkbox"
            checked={versionVisible}
            onChange={(e) => setVersionVisible(e.target.checked)}
            className="mt-0.5 h-4 w-4"
          />
          <span>
            <span className="font-medium">{t("admin.versionVisible")}</span>
            <span className="block text-xs text-muted-foreground">{t("admin.versionHint")}</span>
          </span>
        </label>
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
