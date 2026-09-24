import type { Repo } from "@/lib/api";
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
import { canAdmin, canMaintain, isRepoOwner } from "@/lib/repo-role";

export interface SettingsTabProps {
  owner: string;
  name: string;
  repo: Repo | null;
  setRepo: (repo: Repo) => void;
}

export default function SettingsTab({ owner, name, repo, setRepo }: SettingsTabProps) {
  const role = repo?.role;
  const canMaint = canMaintain(role);
  const canAdm = canAdmin(role);
  const ownerOnly = isRepoOwner(role);
  if (!repo) return null;
  return (
    <div className="space-y-4">
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
      {canAdm && <VisibilityCard owner={owner} name={name} repo={repo} setRepo={setRepo} />}
      {canMaint && <TemplateCard owner={owner} name={name} repo={repo} setRepo={setRepo} />}
      {canMaint && <GcCard owner={owner} name={name} repo={repo} setRepo={setRepo} />}
      {canMaint && <MirrorCard owner={owner} name={name} />}
      {ownerOnly && <DangerZoneCard owner={owner} name={name} />}
    </div>
  );
}
