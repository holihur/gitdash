import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { BookTemplate, CircleDot, GitBranch, KeyRound, PencilLine, Plus, ShieldCheck, Tag, Trash2, Webhook } from "lucide-react";
import { api, type Branch, type BranchProtection, type IncomingWebhook, type Repo, type RepoEnvVar } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import ConfirmDialog from "@/components/confirm-dialog";
import { useI18n } from "@/lib/i18n";
import { apiErrorMsg } from "@/lib/errors";
import { copyText } from "@/lib/utils";

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

function IncomingWebhookCard({ owner, name }: { owner: string; name: string }) {
  const { t, to } = useI18n();
  const [status, setStatus] = useState<IncomingWebhook | null>(null);
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setStatus(await api.getIncomingWebhook(owner, name));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  }, [owner, name, to]);

  useEffect(() => {
    load();
  }, [load]);

  const enable = async () => {
    setBusy(true);
    try {
      const res = await api.setIncomingWebhook(owner, name);
      setStatus(res);
      setToken(res.token ?? "");
      toast.success(t(status?.enabled ? "repo.incomingRotated" : "repo.incomingEnabled"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const disable = async () => {
    setBusy(true);
    try {
      await api.deleteIncomingWebhook(owner, name);
      setStatus({ enabled: false });
      setToken("");
      toast.success(t("repo.incomingDisabled"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const path = status?.path ?? `/api/hooks/incoming/${owner}/${name}`;
  const endpoint = `${typeof window !== "undefined" ? window.location.origin : ""}${path}`;
  const curl = `curl -X POST "${endpoint}" \\\n  -H "X-Gitdash-Token: ${token || "<TOKEN>"}" \\\n  -H "Content-Type: application/json" \\\n  -d '{"title":"Issue title","body":"Issue body"}'`;

  const copy = (text: string, key: string) => {
    copyText(text)
      .then(() => toast.success(t(key)))
      .catch(() => toast.error(t("common.copyFailed")));
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Webhook className="h-4 w-4" />
          {t("repo.incomingWebhook")}
        </CardTitle>
        <CardDescription>{t("repo.incomingWebhookDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap items-center gap-3">
          <Badge variant={status?.enabled ? "secondary" : "outline"}>
            {status?.enabled ? t("repo.incomingOn") : t("repo.incomingOff")}
          </Badge>
          <Button size="sm" variant="outline" disabled={busy || !status} onClick={enable}>
            {status?.enabled ? t("repo.incomingRotate") : t("repo.incomingEnable")}
          </Button>
          {status?.enabled && (
            <Button
              size="sm"
              variant="ghost"
              className="text-destructive hover:text-destructive"
              disabled={busy}
              onClick={disable}
            >
              <Trash2 className="h-3.5 w-3.5" />
              {t("repo.incomingDisable")}
            </Button>
          )}
        </div>
        {status?.enabled && (
          <div className="space-y-2 text-sm">
            <div className="flex items-center gap-2">
              <span className="text-xs text-muted-foreground">{t("repo.incomingEndpoint")}</span>
              <code className="min-w-0 flex-1 truncate rounded bg-muted px-1.5 py-0.5 text-xs">
                {path}
              </code>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => copy(endpoint, "common.copied")}
              >
                {t("common.copy")}
              </Button>
            </div>
            {token ? (
              <div className="space-y-1">
                <div className="flex items-center gap-2">
                  <span className="text-xs text-muted-foreground">{t("repo.incomingToken")}</span>
                  <code className="min-w-0 flex-1 truncate rounded bg-muted px-1.5 py-0.5 font-mono text-xs">
                    {token}
                  </code>
                  <Button size="sm" variant="ghost" onClick={() => copy(token, "common.copied")}>
                    {t("common.copy")}
                  </Button>
                </div>
                <p className="text-xs text-amber-600 dark:text-amber-400">
                  {t("repo.incomingTokenOnce")}
                </p>
              </div>
            ) : (
              <p className="text-xs text-muted-foreground">{t("repo.incomingTokenHidden")}</p>
            )}
            <div className="space-y-1">
              <span className="text-xs text-muted-foreground">{t("repo.incomingExample")}</span>
              <pre className="overflow-x-auto rounded-md border bg-muted/40 p-2 text-xs">
                {curl}
              </pre>
              <Button size="sm" variant="outline" onClick={() => copy(curl, "common.copied")}>
                {t("common.copy")}
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function BranchProtectionsCard({ owner, name }: { owner: string; name: string }) {
  const { t, to } = useI18n();
  const [prots, setProts] = useState<BranchProtection[]>([]);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [branch, setBranch] = useState("");
  const [minApprovals, setMinApprovals] = useState("0");
  const [requireCI, setRequireCI] = useState(false);
  const [blockDeletion, setBlockDeletion] = useState(true);
  const [blockForcePush, setBlockForcePush] = useState(true);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const [p, b] = await Promise.all([
        api.listBranchProtections(owner, name),
        api.branches(owner, name),
      ]);
      setProts(p);
      setBranches(b);
      setBranch((cur) => cur || b[0]?.name || "");
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  }, [owner, name, to]);

  useEffect(() => {
    load();
  }, [load]);

  const save = async () => {
    if (!branch.trim()) return;
    const n = Number.parseInt(minApprovals, 10);
    if (!Number.isFinite(n) || n < 0 || n > 100) {
      toast.error(t("repo.bpInvalidApprovals"));
      return;
    }
    setBusy(true);
    try {
      await api.setBranchProtection(owner, name, branch.trim(), {
        min_approvals: n,
        require_ci: requireCI,
        block_deletion: blockDeletion,
        block_force_push: blockForcePush,
      });
      toast.success(t("repo.bpSaved", { branch: branch.trim() }));
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (b: string) => {
    try {
      await api.deleteBranchProtection(owner, name, b);
      toast.success(t("repo.bpRemoved", { branch: b }));
      load();
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  const ruleSummary = (p: BranchProtection) => {
    const parts: string[] = [];
    if (p.min_approvals > 0) parts.push(t("repo.bpMinApprovals", { count: p.min_approvals }));
    if (p.require_ci) parts.push(t("repo.bpRequireCI"));
    if (p.block_deletion) parts.push(t("repo.bpBlockDeletion"));
    if (p.block_force_push) parts.push(t("repo.bpBlockForcePush"));
    return parts.join(" · ") || t("repo.bpNoRules");
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <ShieldCheck className="h-4 w-4" />
          {t("repo.branchProtection")}
        </CardTitle>
        <CardDescription>{t("repo.branchProtectionDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap items-end gap-2">
          <label className="flex flex-col gap-1 text-xs text-muted-foreground">
            {t("repo.bpBranch")}
            <select
              className="h-9 rounded-md border bg-background px-2 text-sm"
              value={branch}
              onChange={(e) => setBranch(e.target.value)}
            >
              {branches.map((b) => (
                <option key={b.name} value={b.name}>
                  {b.name}
                </option>
              ))}
            </select>
          </label>
          <label className="flex flex-col gap-1 text-xs text-muted-foreground">
            {t("repo.bpMinApprovalsLabel")}
            <Input
              type="number"
              min={0}
              max={100}
              className="w-20"
              value={minApprovals}
              onChange={(e) => setMinApprovals(e.target.value)}
            />
          </label>
          <label className="flex items-center gap-1.5 pb-2 text-sm">
            <input
              type="checkbox"
              checked={requireCI}
              onChange={(e) => setRequireCI(e.target.checked)}
            />
            {t("repo.bpRequireCI")}
          </label>
          <label className="flex items-center gap-1.5 pb-2 text-sm">
            <input
              type="checkbox"
              checked={blockDeletion}
              onChange={(e) => setBlockDeletion(e.target.checked)}
            />
            {t("repo.bpBlockDeletion")}
          </label>
          <label className="flex items-center gap-1.5 pb-2 text-sm">
            <input
              type="checkbox"
              checked={blockForcePush}
              onChange={(e) => setBlockForcePush(e.target.checked)}
            />
            {t("repo.bpBlockForcePush")}
          </label>
          <Button size="sm" className="gap-1 pb-0 mb-0.5" disabled={busy || !branch} onClick={save}>
            <Plus className="h-3.5 w-3.5" />
            {t("repo.bpSave")}
          </Button>
        </div>

        {prots.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t("repo.bpEmpty")}</p>
        ) : (
          <div className="space-y-1.5">
            {prots.map((p) => (
              <div
                key={p.branch}
                className="flex items-center justify-between gap-2 rounded-md border bg-muted/20 p-2 text-sm"
              >
                <div className="flex min-w-0 flex-wrap items-center gap-2">
                  <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{p.branch}</code>
                  <span className="text-xs text-muted-foreground">{ruleSummary(p)}</span>
                </div>
                <Button
                  size="sm"
                  variant="ghost"
                  className="text-destructive hover:text-destructive"
                  onClick={() => remove(p.branch)}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function RepoEnvVarsCard({ owner, name }: { owner: string; name: string }) {
  const { t, to } = useI18n();
  const [vars, setVars] = useState<RepoEnvVar[]>([]);
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setVars(await api.listRepoEnvVars(owner, name));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  }, [owner, name, to]);

  useEffect(() => {
    load();
  }, [load]);

  const save = async () => {
    const k = key.trim();
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(k)) {
      toast.error(t("repo.envInvalidKey"));
      return;
    }
    setBusy(true);
    try {
      setVars(await api.setRepoEnvVar(owner, name, k, value));
      setKey("");
      setValue("");
      toast.success(t("repo.envSaved", { key: k }));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (k: string) => {
    try {
      await api.deleteRepoEnvVar(owner, name, k);
      setVars((v) => v.filter((x) => x.key !== k));
      toast.success(t("repo.envRemoved", { key: k }));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <KeyRound className="h-4 w-4" />
          {t("repo.envTitle")}
        </CardTitle>
        <CardDescription>{t("repo.envDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap items-end gap-2">
          <label className="flex flex-col gap-1 text-xs text-muted-foreground">
            {t("repo.envKey")}
            <Input
              className="w-40 font-mono"
              value={key}
              onChange={(e) => setKey(e.target.value)}
              placeholder="KEY"
            />
          </label>
          <label className="flex flex-col gap-1 text-xs text-muted-foreground">
            {t("repo.envValue")}
            <Input
              className="w-64 font-mono"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              placeholder="VALUE"
            />
          </label>
          <Button size="sm" className="gap-1 mb-0.5" disabled={busy || !key.trim()} onClick={save}>
            <Plus className="h-3.5 w-3.5" />
            {t("repo.envSave")}
          </Button>
        </div>

        {vars.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t("repo.envEmpty")}</p>
        ) : (
          <div className="space-y-1.5">
            {vars.map((v) => (
              <div
                key={v.key}
                className="flex items-center justify-between gap-2 rounded-md border bg-muted/20 p-2 text-sm"
              >
                <div className="flex min-w-0 flex-wrap items-center gap-2">
                  <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{v.key}</code>
                  <span className="min-w-0 truncate font-mono text-xs text-muted-foreground">
                    {v.value}
                  </span>
                </div>
                <Button
                  size="sm"
                  variant="ghost"
                  className="text-destructive hover:text-destructive"
                  onClick={() => remove(v.key)}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function PipelineCard({ owner, name }: { owner: string; name: string }) {
  const { t, to } = useI18n();
  const [enabled, setEnabled] = useState<boolean | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api
      .getPipeline(owner, name)
      .then((p) => setEnabled(p.enabled))
      .catch(() => setEnabled(false));
  }, [owner, name]);

  const toggle = async () => {
    setBusy(true);
    try {
      const res = await api.setPipeline(owner, name, !enabled);
      setEnabled(res.enabled);
      toast.success(t(res.enabled ? "pipeline.enabled" : "pipeline.disabled"));
    } catch (e) {
      toast.error(apiErrorMsg(to, e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("pipeline.title")}</CardTitle>
        <CardDescription>{t("pipeline.hint")}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex flex-wrap items-center gap-3">
          <Badge variant={enabled ? "secondary" : "outline"}>
            {t(enabled ? "pipeline.statusOn" : "pipeline.statusOff")}
          </Badge>
          <Button size="sm" variant="outline" disabled={busy || enabled === null} onClick={toggle}>
            {t(enabled ? "pipeline.turnOff" : "pipeline.turnOn")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
