import { lazy, Suspense, useCallback, useEffect, useState } from "react";
import { BrowserRouter, Link, NavLink, Navigate, Route, Routes } from "react-router-dom";
import { Toaster, toast } from "sonner";
import { Bell, Compass, GitBranch, KeyRound, Loader2, LogOut, FolderGit2, Building2, UserRound } from "lucide-react";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { ThemeToggle, LangToggle } from "@/components/header-controls";
import { useTheme } from "@/lib/theme";
import { useI18n } from "@/lib/i18n";
import Login from "@/pages/Login";

// 页面按需加载：首屏只需 App 外壳 + 登录页，其余页面路由切换时才拉取
const Repos = lazy(() => import("@/pages/Repos"));
const RepoView = lazy(() => import("@/pages/RepoView"));
const Inbox = lazy(() => import("@/pages/Inbox"));
const Explore = lazy(() => import("@/pages/Explore"));
const Keys = lazy(() => import("@/pages/Keys"));
const Orgs = lazy(() => import("@/pages/Orgs"));
const ProfilePage = lazy(() => import("@/pages/Profile"));

function PageLoading() {
  return (
    <div className="flex min-h-[50vh] items-center justify-center text-muted-foreground">
      <Loader2 className="h-6 w-6 animate-spin" />
    </div>
  );
}

export default function App() {
  const { t } = useI18n();
  const { resolved } = useTheme();
  const [user, setUser] = useState<string | null>(null);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    (async () => {
      try {
        // 会话在 httpOnly cookie 中自动携带：刷新后直接探测 /api/me 恢复登录态
        const me = await api.me();
        setUser(me.username);
      } catch {
        // 无有效会话（未登录 / 已过期）→ 展示登录页
        setUser(null);
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
    const timer = setInterval(() => {
      // 页面不可见时跳过轮询
      if (!document.hidden) refreshUnread();
    }, 30_000);
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
            <Button asChild variant="ghost" size="sm" className="px-2 sm:px-3">
              <NavLink to="/explore" className="flex items-center gap-2">
                <Compass className="h-4 w-4" />
                <span className="hidden sm:inline">{t("nav.explore")}</span>
              </NavLink>
            </Button>
            <Button asChild variant="ghost" size="sm" className="px-2 sm:px-3">
              <NavLink to="/orgs" className="flex items-center gap-2">
                <Building2 className="h-4 w-4" />
                <span className="hidden sm:inline">{t("nav.orgs")}</span>
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
          <Route
            path="/"
            element={
              <Suspense fallback={<PageLoading />}>
                <Repos />
              </Suspense>
            }
          />
          <Route
            path="/repo/:owner/:name"
            element={
              <Suspense fallback={<PageLoading />}>
                <RepoView />
              </Suspense>
            }
          />
          <Route
            path="/explore"
            element={
              <Suspense fallback={<PageLoading />}>
                <Explore />
              </Suspense>
            }
          />
          <Route
            path="/inbox"
            element={
              <Suspense fallback={<PageLoading />}>
                <Inbox onChanged={refreshUnread} />
              </Suspense>
            }
          />
          <Route
            path="/keys"
            element={
              <Suspense fallback={<PageLoading />}>
                <Keys />
              </Suspense>
            }
          />
          <Route
            path="/orgs/:org?"
            element={
              <Suspense fallback={<PageLoading />}>
                <Orgs />
              </Suspense>
            }
          />
          <Route
            path="/profile"
            element={
              <Suspense fallback={<PageLoading />}>
                <ProfilePage />
              </Suspense>
            }
          />
          <Route path="/login" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
    </div>
  );
}
