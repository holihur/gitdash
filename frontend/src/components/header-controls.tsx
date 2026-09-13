import { Link } from "react-router-dom";
import { ChevronDown, Languages, LogOut, Moon, Settings, Sun, UserRound } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useTheme, type Theme } from "@/lib/theme";
import { useI18n, LANGS, type Lang } from "@/lib/i18n";
import { Avatar } from "@/components/avatar";

export function ThemeToggle() {
  const { theme, resolved, setTheme } = useTheme();
  const { t } = useI18n();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" className="h-9 w-9" title={t("app.theme")}>
          {resolved === "dark" ? <Moon className="h-4 w-4" /> : <Sun className="h-4 w-4" />}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuRadioGroup value={theme} onValueChange={(v) => setTheme(v as Theme)}>
          <DropdownMenuRadioItem value="light">{t("app.light")}</DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="dark">{t("app.dark")}</DropdownMenuRadioItem>
          <DropdownMenuRadioItem value="system">{t("app.system")}</DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function LangToggle() {
  const { lang, setLang, t } = useI18n();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" className="h-9 w-9" title={t("app.language")}>
          <Languages className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuRadioGroup value={lang} onValueChange={(v) => setLang(v as Lang)}>
          {LANGS.map((l) => (
            <DropdownMenuRadioItem key={l.value} value={l.value}>
              {l.label}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/**
 * 账号下拉菜单：合并用户入口、主题切换、语言切换与退出登录，
 * 避免头部堆叠多个独立按钮。
 */
export function UserMenu({ user, onLogout }: { user: string; onLogout: () => void }) {
  const { theme, resolved, setTheme } = useTheme();
  const { lang, setLang, t } = useI18n();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="gap-2 px-1.5 sm:px-2"
          title={user}
          aria-label={t("app.accountMenu")}
        >
          <Avatar username={user} size={24} />
          <span className="hidden max-w-32 truncate font-medium md:inline">{user}</span>
          <ChevronDown className="h-3.5 w-3.5 shrink-0 opacity-60" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <DropdownMenuLabel className="font-normal">
          <p className="text-xs text-muted-foreground">{t("app.signedInAs")}</p>
          <p className="truncate text-sm font-medium">{user}</p>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild>
          <Link to={`/users/${user}`} className="cursor-pointer">
            <UserRound />
            {t("user.yourProfile")}
          </Link>
        </DropdownMenuItem>
        <DropdownMenuItem asChild>
          <Link to="/profile" className="cursor-pointer">
            <Settings />
            {t("profile.title")}
          </Link>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuSub>
          <DropdownMenuSubTrigger>
            {resolved === "dark" ? <Moon /> : <Sun />}
            {t("app.theme")}
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent>
            <DropdownMenuRadioGroup value={theme} onValueChange={(v) => setTheme(v as Theme)}>
              <DropdownMenuRadioItem value="light">{t("app.light")}</DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="dark">{t("app.dark")}</DropdownMenuRadioItem>
              <DropdownMenuRadioItem value="system">{t("app.system")}</DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger>
            <Languages />
            {t("app.language")}
          </DropdownMenuSubTrigger>
          <DropdownMenuSubContent>
            <DropdownMenuRadioGroup value={lang} onValueChange={(v) => setLang(v as Lang)}>
              {LANGS.map((l) => (
                <DropdownMenuRadioItem key={l.value} value={l.value}>
                  {l.label}
                </DropdownMenuRadioItem>
              ))}
            </DropdownMenuRadioGroup>
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onSelect={onLogout}
          className="text-destructive focus:text-destructive"
        >
          <LogOut />
          {t("app.logout")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
