import { BookOpen } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useI18n } from "@/lib/i18n";

export const PIPELINE_EXAMPLE = `# 单文件：仓库根目录 .gitdash.yml
# 多文件：.gitdash/*.yml 或 *.yaml（每个文件是独立流水线，按各自 on: 触发）
image: alpine:3.19  # 可选：容器镜像；省略则直接在宿主 sh 执行（需服务端开启）
# job_timeout: 30m      # 可选：整次运行超时（缺省 = 不限，上限 2h）
# timeout: 10m          # 可选：单步超时（默认 10m，上限 1h）
# on: [push, pull_request, schedule, workflow_dispatch]  # 自动触发白名单（默认仅 push）
# schedule:              # on 含 schedule 时必填（cron：分 时 日 月 周）
#   - "0 2 * * *"
env:
  - CGO_ENABLED=0
# runs-on: [docker]   # 可选：指定远程 runner 标签
steps:
  - name: build
    run: echo build
  - name: test
    run: |
      echo test
      echo done`;

const DSL_FIELDS: [string, string][] = [
  ["image", "dslImage"],
  ["timeout", "dslTimeout"],
  ["job_timeout", "dslJobTimeout"],
  ["on", "dslOn"],
  ["schedule", "dslSchedule"],
  ["env", "dslEnv"],
  ["volumes", "dslVolumes"],
  ["runs-on", "dslRunsOn"],
  ["steps", "dslSteps"],
];

const TRIGGERS: [string, string][] = [
  ["push", "trigPush"],
  ["pull_request", "trigPr"],
  ["schedule", "trigSchedule"],
  ["workflow_dispatch", "trigDispatch"],
  ["manual", "trigManual"],
];

/** Pipeline 说明文档：文件布局 / DSL 字段 / 触发事件 / 运行生命周期 / 示例。 */
export default function PipelineDocs() {
  const { t } = useI18n();
  const header = (
    <TableHeader>
      <TableRow>
        <TableHead className="w-40">{t("pipeline.docs.dslKey")}</TableHead>
        <TableHead>{t("pipeline.docs.dslDesc")}</TableHead>
      </TableRow>
    </TableHeader>
  );
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="flex items-center gap-2 text-base">
          <BookOpen className="h-4 w-4" />
          {t("pipeline.docs.title")}
        </CardTitle>
        <CardDescription>{t("pipeline.docs.hint")}</CardDescription>
      </CardHeader>
      <CardContent>
        <Tabs defaultValue="files">
          <TabsList className="h-auto flex-wrap">
            <TabsTrigger value="files">{t("pipeline.docs.tabFiles")}</TabsTrigger>
            <TabsTrigger value="dsl">{t("pipeline.docs.tabDsl")}</TabsTrigger>
            <TabsTrigger value="triggers">{t("pipeline.docs.tabTriggers")}</TabsTrigger>
            <TabsTrigger value="lifecycle">{t("pipeline.docs.tabLifecycle")}</TabsTrigger>
            <TabsTrigger value="example">{t("pipeline.docs.tabExample")}</TabsTrigger>
          </TabsList>

          <TabsContent value="files" className="space-y-2 pt-3 text-sm text-muted-foreground">
            <p>{t("pipeline.docs.files1")}</p>
            <p>{t("pipeline.docs.files2")}</p>
            <p>{t("pipeline.docs.files3")}</p>
            <code className="block rounded bg-muted px-2 py-1 font-mono text-xs">.gitdash.yml</code>
            <code className="block rounded bg-muted px-2 py-1 font-mono text-xs">
              .gitdash/ci.yml · .gitdash/deploy.yaml
            </code>
          </TabsContent>

          <TabsContent value="dsl" className="pt-3">
            <Table>
              {header}
              <TableBody>
                {DSL_FIELDS.map(([k, d]) => (
                  <TableRow key={k}>
                    <TableCell className="align-top">
                      <code className="font-mono text-xs">{k}</code>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">{t(`pipeline.docs.${d}`)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TabsContent>

          <TabsContent value="triggers" className="pt-3">
            <Table>
              {header}
              <TableBody>
                {TRIGGERS.map(([k, d]) => (
                  <TableRow key={k}>
                    <TableCell className="align-top">
                      <code className="font-mono text-xs">{k}</code>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">{t(`pipeline.docs.${d}`)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TabsContent>

          <TabsContent value="lifecycle" className="space-y-2 pt-3 text-sm text-muted-foreground">
            <p>{t("pipeline.docs.life1")}</p>
            <p>{t("pipeline.docs.life2")}</p>
            <p>{t("pipeline.docs.life3")}</p>
            <p>{t("pipeline.docs.life4")}</p>
          </TabsContent>

          <TabsContent value="example" className="pt-3">
            <pre className="overflow-x-auto rounded-md border bg-muted/40 p-3 font-mono text-xs leading-relaxed">
              {PIPELINE_EXAMPLE}
            </pre>
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  );
}
