import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function GoogleIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" aria-hidden="true">
      <path
        fill="#4285F4"
        d="M23.49 12.27c0-.79-.07-1.54-.19-2.27H12v4.51h6.47a5.53 5.53 0 0 1-2.4 3.63v3h3.88c2.27-2.09 3.54-5.17 3.54-8.87z"
      />
      <path
        fill="#34A853"
        d="M12 24c3.24 0 5.95-1.08 7.94-2.91l-3.88-3c-1.08.72-2.45 1.16-4.06 1.16-3.13 0-5.78-2.11-6.73-4.96H1.29v3.09A12 12 0 0 0 12 24z"
      />
      <path
        fill="#FBBC05"
        d="M5.27 14.29a7.2 7.2 0 0 1 0-4.58V6.62H1.29a12 12 0 0 0 0 10.76l3.98-3.09z"
      />
      <path
        fill="#EA4335"
        d="M12 4.75c1.77 0 3.35.61 4.6 1.8l3.44-3.44C17.95 1.19 15.24 0 12 0A12 12 0 0 0 1.29 6.62l3.98 3.09C6.22 6.86 8.87 4.75 12 4.75z"
      />
    </svg>
  );
}


export type StrengthKey = "weak" | "fair" | "good" | "strong";

export function passwordStrength(pw: string): { score: number; key: StrengthKey } {
  if (!pw) return { score: 0, key: "weak" };
  const len = pw.length;
  const classes = [/[a-z]/, /[A-Z]/, /[0-9]/, /[^A-Za-z0-9]/].filter((re) => re.test(pw)).length;
  let score = 0;
  if (len >= 8) score++;
  if (classes >= 3) score++;
  if (len >= 12) score++;
  if (classes === 4) score++;
  if (score <= 1) return { score: 1, key: "weak" };
  if (score === 2) return { score: 2, key: "fair" };
  if (score === 3) return { score: 3, key: "good" };
  return { score: 4, key: "strong" };
}

export function PasswordStrengthMeter({ password }: { password: string }) {
  const { t } = useI18n();
  const s = passwordStrength(password);
  if (s.score === 0) return null;
  const color: Record<StrengthKey, string> = {
    weak: "bg-red-500",
    fair: "bg-amber-500",
    good: "bg-lime-500",
    strong: "bg-emerald-500",
  };
  const label: Record<StrengthKey, string> = {
    weak: t("login.passwordWeak"),
    fair: t("login.passwordFair"),
    good: t("login.passwordGood"),
    strong: t("login.passwordStrong"),
  };
  return (
    <div className="grid gap-1.5">
      <div className="flex gap-1" aria-hidden="true">
        {[1, 2, 3, 4].map((i) => (
          <span
            key={i}
            className={cn("h-1.5 flex-1 rounded-full", i <= s.score ? color[s.key] : "bg-muted")}
          />
        ))}
      </div>
      <p className="text-xs text-muted-foreground">
        {t("login.passwordStrength")}: <span className="font-medium">{label[s.key]}</span>
      </p>
    </div>
  );
}


export function CredentialsFields(props: {
  username: string;
  password: string;
  setUsername: (v: string) => void;
  setPassword: (v: string) => void;
  passwordHint?: string;
}) {
  const { t } = useI18n();
  return (
    <div className="grid gap-4">
      <div className="grid gap-2">
        <Label htmlFor="username">{t("login.username")}</Label>
        <Input
          id="username"
          placeholder={t("login.usernamePlaceholder")}
          autoComplete="username"
          value={props.username}
          onChange={(e) => props.setUsername(e.target.value)}
        />
      </div>
      <div className="grid gap-2">
        <Label htmlFor="password">
          {props.passwordHint
            ? t("login.passwordWithHint", { hint: props.passwordHint })
            : t("login.password")}
        </Label>
        <Input
          id="password"
          type="password"
          autoComplete={props.passwordHint ? "new-password" : "current-password"}
          value={props.password}
          onChange={(e) => props.setPassword(e.target.value)}
        />
      </div>
    </div>
  );
}
