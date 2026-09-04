import { useEffect, useState } from "react";
import { BrowserRouter, Link, NavLink, Route, Routes } from "react-router-dom";
import { Toaster, toast } from "sonner";
import { GitBranch, KeyRound, LogOut, FolderGit2 } from "lucide-react";
import { api, clearToken, getToken } from "@/lib/api";
import { Button } from "@/components/ui/button";
import Repos from "@/pages/Repos";
import RepoView from "@/pages/RepoView";
import Keys from "@/pages/Keys";
import Login from "@/pages/Login";

export default function App() {
  const [user, setUser] = useState<string | null>(null);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    (async () => {
      if (getToken()) {
        try {
          setUser((await api.me()).username);
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
    toast.success("已退出登录");
  };

  if (!ready) return null;

  return (
    <BrowserRouter>
      {user ? (
        <Shell user={user} onLogout={logout} />
      ) : (
        <Login onAuthed={(u) => setUser(u)} />
      )}
      <Toaster richColors position="top-center" />
    </BrowserRouter>
  );
}

function Shell({ user, onLogout }: { user: string; onLogout: () => void }) {
  return (
    <div className="min-h-screen">
      <header className="sticky top-0 z-40 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="container flex h-14 items-center gap-4">
          <Link to="/" className="flex items-center gap-2 text-lg font-bold">
            <GitBranch className="h-5 w-5" />
            gitdash
          </Link>
          <nav className="flex items-center gap-1">
            <Button asChild variant="ghost" size="sm">
              <NavLink to="/" className="flex items-center gap-2">
                <FolderGit2 className="h-4 w-4" />
                仓库
              </NavLink>
            </Button>
            <Button asChild variant="ghost" size="sm">
              <NavLink to="/keys" className="flex items-center gap-2">
                <KeyRound className="h-4 w-4" />
                SSH Keys
              </NavLink>
            </Button>
          </nav>
          <div className="ml-auto flex items-center gap-2">
            <span className="text-sm text-muted-foreground">
              <span className="font-medium text-foreground">{user}</span>
            </span>
            <Button variant="outline" size="sm" className="gap-2" onClick={onLogout}>
              <LogOut className="h-4 w-4" />
              退出
            </Button>
          </div>
        </div>
      </header>
      <main className="container py-8">
        <Routes>
          <Route path="/" element={<Repos />} />
          <Route path="/repo/:owner/:name" element={<RepoView />} />
          <Route path="/keys" element={<Keys />} />
        </Routes>
      </main>
    </div>
  );
}
