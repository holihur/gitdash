import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { BookTemplate, CircleDot, GitBranch, PencilLine, Recycle, RefreshCw, Tag, Trash2 } from "lucide-react";
import { api, type Branch, type Repo } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import ConfirmDialog from "@/components/confirm-dialog";
import MirrorDialog from "@/components/mirror-dialog";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { cn, formatSize } from "@/lib/utils";
import { IncomingWebhookCard } from "./settings/IncomingWebhookCard";
import { BranchProtectionsCard } from "./settings/BranchProtectionsCard";
import { RepoEnvVarsCard } from "./settings/RepoEnvVarsCard";
import { PipelineCard } from "./settings/PipelineCard";

export interface SettingsTabProps {
  owner: string;
  name: string;
  repo: Repo | null;
  setRepo: (repo: Repo) => void;
}

export default function SettingsTab({ owner, name, repo, setRepo }: SettingsTabProps) {
  const { t, to } = useI18n();
  const navigate = useNavigate();
  const [visibilityBusy, setVisibilityBusy] = useState(false);
  const [templateBusy, setTemplateBusy] = useState(false);
  const [description, setDescription] = useState("");
  const [descBusy, setDescBusy] = useState(false);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [defaultBranch, setDefaultBranch] = useState("");
  const [branchBusy, setBranchBusy] = useState(false);
  const [issuesBusy, setIssuesBusy] = useState(false);
  const [topicsInput, setTopicsInput] = useState("");
  const [topicsBusy, setTopicsBusy] = useState(false);
  const [deleteRepoOpen, setDeleteRepoOpen] = useState(false);
  const [deleteRepoBusy, setDeleteRepoBusy] = useState(false);
  const [gcOpen, setGcOpen] = useState(false);
  const [gcBusy, setGcBusy] = useState(false);
  const [mirrorOpen, setMirrorOpen] = useState(false);

  useEffect(() => {
    setDescription(repo?.description ?? "");
  }, [repo?.description]);

  useEffect(() => {
    setTopicsInput((repo?.topics ?? []).join(", "));
  }, [repo?.topics]);

  useEffect(() => {
    if (repo?.role !== "owner") return;
    api
      .branches(owner, name)
      .then(setBranches)
      .catch(() => setBranches([]));
  }, [owner, name, repo?.role]);

  useEffect(() => {
    setDefaultBranch(repo?.default_branch || branches[0]?.name || "");
  }, [repo?.default_branch, branches]);

  const saveDefaultBranch = async () => {
    if (!repo || !defaultBranch) return;
    setBranchBusy(true);
    try {
      const r = await api.setRepoDefaultBranch(owner, name, defaultBranch);
      setRepo({ ...repo, default_branch: r.default_branch });
      toast.success(t("repo.defaultBranchSaved", { branch: r.default_branch ?? defaultBranch }));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBranchBusy(false);
    }
  };

  const toggleIssues = async () => {
    if (!repo) return;
    setIssuesBusy(true);
    try {
      const next = !(repo.has_issues ?? true);
      const r = await api.setRepoIssues(owner, name, next);
      setRepo({ ...repo, has_issues: r.has_issues });
      toast.success(t(r.has_issues ? "repo.issuesNowOn" : "repo.issuesNowOff"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setIssuesBusy(false);
    }
  };

  const saveDescription = async () => {
    if (!repo) return;
    setDescBusy(true);
    try {
      const r = await api.setRepoDescription(owner, name, description.trim());
      setRepo({ ...repo, description: r.description });
      setDescription(r.description);
      toast.success(t("repo.descriptionSaved"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setDescBusy(false);
    }
  };

  const saveTopics = async () => {
    if (!repo) return;
    setTopicsBusy(true);
    try {
      const topics = topicsInput
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean);
      const r = await api.setRepoTopics(owner, name, topics);
      setRepo({ ...repo, topics: r.topics });
      setTopicsInput(r.topics.join(", "));
      toast.success(t("explore.topicsUpdated"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setTopicsBusy(false);
    }
  };

  const toggleVisibility = async () => {
    if (!repo) return;
    setVisibilityBusy(true);
    try {
      const r = await api.setRepoVisibility(owner, name, !repo.private);
      setRepo({ ...repo, private: r.private });
      toast.success(t(r.private ? "repo.visibilityNowPrivate" : "repo.visibilityNowPublic"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setVisibilityBusy(false);
    }
  };

  const toggleTemplate = async () => {
    if (!repo) return;
    setTemplateBusy(true);
    try {
      const r = await api.setRepoTemplate(owner, name, !repo.is_template);
      setRepo({ ...repo, is_template: r.is_template });
      toast.success(t(r.is_template ? "repo.templateNowOn" : "repo.templateNowOff"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setTemplateBusy(false);
    }
  };

  const doGC = async () => {
    if (!repo) return;
    setGcBusy(true);
    try {
      const res = await api.gcRepo(owner, name);
      setRepo({ ...repo, size: res.after_bytes });
      if (res.freed_bytes > 0) toast.success(t("repo.gcDone", { freed: formatSize(res.freed_bytes) }));
      else toast.success(t("repo.gcDoneNoop"));
      setGcOpen(false);
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setGcBusy(false);
    }
  };

  const doDeleteRepo = async () => {
    setDeleteRepoBusy(true);
    try {
      await api.deleteRepo(owner, name);
      toast.success(t("repo.repoDeleted"));
      setDeleteRepoOpen(false);
      navigate("/repos");
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setDeleteRepoBusy(false);
    }
  };

  return (
    <div className="space-y-4">
      {repo?.role === "owner" && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <PencilLine className="h-4 w-4" />
              {t("repo.description")}
            </CardTitle>
            <CardDescription>{t("repo.descriptionDesc")}</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex max-w-xl flex-wrap items-center gap-2">
              <Input
                value={description}
                maxLength={500}
                placeholder={t("repo.descriptionPlaceholder")}
                disabled={descBusy}
                className="min-w-0 flex-1"
                onChange={(e) => setDescription(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") void saveDescription();
                }}
              />
              <Button
                size="sm"
                disabled={descBusy || description.trim() === (repo?.description ?? "")}
                onClick={saveDescription}
              >
                {t("common.save")}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}
      {repo?.role === "owner" && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <Tag className="h-4 w-4" />
              {t("explore.editTopics")}
            </CardTitle>
            <CardDescription>{t("explore.topicsHint")}</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex max-w-xl flex-wrap items-center gap-2">
              <Input
                value={topicsInput}
                placeholder={t("explore.topicsPlaceholder")}
                disabled={topicsBusy}
                className="min-w-0 flex-1"
                onChange={(e) => setTopicsInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") void saveTopics();
                }}
              />
              <Button size="sm" disabled={topicsBusy} onClick={saveTopics}>
                {t("common.save")}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}
      <BranchProtectionsCard owner={owner} name={name} />
      {repo?.role === "owner" && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <GitBranch className="h-4 w-4" />
              {t("repo.defaultBranch")}
            </CardTitle>
            <CardDescription>{t("repo.defaultBranchDesc")}</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex flex-wrap items-center gap-2">
              <select
                className="h-9 max-w-full rounded-md border bg-background px-2 text-sm"
                value={defaultBranch}
                disabled={branchBusy || branches.length === 0}
                onChange={(e) => setDefaultBranch(e.target.value)}
              >
                {branches.map((b) => (
                  <option key={b.name} value={b.name}>
                    {b.name}
                  </option>
                ))}
              </select>
              <Button
                size="sm"
                disabled={
                  branchBusy ||
                  !defaultBranch ||
                  defaultBranch === (repo?.default_branch ?? "")
                }
                onClick={saveDefaultBranch}
              >
                {t("common.save")}
              </Button>
            </div>
            {branches.length === 0 && (
              <p className="mt-2 text-xs text-muted-foreground">
                {t("repo.defaultBranchEmpty")}
              </p>
            )}
          </CardContent>
        </Card>
      )}
      {repo?.role === "owner" && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <CircleDot className="h-4 w-4" />
              {t("repo.issuesFeature")}
            </CardTitle>
            <CardDescription>{t("repo.issuesFeatureDesc")}</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex flex-wrap items-center gap-3">
              <Badge variant={repo.has_issues === false ? "outline" : "secondary"}>
                {repo.has_issues === false ? t("repo.issuesOff") : t("repo.issuesOn")}
              </Badge>
              <Button size="sm" variant="outline" disabled={issuesBusy} onClick={toggleIssues}>
                {repo.has_issues === false ? t("repo.enableIssues") : t("repo.disableIssues")}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}
      {repo?.role === "owner" && <IncomingWebhookCard owner={owner} name={name} />}
      {repo?.role === "owner" && <PipelineCard owner={owner} name={name} />}
      {repo?.role === "owner" && <RepoEnvVarsCard owner={owner} name={name} />}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t("repo.visibility")}</CardTitle>
          <CardDescription>{t("repo.visibilityDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap items-center gap-3">
            <Badge variant={repo?.private ? "secondary" : "outline"}>
              {repo?.private ? t("repo.privateRepo") : t("repo.publicRepo")}
            </Badge>
            <Button
              size="sm"
              variant="outline"
              disabled={visibilityBusy || !repo}
              onClick={toggleVisibility}
            >
              {repo?.private ? t("repo.makePublic") : t("repo.makePrivate")}
            </Button>
          </div>
        </CardContent>
      </Card>
      {repo?.role === "owner" && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <BookTemplate className="h-4 w-4" />
              {t("repo.templateRepo")}
            </CardTitle>
            <CardDescription>{t("repo.templateRepoDesc")}</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex flex-wrap items-center gap-3">
              <Badge variant={repo?.is_template ? "secondary" : "outline"}>
                {repo?.is_template ? t("repo.templateOn") : t("repo.templateOff")}
              </Badge>
              <Button
                size="sm"
                variant="outline"
                disabled={templateBusy || !repo}
                onClick={toggleTemplate}
              >
                {repo?.is_template ? t("repo.makeNotTemplate") : t("repo.makeTemplate")}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}
      {repo?.role === "owner" && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <Recycle className="h-4 w-4" />
              {t("repo.gcTitle")}
            </CardTitle>
            <CardDescription>{t("repo.gcDesc")}</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="flex flex-wrap items-center gap-3">
              {typeof repo.size === "number" && (
                <span className="text-xs text-muted-foreground">
                  {t("repo.gcCurrentSize", { size: formatSize(repo.size) })}
                </span>
              )}
              <Button
                size="sm"
                variant="outline"
                className="gap-1.5"
                disabled={gcBusy || !repo}
                onClick={() => setGcOpen(true)}
              >
                <Recycle className={cn("h-3.5 w-3.5", gcBusy && "animate-spin")} />
                {gcBusy ? t("repo.gcRunning") : t("repo.gcRun")}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}
      {repo?.role === "owner" && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <RefreshCw className="h-4 w-4" />
              {t("mirror.title")}
            </CardTitle>
            <CardDescription>{t("mirror.hint")}</CardDescription>
          </CardHeader>
          <CardContent>
            <Button
              size="sm"
              variant="outline"
              className="gap-1.5"
              onClick={() => setMirrorOpen(true)}
            >
              <RefreshCw className="h-3.5 w-3.5" />
              {t("mirror.sync")}
            </Button>
          </CardContent>
        </Card>
      )}
      <Card className="border-destructive/40">
        <CardHeader>
          <CardTitle className="text-base text-destructive">{t("repo.dangerZone")}</CardTitle>
          <CardDescription>{t("repo.deleteRepoDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <Button
            size="sm"
            variant="outline"
            className="text-destructive hover:text-destructive"
            onClick={() => setDeleteRepoOpen(true)}
          >
            <Trash2 className="h-3.5 w-3.5" />
            {t("repo.deleteRepo")}
          </Button>
        </CardContent>
      </Card>
      <MirrorDialog
        open={mirrorOpen}
        onOpenChange={setMirrorOpen}
        owner={owner}
        repo={name}
      />
      <ConfirmDialog
        open={gcOpen}
        onOpenChange={setGcOpen}
        title={t("repo.gcRun")}
        description={t("repo.gcConfirm", { name: `${owner}/${name}` })}
        onConfirm={doGC}
        busy={gcBusy}
      />
      <ConfirmDialog
        open={deleteRepoOpen}
        onOpenChange={setDeleteRepoOpen}
        title={t("repo.deleteRepo")}
        description={t("repo.deleteRepoConfirm", { name: `${owner}/${name}` })}
        onConfirm={doDeleteRepo}
        busy={deleteRepoBusy}
      />
    </div>
  );
}

