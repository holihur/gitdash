import { useRef } from "react";
import { ImagePlus, Trash2 } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

interface Props {
  /** 封面地址（未设置时为 undefined）。 */
  coverUrl?: string;
  /** 上传 / 删除后的缓存失效版本号。 */
  version?: number;
  /** 是否显示编辑按钮（本人 / 组织 owner）。 */
  editable?: boolean;
  busy?: boolean;
  className?: string;
  onPick?: (file: File) => void;
  onRemove?: () => void;
}

/** 个人 / 组织主页顶部横幅。可编辑时提供上传与移除按钮。 */
export function CoverBanner({
  coverUrl,
  version,
  editable,
  busy,
  className,
  onPick,
  onRemove,
}: Props) {
  const { t } = useI18n();
  const inputRef = useRef<HTMLInputElement>(null);
  const src = coverUrl ? (version ? `${coverUrl}?v=${version}` : coverUrl) : "";

  return (
    <div className={cn("relative overflow-hidden rounded-lg border bg-muted", className)}>
      {src ? (
        <img src={src} alt="" className="h-40 w-full object-cover sm:h-56" />
      ) : (
        <div className="h-40 w-full bg-gradient-to-r from-muted via-muted/60 to-muted sm:h-56" />
      )}
      {editable && (
        <>
          <input
            ref={inputRef}
            type="file"
            accept="image/png,image/jpeg,image/gif,image/webp"
            className="hidden"
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) onPick?.(f);
              e.target.value = "";
            }}
          />
          <div className="absolute right-2 top-2 flex gap-2">
            <Button
              size="sm"
              variant="secondary"
              className="gap-1.5 shadow-sm"
              disabled={busy}
              onClick={() => inputRef.current?.click()}
            >
              <ImagePlus className="h-4 w-4" />
              {t("cover.upload")}
            </Button>
            {coverUrl && (
              <Button
                size="sm"
                variant="secondary"
                className="gap-1.5 shadow-sm"
                disabled={busy}
                onClick={onRemove}
              >
                <Trash2 className="h-4 w-4" />
                {t("cover.remove")}
              </Button>
            )}
          </div>
        </>
      )}
    </div>
  );
}
