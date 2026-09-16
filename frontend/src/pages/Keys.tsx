import { useState } from "react";
import { KeyRound, KeySquare } from "lucide-react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { dateLocale, useI18n } from "@/lib/i18n";
import { SSHKeysSection } from "./keys/SSHKeysSection";
import { PATSection } from "./keys/PATSection";

export default function Keys() {
  const { t, lang, to } = useI18n();
  const locale = dateLocale(lang);
  const [tab, setTab] = useState("ssh");
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("keys.title")}</h1>
      </div>
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="ssh" className="gap-2">
            <KeyRound className="h-4 w-4" />
            SSH Keys
          </TabsTrigger>
          <TabsTrigger value="pats" className="gap-2">
            <KeySquare className="h-4 w-4" />
            {t("pats.title")}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="ssh" className="mt-4">
          <SSHKeysSection t={t} to={to} locale={locale} />
        </TabsContent>
        <TabsContent value="pats" className="mt-4">
          <PATSection t={t} to={to} locale={locale} />
        </TabsContent>
      </Tabs>
    </div>
  );
}

