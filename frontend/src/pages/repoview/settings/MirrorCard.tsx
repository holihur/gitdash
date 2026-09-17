import { useState } from "react";
import { RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import MirrorDialog from "@/components/mirror-dialog";
import { useI18n } from "@/lib/i18n";

/** 镜像同步（仅 owner） */
export function MirrorCard({ owner, name }: { owner: string; name: string }) {
  const { t } = useI18n();
  const [mirrorOpen, setMirrorOpen] = useState(false);

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <RefreshCw className="h-4 w-4" />
            {t("mirror.title")}
          </CardTitle>
          <CardDescription>{t("mirror.hint")}</CardDescription>
        </CardHeader>
        <CardContent>
          <Button size="sm" variant="outline" className="gap-1.5" onClick={() => setMirrorOpen(true)}>
            <RefreshCw className="h-3.5 w-3.5" />
            {t("mirror.sync")}
          </Button>
        </CardContent>
      </Card>
      <MirrorDialog open={mirrorOpen} onOpenChange={setMirrorOpen} owner={owner} repo={name} />
    </>
  );
}
