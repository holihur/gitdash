import { Copy } from "lucide-react";
import { Button } from "@/components/ui/button";

export function CodeBlock({ text, onCopy }: { text: string; onCopy: () => void }) {
  return (
    <div className="flex items-center gap-2 rounded-md border bg-muted/50 px-3 py-2">
      <code className="min-w-0 flex-1 overflow-x-auto whitespace-pre text-xs">{text}</code>
      <Button variant="ghost" size="icon" className="h-6 w-6 shrink-0" onClick={onCopy}>
        <Copy className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}
