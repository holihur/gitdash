import type { Repo } from "@/lib/api";
import type { ReactNode } from "react";
import { useI18n } from "@/lib/i18n";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Check, X } from "lucide-react";
import { IncomingWebhookCard } from "./settings/IncomingWebhookCard";
import { OutgoingWebhooksCard } from "./settings/OutgoingWebhooksCard";
import { BranchProtectionsCard } from "./settings/BranchProtectionsCard";
import { RepoEnvVarsCard } from "./settings/RepoEnvVarsCard";
import { SecretsCard } from "./settings/SecretsCard";
import { PipelineCard } from "./settings/PipelineCard";
import { DescriptionCard } from "./settings/DescriptionCard";
import { BadgeDisplayPicker } from "@/components/badge-display-picker";
import { TopicsCard } from "./settings/TopicsCard";
import { DefaultBranchCard } from "./settings/DefaultBranchCard";
import { IssuesFeatureCard } from "./settings/IssuesFeatureCard";
import { PagesCard } from "./settings/PagesCard";
import { DeployKeysCard } from "./settings/DeployKeysCard";
import { CommitRulesCard } from "./settings/CommitRulesCard";
import { VisibilityCard } from "./settings/VisibilityCard";
import { TemplateCard } from "./settings/TemplateCard";
import { GcCard } from "./settings/GcCard";
import { MirrorCard } from "./settings/MirrorCard";
import { DangerZoneCard } from "./settings/DangerZoneCard";
import { RepoTeamAccess } from "@/components/repo-team-access";
import {
  canAdmin,
  canMaintain,
  canRead,
  canTriage,
  canWrite,
  isRepoOwner,
} from "@/lib/repo-role";

export interface SettingsTabProps {
  owner: string;
  name: string;
  repo: Repo | null;
  setRepo: (repo: Repo) => void;
}

/** 权限不足时置灰展示（而非隐藏），便于用户发现能力边界。 */
function Gated({ enabled, reason, children }: { enabled: boolean; reason: string; children: ReactNode }) {
  if (enabled) return <>{children}</>;
  return (
    <div
      aria-disabled="true"
      title={reason}
      className="pointer-events-none opacity-50 [&_button]:pointer-events-none [&_input]:pointer-events-none [&_select]:pointer-events-none [&_textarea]:pointer-events-none"
    >
      {children}
    </div>
  );
}

export default function SettingsTab({ owner, name, repo, setRepo }: SettingsTabProps) {
  const role = repo?.role;
  const canMaint = canMaintain(role);
  const canAdm = canAdmin(role);
  const ownerOnly = isRepoOwner(role);
  const { t } = useI18n();
  if (!repo) return null;
  const caps: [boolean, string][] = [
    [canRead(role), t("repoAccess.read")],
    [canTriage(role), t("repoAccess.triage")],
    [canWrite(role), t("repoAccess.write")],
    [canMaintain(role), t("repoAccess.maintain")],
    [canAdmin(role), t("repoAccess.admin")],
  ];
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">{t("repoAccess.title")}</CardTitle>
          <CardDescription className="flex items-center gap-2">
            {t("repoAccess.yourRole")}
            <Badge variant="secondary">{t(`collabs.${role ?? "read"}`)}</Badge>
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-1.5 text-sm sm:grid-cols-2">
          {caps.map(([ok, label]) => (
            <span key={label} className="flex items-center gap-1.5">
              {ok ? (
                <Check className="h-3.5 w-3.5 shrink-0 text-green-600" />
              ) : (
                <X className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
              )}
              <span className={ok ? "" : "text-muted-foreground"}>{label}</span>
            </span>
          ))}
        </CardContent>
      </Card>
      {canMaint && <DescriptionCard owner={owner} name={name} repo={repo} setRepo={setRepo} />}
      {canMaint && <BadgeDisplayPicker kind="repo" owner={owner} repo={name} />}
      {canMaint && <TopicsCard owner={owner} name={name} repo={repo} setRepo={setRepo} />}
      {canMaint && <BranchProtectionsCard owner={owner} name={name} />}
      {canMaint && <DefaultBranchCard owner={owner} name={name} repo={repo} setRepo={setRepo} />}
      {canMaint && <IssuesFeatureCard owner={owner} name={name} repo={repo} setRepo={setRepo} />}
      {canMaint && <PagesCard owner={owner} name={name} />}
      {canMaint && <DeployKeysCard owner={owner} name={name} />}
      {canMaint && <CommitRulesCard owner={owner} name={name} />}
      {canMaint && <IncomingWebhookCard owner={owner} name={name} />}
      {canMaint && <OutgoingWebhooksCard owner={owner} name={name} />}
      {canMaint && <PipelineCard owner={owner} name={name} />}
      {canMaint && <RepoEnvVarsCard owner={owner} name={name} />}
      {canMaint && <SecretsCard owner={owner} name={name} />}
      <Gated enabled={canAdm} reason={t("repoAccess.requiresAdmin")}>
        <VisibilityCard owner={owner} name={name} repo={repo} setRepo={setRepo} />
      </Gated>
      <Gated enabled={canAdm} reason={t("repoAccess.requiresAdmin")}>
        <RepoTeamAccess owner={owner} name={name} />
      </Gated>
      {canMaint && <TemplateCard owner={owner} name={name} repo={repo} setRepo={setRepo} />}
      {canMaint && <GcCard owner={owner} name={name} repo={repo} setRepo={setRepo} />}
      {canMaint && <MirrorCard owner={owner} name={name} />}
      <Gated enabled={ownerOnly} reason={t("repoAccess.requiresOwner")}>
        <DangerZoneCard owner={owner} name={name} />
      </Gated>
    </div>
  );
}
