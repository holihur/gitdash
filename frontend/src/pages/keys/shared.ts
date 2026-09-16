export interface SectionProps {
  t: (key: string, vars?: Record<string, string | number>) => string;
  to: (key: string, vars?: Record<string, string | number>) => string | undefined;
  locale: string;
}

export const SCOPE_LABEL_KEY: Record<string, string> = {
  repo: "pats.scopeRepo",
  inbox: "pats.scopeInbox",
  keys: "pats.scopeKeys",
};

export const EXPIRY_OPTIONS: { value: number | null; labelKey: string }[] = [
  { value: null, labelKey: "pats.expiresNever" },
  { value: 7, labelKey: "pats.expires7" },
  { value: 30, labelKey: "pats.expires30" },
  { value: 90, labelKey: "pats.expires90" },
  { value: 180, labelKey: "pats.expires180" },
  { value: 365, labelKey: "pats.expires365" },
];

export function expiryRFC3339(days: number | null): string {
  if (days == null) return "";
  return new Date(Date.now() + days * 86400_000).toISOString();
}
