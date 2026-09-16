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
import { Textarea } from "@/components/ui/textarea";
import { useI18n } from "@/lib/i18n";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  url: string;
  onUrl: (v: string) => void;
  name: string;
  onName: (v: string) => void;
  privateRepo: boolean;
  onPrivate: (v: boolean) => void;
  key: string;
  onKey: (v: string) => void;
  busy: boolean;
  onImport: () => void;
}

/** 从第三方远端导入仓库（可选私钥）对话框。 */
export default function ImportRepoDialog({
  open,
  onOpenChange,
  url,
  onUrl,
  name,
  onName,
  privateRepo,
  onPrivate,
  key,
  onKey,
  busy,
  onImport,
}: Props) {
  const { t } = useI18n();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[calc(100vw-2rem)] sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("imports.importTitle")}</DialogTitle>
          <DialogDescription>{t("imports.importDescription")}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="import-url">{t("imports.urlLabel")}</Label>
            <Input
              id="import-url"
              placeholder="https://github.com/owner/repo.git"
              value={url}
              onChange={(e) => onUrl(e.target.value)}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="import-name">{t("imports.nameLabel")}</Label>
            <Input
              id="import-name"
              placeholder={t("common.optional")}
              value={name}
              onChange={(e) => onName(e.target.value)}
            />
          </div>
          <div className="flex items-center gap-2">
            <input
              id="import-private"
              type="checkbox"
              checked={privateRepo}
              onChange={(e) => onPrivate(e.target.checked)}
              className="h-4 w-4"
            />
            <Label htmlFor="import-private" className="text-sm font-normal">
              {t("imports.privateLabel")}
            </Label>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="import-key">{t("imports.keyLabel")}</Label>
            <Textarea
              id="import-key"
              rows={4}
              placeholder={t("imports.keyPlaceholder")}
              value={key}
              onChange={(e) => onKey(e.target.value)}
              className="font-mono text-xs"
            />
            <p className="text-xs text-muted-foreground">{t("imports.keyHint")}</p>
          </div>
        </div>
        <DialogFooter>
          <Button onClick={onImport} disabled={busy || !url.trim()}>
            {t("imports.import")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
