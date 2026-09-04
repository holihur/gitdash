import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { KeyRound, Plus, Trash2 } from "lucide-react";
import { api, type SSHKey } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { formatDate } from "@/lib/utils";

export default function Keys() {
  const [keys, setKeys] = useState<SSHKey[]>([]);
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [publicKey, setPublicKey] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setKeys(await api.listKeys());
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const create = async () => {
    setBusy(true);
    try {
      await api.createKey(name.trim(), publicKey.trim());
      toast.success(`SSH Key ${name.trim()} 已添加`);
      setOpen(false);
      setName("");
      setPublicKey("");
      load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (key: SSHKey) => {
    if (!window.confirm(`删除 SSH Key ${key.name}？该 key 将无法再通过 SSH 访问。`)) return;
    try {
      await api.deleteKey(key.id);
      toast.success("已删除");
      load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">SSH Keys</h1>
          <p className="text-sm text-muted-foreground">
            在这里添加公钥后，即可用对应私钥通过 SSH 操作仓库（clone / push）。
          </p>
        </div>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button className="gap-2">
              <Plus className="h-4 w-4" />
              添加 Key
            </Button>
          </DialogTrigger>
          <DialogContent className="sm:max-w-xl">
            <DialogHeader>
              <DialogTitle>添加 SSH 公钥</DialogTitle>
              <DialogDescription>
                粘贴 openssh 格式的公钥，例如 <code>ssh-ed25519 AAAA... user@host</code>
              </DialogDescription>
            </DialogHeader>
            <div className="grid gap-4">
              <div className="grid gap-2">
                <Label htmlFor="key-name">备注名称</Label>
                <Input
                  id="key-name"
                  placeholder="my laptop"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="key-pub">公钥</Label>
                <Textarea
                  id="key-pub"
                  placeholder="ssh-ed25519 AAAAC3Nza..."
                  className="min-h-32 font-mono text-xs"
                  value={publicKey}
                  onChange={(e) => setPublicKey(e.target.value)}
                />
              </div>
            </div>
            <DialogFooter>
              <Button onClick={create} disabled={busy || !name.trim() || !publicKey.trim()}>
                添加
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>

      {keys.length === 0 ? (
        <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed py-16 text-center">
          <KeyRound className="h-10 w-10 text-muted-foreground" />
          <p className="font-medium">还没有 SSH Key</p>
          <p className="text-sm text-muted-foreground">添加公钥后才能通过 SSH 访问仓库。</p>
        </div>
      ) : (
        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>名称</TableHead>
                <TableHead>指纹 (SHA256)</TableHead>
                <TableHead>公钥类型</TableHead>
                <TableHead>添加时间</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {keys.map((key) => (
                <TableRow key={key.id}>
                  <TableCell className="font-medium">{key.name}</TableCell>
                  <TableCell className="font-mono text-xs">{key.fingerprint}</TableCell>
                  <TableCell>
                    <Badge variant="secondary">{key.public_key.split(" ")[0]}</Badge>
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(key.created_at)}
                  </TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8 text-destructive hover:text-destructive"
                      onClick={() => remove(key)}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}
