import { useEffect, useState } from "react";
import { BookOpen } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { adminReq, type Settings } from "./api";

/** 文档站地址配置：登录页与页头据此显示文档入口。 */
export function DocsSettings({ settings, onChange }: { settings: Settings | null; onChange: () => void }) {
  const { t, to } = useI18n();
  const [url, setUrl] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  useEffect(() => {
    if (settings) setUrl(settings.docs_url ?? "");
  }, [settings]);

  const save = async () => {
    setBusy(true);
    setMsg("");
    try {
      await adminReq("/settings", { docs_url: url.trim() });
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
          <BookOpen className="h-4 w-4" />
          {t("admin.docsTitle")}
        </CardTitle>
        <CardDescription>{t("admin.docsHint")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        <div className="grid gap-2">
          <Label htmlFor="docs-url">{t("admin.docsUrl")}</Label>
          <Input
            id="docs-url"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder="https://holihur.github.io/gitdash/"
            autoComplete="off"
          />
          <p className="text-xs text-muted-foreground">{t("admin.docsEnvHint")}</p>
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
