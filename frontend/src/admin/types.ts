export interface Quota {
  max_repos_per_user: number;
  max_repos_per_org: number;
  max_orgs_per_user: number;
  max_org_members: number;
  max_ssh_keys_per_user: number;
  max_gpg_keys_per_user: number;
  max_pats_per_user: number;
  max_webhooks_per_repo: number;
}

export interface QuotaOverride {
  scope: "user" | "org";
  name: string;
  quota: Quota;
}

export const EMPTY_QUOTA: Quota = {
  max_repos_per_user: 0,
  max_repos_per_org: 0,
  max_orgs_per_user: 0,
  max_org_members: 0,
  max_ssh_keys_per_user: 0,
  max_gpg_keys_per_user: 0,
  max_pats_per_user: 0,
  max_webhooks_per_repo: 0,
};

export const QUOTA_FIELDS: { key: keyof Quota; label: string }[] = [
  { key: "max_repos_per_user", label: "quotaReposPerUser" },
  { key: "max_repos_per_org", label: "quotaReposPerOrg" },
  { key: "max_orgs_per_user", label: "quotaOrgsPerUser" },
  { key: "max_org_members", label: "quotaOrgMembers" },
  { key: "max_ssh_keys_per_user", label: "quotaSSHKeys" },
  { key: "max_gpg_keys_per_user", label: "quotaGPGKeys" },
  { key: "max_pats_per_user", label: "quotaPATs" },
  { key: "max_webhooks_per_repo", label: "quotaWebhooks" },
];


export interface AdminUser {
  id: number;
  username: string;
  email: string | null;
  created_at: string;
  mfa_enabled: boolean;
  notify_email: boolean;
  banned: boolean;
}

export interface AdminRepo {
  id: number;
  owner: string;
  name: string;
  description: string;
  private: boolean;
  is_template: boolean;
  banned: boolean;
  created_at: string;
}

export interface AdminOrg {
  id: number;
  name: string;
  display: string;
  created_at: string;
  banned: boolean;
}

export interface AdminIPBan {
  id: number;
  cidr: string;
  note: string;
  created_by: string;
  created_at: string;
}

export const PAGE_SIZE = 20;
