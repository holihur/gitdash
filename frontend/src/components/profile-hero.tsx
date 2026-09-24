import type { ReactNode } from "react";
import { CoverBanner } from "@/components/cover-banner";

interface Props {
  /** 封面地址（未设置时显示渐变占位）。 */
  coverUrl?: string;
  /** 头像 / 组织图标（建议 80px；调用方负责圆角与 ring）。 */
  avatar: ReactNode;
  title: string;
  /** 次要标识，如 @username / 组织名。 */
  subtitle?: string;
  /** 角色等徽标。 */
  badge?: ReactNode;
  /** 徽章条（用户名下方） */
  badges?: ReactNode;
  bio?: string;
  /** 创建时间 / followers 等元信息。 */
  meta?: ReactNode;
  /** 右侧操作区（关注 / 编辑 / 删除等）。 */
  actions?: ReactNode;
}

/**
 * 用户 / 组织主页头部：封面横幅 + 半悬浮头像 + 名称信息 + 操作。
 * 头像压在封面下沿（`-bottom-10`），让封面与身份区连成一体，而不是割裂的一条顶栏。
 */
export function ProfileHero({ coverUrl, avatar, title, subtitle, badge, badges, bio, meta, actions }: Props) {
  return (
    <div>
      <div className="relative">
        <CoverBanner coverUrl={coverUrl} />
        <div className="absolute -bottom-10 left-4 z-10 sm:left-6">{avatar}</div>
      </div>
      <div className="flex flex-col gap-3 pt-12 sm:flex-row sm:items-start sm:justify-between sm:pt-14">
        <div className="min-w-0 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="truncate text-2xl font-bold">{title}</h1>
            {subtitle && (
              <span className="font-mono text-sm text-muted-foreground">{subtitle}</span>
            )}
            {badge}
          </div>
          {badges}
          {bio && <p className="whitespace-pre-wrap text-sm">{bio}</p>}
          {meta}
        </div>
        {actions && <div className="flex shrink-0 flex-wrap gap-2 self-start">{actions}</div>}
      </div>
    </div>
  );
}
