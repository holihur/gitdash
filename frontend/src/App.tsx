import { useState } from "react";
import { BrowserRouter, Link, NavLink, Route, Routes } from "react-router-dom";
import { Toaster, toast } from "sonner";
import { GitBranch, KeyRound, Settings, FolderGit2 } from "lucide-react";
import { getToken, setToken } from "@/lib/api";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import Repos from "@/pages/Repos";
import RepoView from "@/pages/RepoView";
import Keys from "@/pages/Keys";

function TokenDialog() {
  const [value, setValue] = useState(getToken());

  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm" className="gap-2">
          <Settings className="h-4 w-4" />
          API Token
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>API Token</DialogTitle>
          <DialogDescription>
            后端通过 Authorization: Bearer 鉴权，默认 token 为 dev。可用环境变量 GITDASH_TOKEN 修改。
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-2">
          <Label htmlFor="token">Token</Label>
          <Input id="token" value={value} onChange={(e) => setValue(e.target.value)} />
        </div>
        <DialogFooter>
          <Button
            onClick={() => {
              setToken(value.trim());
              toast.success("Token 已保存");
            }}
          >
            保存
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default function App() {
  return (
    <BrowserRouter>
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
            <div className="ml-auto">
              <TokenDialog />
            </div>
          </div>
        </header>
        <main className="container py-8">
          <Routes>
            <Route path="/" element={<Repos />} />
            <Route path="/repo/:name" element={<RepoView />} />
            <Route path="/keys" element={<Keys />} />
          </Routes>
        </main>
      </div>
      <Toaster richColors position="top-center" />
    </BrowserRouter>
  );
}
