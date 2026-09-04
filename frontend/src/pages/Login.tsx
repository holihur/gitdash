import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { GitBranch } from "lucide-react";
import { api, setToken } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

interface Props {
  onAuthed: (username: string) => void;
}

export default function Login({ onAuthed }: Props) {
  const nav = useNavigate();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (mode: "login" | "register") => {
    if (!username.trim() || !password) {
      toast.error("请输入用户名和密码");
      return;
    }
    setBusy(true);
    try {
      const r =
        mode === "login"
          ? await api.login(username.trim(), password)
          : await api.register(username.trim(), password);
      setToken(r.token);
      onAuthed(r.username);
      toast.success(mode === "login" ? `欢迎回来，${r.username}` : `账号已创建：${r.username}`);
      nav("/");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <Card className="w-full max-w-sm">
        <CardHeader className="items-center text-center">
          <div className="mx-auto mb-1 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
            <GitBranch className="h-6 w-6" />
          </div>
          <CardTitle>gitdash</CardTitle>
          <CardDescription>登录或注册以管理你的仓库</CardDescription>
        </CardHeader>
        <CardContent>
          <Tabs defaultValue="login">
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="login">登录</TabsTrigger>
              <TabsTrigger value="register">注册</TabsTrigger>
            </TabsList>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                submit("login");
              }}
            >
              <TabsContent value="login" className="mt-4 space-y-4">
                <CredentialsFields
                  username={username}
                  password={password}
                  setUsername={setUsername}
                  setPassword={setPassword}
                />
                <Button type="submit" className="w-full" disabled={busy}>
                  登录
                </Button>
              </TabsContent>
            </form>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                submit("register");
              }}
            >
              <TabsContent value="register" className="mt-4 space-y-4">
                <CredentialsFields
                  username={username}
                  password={password}
                  setUsername={setUsername}
                  setPassword={setPassword}
                  passwordHint="至少 8 位"
                />
                <Button type="submit" className="w-full" disabled={busy}>
                  注册并登录
                </Button>
              </TabsContent>
            </form>
          </Tabs>
          <p className="mt-4 text-center text-xs text-muted-foreground">
            注册后请在「SSH Keys」页面添加公钥，才能通过 SSH 访问仓库。
          </p>
        </CardContent>
      </Card>
    </div>
  );
}

function CredentialsFields(props: {
  username: string;
  password: string;
  setUsername: (v: string) => void;
  setPassword: (v: string) => void;
  passwordHint?: string;
}) {
  return (
    <div className="grid gap-4">
      <div className="grid gap-2">
        <Label htmlFor="username">用户名</Label>
        <Input
          id="username"
          placeholder="小写字母、数字、_ 或 -"
          autoComplete="username"
          value={props.username}
          onChange={(e) => props.setUsername(e.target.value)}
        />
      </div>
      <div className="grid gap-2">
        <Label htmlFor="password">密码{props.passwordHint ? `（${props.passwordHint}）` : ""}</Label>
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
