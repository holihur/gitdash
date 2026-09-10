import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Check, Copy, Cpu, Download, Play, Trash2 } from "lucide-react";
import { api, type Runner } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn, copyText } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

const INSTALL_BASE =
  "curl -fsSL https://raw.githubusercontent.com/holihur/gitdash/main/install.sh | bash -s -- runner";

export default function Runners() {
  const { t, to } = useI18n();
  const [runners, setRunners] = useState<Runner[] | null>(null);
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [copied, setCopied] = useState(false);
  const [mode, setMode] = useState<"dial" | "reverse">("dial");
  const [reverseUrl, setReverseUrl] = useState("");

  const load = useCallback(async () => {
    try {
      setRunners(await api.listRunners());
    } catch {
      setRunners([]);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const server = window.location.origin;

  const installCmd = !token
    ? null
    : mode === "dial"
      ? `${INSTALL_BASE} && gitdash-runner register -server ${server} -name my-runner -labels docker -token ${token} && gitdash-runner run`
      : reverseUrl.trim()
        ? `${INSTALL_BASE} && gitdash-runner register -server ${server} -name my-runner -labels docker -token ${token} -reverse -url ${reverseUrl.trim()} && gitdash-runner serve`
        : null;

  const issue = async () => {
    setBusy(true);
    try {
      const res = await api.createRunnerToken("user");
      setToken(res.token);
      toast.success(t("runner.tokenIssued"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const copy = (text: string) => {
    copyText(text)
      .then(() => {
        setCopied(true);
        toast.success(t("common.copied"));
        window.setTimeout(() => setCopied(false), 1500);
      })
      .catch(() => toast.error(t("common.copyFailed")));
  };

  const remove = async (name: string) => {
    try {
      await api.deleteRunner(name);
      toast.success(t("runner.deleted"));
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const steps = [
    { title: t("runnerGuide.step1Title"), desc: t("runnerGuide.step1Desc") },
    { title: t("runnerGuide.step2Title"), desc: t("runnerGuide.step2Desc") },
    { title: t("runnerGuide.step3Title"), desc: t("runnerGuide.step3Desc") },
  ];

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="flex items-center gap-2 text-base">
            <Cpu className="h-4 w-4" />
            {t("runnerGuide.title")}
          </CardTitle>
          <CardDescription className="mt-1">{t("runnerGuide.intro")}</CardDescription>
        </CardHeader>
        <CardContent>
          <ol className="space-y-3">
            {steps.map((s, i) => (
              <li key={i} className="flex gap-3">
                <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground">
                  {i + 1}
                </span>
                <div className="min-w-0">
                  <p className="text-sm font-medium">{s.title}</p>
                  <p className="mt-0.5 text-sm text-muted-foreground">{s.desc}</p>
                </div>
              </li>
            ))}
          </ol>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="flex items-center gap-2 text-base">
            <Download className="h-4 w-4" />
            {t("runnerGuide.installTitle")}
          </CardTitle>
          <CardDescription className="mt-1">{t("runnerGuide.installHint")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {!token && (
            <Button size="sm" variant="outline" disabled={busy} onClick={issue}>
              {t("runner.issueToken")}
            </Button>
          )}
          {token && (
            <div className="space-y-2">
              <Label className="text-xs">{t("runner.modeLabel")}</Label>
              <div className="flex gap-2">
                {(["dial", "reverse"] as const).map((m) => (
                  <Button
                    key={m}
                    size="sm"
                    variant={mode === m ? "default" : "outline"}
                    onClick={() => setMode(m)}
                  >
                    {t(`runner.mode.${m}`)}
                  </Button>
                ))}
              </div>
              {mode === "reverse" && (
                <div className="space-y-1.5">
                  <p className="text-xs text-muted-foreground">{t("runner.reverseDesc")}</p>
                  <Label className="text-xs" htmlFor="runner-reverse-url">
                    {t("runner.reverseUrlLabel")}
                  </Label>
                  <Input
                    id="runner-reverse-url"
                    value={reverseUrl}
                    onChange={(e) => setReverseUrl(e.target.value)}
                    placeholder={t("runner.reverseUrlPlaceholder")}
                    className="font-mono text-xs"
                  />
                </div>
              )}
            </div>
          )}
          {installCmd && (
            <>
              <p className="text-xs text-muted-foreground">{t("runner.tokenOnce")}</p>
              <div className="rounded-lg border bg-muted/40">
                <div className="flex items-center justify-between gap-2 border-b px-3 py-1.5">
                  <span className="text-xs font-medium text-muted-foreground">
                    {t("runnerGuide.oneLine")}
                  </span>
                  <Button size="icon" variant="ghost" className="h-6 w-6" onClick={() => copy(installCmd)}>
                    {copied ? (
                      <Check className="h-3.5 w-3.5 text-emerald-600" />
                    ) : (
                      <Copy className="h-3.5 w-3.5" />
                    )}
                  </Button>
                </div>
                <pre className="overflow-x-auto p-3 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap">
                  {installCmd}
                </pre>
              </div>
              <p className="text-xs text-muted-foreground">{t("runnerGuide.tokenNote")}</p>
            </>
          )}
          {token && mode === "reverse" && !reverseUrl.trim() && (
            <p className="text-xs text-destructive">{t("runner.reverseUrlRequired")}</p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">{t("runnerGuide.runsOnTitle")}</CardTitle>
          <CardDescription className="mt-1">{t("runnerGuide.runsOnDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <pre className="overflow-x-auto rounded-md border bg-muted/40 p-3 font-mono text-xs leading-relaxed">{`runs-on: [docker]   # 目标 runner 标签（须为 agent 标签的子集）`}</pre>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">{t("runner.title")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {runners === null ? null : runners.length === 0 ? (
            <p className="py-2 text-sm text-muted-foreground">{t("runner.none")}</p>
          ) : (
            <div className="space-y-2">
              {runners.map((r) => (
                <div key={r.id} className="flex items-center justify-between rounded-lg border p-3">
                  <div className="min-w-0">
                    <p className="flex items-center gap-2 text-sm font-medium">
                      <span
                        className={cn(
                          "inline-block h-2 w-2 rounded-full",
                          r.status === "online" ? "bg-emerald-500" : "bg-muted-foreground/40",
                        )}
                      />
                      {r.name}
                      <Badge variant="outline" className="text-[10px]">
                        {t(`runner.scope.${r.scope === "" ? "global" : r.scope.split(":")[0]}`)}
                      </Badge>
                      {r.mode === "reverse" && (
                        <Badge variant="secondary" className="text-[10px]">
                          {t("runner.mode.reverse")}
                        </Badge>
                      )}
                    </p>
                    <p className="mt-0.5 truncate text-xs text-muted-foreground">
                      {(r.labels.length > 0 ? r.labels.join(", ") : "—") +
                        (r.mode === "reverse" && r.url ? ` · ${r.url}` : "")}
                    </p>
                  </div>
                  <Button size="icon" variant="ghost" onClick={() => remove(r.name)}>
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              ))}
            </div>
          )}
          <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <Play className="h-3 w-3" />
            {t("runnerGuide.listHint")}
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
