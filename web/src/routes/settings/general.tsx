import { createFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ShortcutList } from "@/components/layout/app-shell";
import { PageTitle } from "@/features/admin/admin-only";
import { LanguageCard, ThemeCard } from "@/features/settings/appearance";

export const Route = createFileRoute("/settings/general")({ component: GeneralPage });

function GeneralPage() {
  const { t } = useTranslation();
  return (
    <div className="mx-auto max-w-3xl space-y-4 p-4 lg:p-6">
      <PageTitle title={t("set.general")} subtitle={t("set.generalText")} />
      <ThemeCard />
      <LanguageCard />
      <Card>
        <CardHeader>
          <div>
            <CardTitle>{t("shortcuts.title")}</CardTitle>
            <p className="mt-0.5 text-xs text-muted-foreground">{t("set.shortcutsText")}</p>
          </div>
        </CardHeader>
        <CardContent className="max-w-md">
          <ShortcutList />
        </CardContent>
      </Card>
    </div>
  );
}
