import { Component, type ErrorInfo, type ReactNode } from "react";
import { AlertTriangle, RotateCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useI18n } from "@/lib/i18n";

function ErrorFallback({ error, onReset }: { error: Error; onReset: () => void }) {
  const { t } = useI18n();
  return (
    <div className="flex min-h-[50vh] items-center justify-center p-6">
      <div className="w-full max-w-md space-y-3 rounded-lg border border-destructive/40 bg-card p-6 text-center">
        <AlertTriangle className="mx-auto h-8 w-8 text-destructive" />
        <p className="font-medium">{t("common.boundaryTitle")}</p>
        <p className="text-sm text-muted-foreground">{t("common.boundaryBody")}</p>
        {/* 错误详情仅便于排查；不展示未消毒的堆栈给终端用户，只给简短 message */}
        {error.message && (
          <pre className="max-h-32 overflow-auto rounded-md border bg-muted/40 p-2 text-left font-mono text-[11px] text-muted-foreground">
            {error.message}
          </pre>
        )}
        <Button
          variant="outline"
          className="gap-1.5"
          onClick={() => {
            onReset();
            // 兜底：若错误由无法自愈的状态引起，允许整页重载。
            window.location.reload();
          }}
        >
          <RotateCw className="h-4 w-4" />
          {t("common.reload")}
        </Button>
      </div>
    </div>
  );
}

interface Props {
  children: ReactNode;
}

interface State {
  error: Error | null;
}

/**
 * 渲染错误边界：捕获子树中的渲染/生命周期错误，展示可恢复的提示页，
 * 避免任何组件抛错导致整页白屏。
 *
 * 注意：不会捕获异步/事件回调中的错误（那些应各自 try/catch）。
 */
export default class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("render error boundary:", error, info.componentStack);
  }

  private reset = () => this.setState({ error: null });

  render() {
    if (this.state.error) {
      return <ErrorFallback error={this.state.error} onReset={this.reset} />;
    }
    return this.props.children;
  }
}
