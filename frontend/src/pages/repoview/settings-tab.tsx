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

export interface SettingsTabProps {
  owner: string;
  name: string;
  repo: Repo | null;
  setRepo: (repo: Repo) => void;
}

export default function SettingsTab({ owner, name, repo, setRepo }: SettingsTabProps) {
  const isOwner = repo?.role === "owner";
  return (
    <div className="space-y-4">
      {isOwner && <DescriptionCard owner={owner} name={name} repo={repo} setRepo={setRepo} />}
      {isOwner && <BadgeDisplayPicker kind="repo" owner={owner} repo={name} />}
      {isOwner && <TopicsCard owner={owner} name={name} repo={repo} setRepo={setRepo} />}
      <BranchProtectionsCard owner={owner} name={name} />
      {isOwner && <DefaultBranchCard owner={owner} name={name} repo={repo} setRepo={setRepo} />}
      {isOwner && <IssuesFeatureCard owner={owner} name={name} repo={repo} setRepo={setRepo} />}
      {isOwner && <PagesCard owner={owner} name={name} />}
      {isOwner && <DeployKeysCard owner={owner} name={name} />}
      {isOwner && <CommitRulesCard owner={owner} name={name} />}
      {isOwner && <IncomingWebhookCard owner={owner} name={name} />}
      {isOwner && <OutgoingWebhooksCard owner={owner} name={name} />}
      {isOwner && <PipelineCard owner={owner} name={name} />}
      {isOwner && <RepoEnvVarsCard owner={owner} name={name} />}
      {isOwner && <SecretsCard owner={owner} name={name} />}
      <VisibilityCard owner={owner} name={name} repo={repo} setRepo={setRepo} />
      {isOwner && <TemplateCard owner={owner} name={name} repo={repo} setRepo={setRepo} />}
      {isOwner && <GcCard owner={owner} name={name} repo={repo} setRepo={setRepo} />}
      {isOwner && <MirrorCard owner={owner} name={name} />}
      <DangerZoneCard owner={owner} name={name} />
    </div>
  );
}
