import { Badge } from "@/components/ui/badge";

export interface DiffFileInfo {
  path: string;
  status: "A" | "M" | "D";
  insertions: number;
  deletions: number;
}

/** 统一 diff 展示：文件统计徽标 + 逐行着色补丁。 */
export function DiffView({ files, patch }: { files: DiffFileInfo[]; patch: string }) {
  return (
    <div className="space-y-2">
      {files.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {files.map((f) => (
            <Badge
              key={f.path}
              variant="outline"
              className="max-w-full font-mono text-xs"
              title={`${f.path} (+${f.insertions} -${f.deletions})`}
            >
              <span
                className={
                  f.status === "A"
                    ? "text-green-600 dark:text-green-400"
                    : f.status === "D"
                      ? "text-red-600 dark:text-red-400"
                      : "text-yellow-600 dark:text-yellow-400"
                }
              >
                {f.status}
              </span>
              <span className="mx-1 truncate">{f.path}</span>
              <span className="shrink-0 text-muted-foreground">
                +{f.insertions} -{f.deletions}
              </span>
            </Badge>
          ))}
        </div>
      )}
      {patch ? (
        <div className="max-h-[60vh] overflow-auto rounded-md border bg-background/60">
          <DiffLines patch={patch} />
        </div>
      ) : (
        <p className="text-xs text-muted-foreground">-</p>
      )}
    </div>
  );
}

export function DiffLines({ patch }: { patch: string }) {
  return (
    <pre className="min-w-max px-3 py-2 font-mono text-xs leading-5">
      {patch.split("\n").map((line, i) => {
        let cls = "";
        if (line.startsWith("+") && !line.startsWith("+++"))
          cls = "bg-green-500/15 text-green-700 dark:text-green-400";
        else if (line.startsWith("-") && !line.startsWith("---"))
          cls = "bg-red-500/15 text-red-700 dark:text-red-400";
        else if (line.startsWith("@")) cls = "text-blue-600 dark:text-blue-400";
        else if (line.startsWith("diff --git") || line.startsWith("index "))
          cls = "text-muted-foreground";
        return (
          <div key={i} className={cls}>
            {line || " "}
          </div>
        );
      })}
    </pre>
  );
}
