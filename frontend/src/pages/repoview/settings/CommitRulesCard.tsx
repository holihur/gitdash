import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { UserCheck } from "lucide-react";
import { api } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";

/** 提交身份校验：限制 push 提交的作者/提交者姓名与邮箱格式（仅 owner）。 */
export function CommitRulesCard({ owner, name }: { owner: string; name: string }) {
  const { t, to } = useI18n();
  const [enabled, setEnabled] = useState(false);
  const [namePattern, setNamePattern] = useState("");
  const [emailPattern, setEmailPattern] = useState("");
  const [busy, setBusy] = useState(false);
  const [loaded, setLoaded] = useState(false);

  const load = useCallback(async () => {
    try {
      const r = await api.getRepoCommitRules(owner, name);
      setEnabled(r.enabled);
      setNamePattern(r.name_pattern);
      setEmailPattern(r.email_pattern);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setLoaded(true);
    }
  }, [owner, name, to]);

  useEffect(() => {
    void load();
  }, [load]);

  const save = async (namePat: string, emailPat: string) => {
    setBusy(true);
    try {
      const r = await api.setRepoCommitRules(owner, name, {
        name_pattern: namePat.trim(),
        email_pattern: emailPat.trim(),
      });
      setEnabled(r.enabled);
      setNamePattern(r.name_pattern);
      setEmailPattern(r.email_pattern);
      toast.success(t("repo.commitRulesSaved"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <UserCheck className="h-4 w-4" />
          {t("repo.commitRulesTitle")}
        </CardTitle>
        <CardDescription>{t("repo.commitRulesDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {!loaded ? (
          <p className="text-sm text-muted-foreground">…</p>
        ) : (
          <>
            <Badge variant={enabled ? "secondary" : "outline"}>
              {enabled ? t("repo.commitRulesEnabled") : t("repo.commitRulesDisabled")}
            </Badge>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="grid gap-1.5">
                <Label htmlFor="commit-name-pattern">{t("repo.commitRulesName")}</Label>
                <Input
                  id="commit-name-pattern"
                  value={namePattern}
                  onChange={(e) => setNamePattern(e.target.value)}
                  placeholder="^[A-Za-z][A-Za-z .'-]*$"
                  className="font-mono text-xs"
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="commit-email-pattern">{t("repo.commitRulesEmail")}</Label>
                <Input
                  id="commit-email-pattern"
                  value={emailPattern}
                  onChange={(e) => setEmailPattern(e.target.value)}
                  placeholder="^[^@]+@example\\.com$"
                  className="font-mono text-xs"
                />
              </div>
            </div>
            <p className="text-xs text-muted-foreground">{t("repo.commitRulesHint")}</p>
            <div className="flex flex-wrap gap-2">
              <Button
                size="sm"
                disabled={busy || (!namePattern.trim() && !emailPattern.trim())}
                onClick={() => save(namePattern, emailPattern)}
              >
                {t("repo.commitRulesSave")}
              </Button>
              {enabled && (
                <Button
                  size="sm"
                  variant="outline"
                  disabled={busy}
                  onClick={() => save("", "")}
                >
                  {t("repo.commitRulesClear")}
                </Button>
              )}
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}
