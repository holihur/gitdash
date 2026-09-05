import { useCallback, useEffect, useState } from "react";
import { BrowserRouter, Link, NavLink, Route, Routes } from "react-router-dom";
import { Toaster, toast } from "sonner";
import { Bell, GitBranch, KeyRound, LogOut, FolderGit2, UserRound } from "lucide-react";
import { api, clearToken, getToken } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { ThemeToggle, LangToggle } from "@/components/header-controls";
import { useTheme } from "@/lib/theme";
import { useI18n } from "@/lib/i18n";
import Repos from "@/pages/Repos";
import RepoView from "@/pages/RepoView";
import Inbox from "@/pages/Inbox";
import Keys from "@/pages/Keys";
import Login from "@/pages/Login";
import ProfilePage from "@/pages/Profile";

export default function App() {
  const { t } = useI18n();
  const { resolved } = useTheme();
  const [user, setUser] = useState<string | null>(null);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    (async () => {
      if (getToken()) {
        try {
          const me = await api.me();
          setUser(me.username);
        } catch {
          clearToken();
        }
      }
      setReady(true);
    })();
  }, []);

  const logout = async () => {
    try {
      await api.logout();
    } catch {
      /* ignore */
    }
    clearToken();
    setUser(null);
    toast.success(t("app.loggedOut"));
  };

  if (!ready) return null;

  return (
    <BrowserRouter>
      {user ? (
        <Shell user={user} onLogout={logout} />
      ) : (
        <Login onAuthed={(u) => setUser(u)} />
      )}
      <Toaster richColors position="top-center" theme={resolved} />
    </BrowserRouter>
  );
}

function Shell({ user, onLogout }: { user: string; onLogout: () => void }) {
  const { t } = useI18n();
  const [unread, setUnread] = useState(0);

  // 周期刷新未读通知数（30s），供导航铃铛角标使用。
  const refreshUnread = useCallback(async () => {
    try {
      const { count } = await api.inboxUnread();
      setUnread(count);
    } catch {
      /* 会话失效由全局 401 处理 */
    }
  }, []);

  useEffect(() => {
    refreshUnread();
    const timer = setInterval(refreshUnread, 30_000);
    return () => clearInterval(timer);
  }, [refreshUnread]);

  return (
    <div className="min-h-screen">
      <header className="sticky top-0 z-40 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="container flex h-14 items-center gap-2 sm:gap-4">
          <Link to="/" className="flex shrink-0 items-center gap-2 text-lg font-bold">
            <GitBranch className="h-5 w-5" />
            <span className="hidden sm:inline">gitdash</span>
          </Link>
          <nav className="flex items-center gap-1">
            <Button asChild variant="ghost" size="sm" className="px-2 sm:px-3">
              <NavLink to="/" className="flex items-center gap-2">
                <FolderGit2 className="h-4 w-4" />
                <span className="hidden sm:inline">{t("nav.repos")}</span>
              </NavLink>
            </Button>
            <Button asChild variant="ghost" size="sm" className="relative px-2 sm:px-3">
              <NavLink to="/inbox" className="flex items-center gap-2">
                <Bell className="h-4 w-4" />
                <span className="hidden sm:inline">{t("nav.inbox")}</span>
                {unread > 0 && (
                  <span className="absolute -right-0.5 -top-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-destructive px-1 text-[10px] font-bold leading-none text-destructive-foreground sm:static">
                    {unread > 99 ? "99+" : unread}
                  </span>
                )}
              </NavLink>
            </Button>
            <Button asChild variant="ghost" size="sm" className="px-2 sm:px-3">
              <NavLink to="/keys" className="flex items-center gap-2">
                <KeyRound className="h-4 w-4" />
                <span className="hidden sm:inline">{t("nav.keys")}</span>
              </NavLink>
            </Button>
          </nav>
          <div className="ml-auto flex items-center gap-1 sm:gap-2">
            <ThemeToggle />
            <LangToggle />
            <Button asChild variant="ghost" size="sm" className="px-2 sm:px-3">
              <Link to="/profile" className="flex items-center gap-2">
                <UserRound className="h-4 w-4" />
                <span className="hidden max-w-32 truncate font-medium text-foreground md:inline">
                  {user}
                </span>
              </Link>
            </Button>
            <Button variant="outline" size="sm" className="gap-2 px-2 sm:px-3" onClick={onLogout}>
              <LogOut className="h-4 w-4" />
              <span className="hidden sm:inline">{t("app.logout")}</span>
            </Button>
          </div>
        </div>
      </header>
      <main className="container py-4 sm:py-8">
        <Routes>
          <Route path="/" element={<Repos />} />
          <Route path="/repo/:owner/:name" element={<RepoView />} />
          <Route path="/inbox" element={<Inbox onChanged={refreshUnread} />} />
          <Route path="/keys" element={<Keys />} />
          <Route path="/profile" element={<ProfilePage />} />
        </Routes>
      </main>
    </div>
  );
}
