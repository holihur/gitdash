import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useI18n } from "@/lib/i18n";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  owner: string;
  name: string;
  forkName: string;
  onForkName: (v: string) => void;
  busy: boolean;
  onFork: () => void;
}

/** fork 仓库对话框：输入目标名称后创建 fork。 */
export default function ForkDialog({
  open,
  onOpenChange,
  owner,
  name,
  forkName,
  onForkName,
  busy,
  onFork,
}: Props) {
  const { t } = useI18n();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("social.forkTitle")}</DialogTitle>
          <DialogDescription>
            {t("social.forkDescription", { name: `${owner}/${name}` })}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-2">
          <Label htmlFor="fork-name">{t("social.forkNameLabel")}</Label>
          <Input id="fork-name" value={forkName} onChange={(e) => onForkName(e.target.value)} />
        </div>
        <DialogFooter>
          <Button onClick={onFork} disabled={busy || !forkName.trim()}>
            {busy ? t("social.forking") : t("social.fork")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
