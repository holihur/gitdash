import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import {
  Bell,
  Building2,
  Compass,
  Cpu,
  FolderGit2,
  KeyRound,
  Package,
  Search,
  UserRound,
} from "lucide-react";

import { api } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";

interface Command {
  id: string;
  group: string;
  label: string;
  hint?: string;
  keywords?: string;
  icon: ReactNode;
  run: () => void;
}

/** 命令面板（Ctrl/Cmd+K 或 /）：快速跳转页面与仓库。 */
export function CommandPalette({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useI18n();
  const navigate = useNavigate();
  const [q, setQ] = useState("");
  const [active, setActive] = useState(0);
  const [repos, setRepos] = useState<{ owner: string; name: string }[]>([]);

  useEffect(() => {
    if (!open) return;
    setQ("");
    setActive(0);
    api
      .listRepos(200, 0)
      .then((r) => setRepos(r.items.map((x) => ({ owner: x.owner, name: x.name }))))
      .catch(() => setRepos([]));
  }, [open]);

  const commands = useMemo<Command[]>(() => {
    const nav = (id: string, to: string, label: string, icon: ReactNode) => ({
      id,
      group: t("app.cmdNav"),
      label,
      icon,
      run: () => navigate(to),
    });
    const navCmds: Command[] = [
      nav("nav-repos", "/", t("nav.repos"), <FolderGit2 className="h-4 w-4" />),
      nav("nav-explore", "/explore", t("nav.explore"), <Compass className="h-4 w-4" />),
      nav("nav-orgs", "/orgs", t("nav.orgs"), <Building2 className="h-4 w-4" />),
      nav("nav-inbox", "/inbox", t("nav.inbox"), <Bell className="h-4 w-4" />),
      nav("nav-runners", "/runners", t("nav.runners"), <Cpu className="h-4 w-4" />),
      nav("nav-keys", "/keys", t("nav.keys"), <KeyRound className="h-4 w-4" />),
      nav("nav-packages", "/packages", t("nav.packages"), <Package className="h-4 w-4" />),
      nav("nav-profile", "/profile", t("profile.title"), <UserRound className="h-4 w-4" />),
    ];
    const repoCmds: Command[] = repos.map((r) => ({
      id: `repo-${r.owner}/${r.name}`,
      group: t("app.cmdRepos"),
      label: `${r.owner}/${r.name}`,
      keywords: r.name,
      icon: <FolderGit2 className="h-4 w-4" />,
      run: () => navigate(`/repo/${r.owner}/${r.name}`),
    }));
    return [...navCmds, ...repoCmds];
  }, [t, navigate, repos]);

  const query = q.trim().toLowerCase();
  const filtered = query
    ? commands.filter(
        (c) =>
          c.label.toLowerCase().includes(query) ||
          c.group.toLowerCase().includes(query) ||
          (c.keywords ?? "").toLowerCase().includes(query),
      )
    : commands;

  const pick = (c: Command | undefined) => {
    if (!c) return;
    onOpenChange(false);
    c.run();
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setActive((a) => Math.min(a + 1, filtered.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActive((a) => Math.max(a - 1, 0));
    } else if (e.key === "Enter") {
      e.preventDefault();
      pick(filtered[active]);
    }
  };

  let lastGroup = "";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="top-[15%] translate-y-0 gap-0 overflow-hidden p-0 sm:max-w-xl">
        <DialogTitle className="sr-only">{t("app.commandPalette")}</DialogTitle>
        <div className="flex items-center gap-2 border-b px-3">
          <Search className="h-4 w-4 shrink-0 text-muted-foreground" />
          <input
            autoFocus
            value={q}
            onChange={(e) => {
              setQ(e.target.value);
              setActive(0);
            }}
            onKeyDown={onKeyDown}
            placeholder={t("app.commandPlaceholder")}
            className="h-11 w-full bg-transparent pr-8 text-sm outline-none placeholder:text-muted-foreground"
          />
        </div>
        <div className="max-h-80 overflow-y-auto p-1">
          {filtered.length === 0 ? (
            <p className="p-3 text-sm text-muted-foreground">{t("app.noCommands")}</p>
          ) : (
            filtered.map((c, i) => {
              const header = c.group !== lastGroup ? c.group : null;
              lastGroup = c.group;
              return (
                <div key={c.id}>
                  {header && (
                    <p className="px-2 pb-1 pt-2 text-xs font-medium text-muted-foreground">{header}</p>
                  )}
                  <button
                    type="button"
                    onMouseEnter={() => setActive(i)}
                    onClick={() => pick(c)}
                    className={cn(
                      "flex w-full items-center gap-2 rounded-sm px-2 py-2 text-left text-sm",
                      i === active ? "bg-accent text-accent-foreground" : "text-foreground",
                    )}
                  >
                    <span className="shrink-0 text-muted-foreground">{c.icon}</span>
                    <span className="min-w-0 flex-1 truncate">{c.label}</span>
                    {c.hint && <span className="shrink-0 text-xs text-muted-foreground">{c.hint}</span>}
                  </button>
                </div>
              );
            })
          )}
        </div>
        <div className="border-t px-3 py-2 text-[11px] text-muted-foreground">{t("app.cmdHint")}</div>
      </DialogContent>
    </Dialog>
  );
}
