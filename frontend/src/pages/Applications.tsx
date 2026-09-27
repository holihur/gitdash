import { useState } from "react";
import { KeyRound } from "lucide-react";
import { dateLocale, useI18n } from "@/lib/i18n";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AppsSection } from "./applications/AppsSection";
import { AuthorizedSection } from "./applications/AuthorizedSection";

export default function Applications() {
  const { t, to, lang } = useI18n();
  const locale = dateLocale(lang);
  const [tab, setTab] = useState("apps");

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("oauthApps.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("oauthApps.subtitle")}</p>
      </div>
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="apps" className="gap-2">
            <KeyRound className="h-4 w-4" />
            {t("oauthApps.tabApps")}
          </TabsTrigger>
          <TabsTrigger value="authorized" className="gap-2">
            {t("oauthApps.tabAuthorized")}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="apps" className="mt-4">
          <AppsSection t={t} to={to} locale={locale} />
        </TabsContent>
        <TabsContent value="authorized" className="mt-4">
          <AuthorizedSection t={t} to={to} locale={locale} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
