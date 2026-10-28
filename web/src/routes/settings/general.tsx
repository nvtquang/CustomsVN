import { createFileRoute } from "@tanstack/react-router";
import { Monitor, Moon, Sun } from "lucide-react";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ShortcutList } from "@/components/layout/app-shell";
import { PageTitle } from "@/features/admin/admin-only";
import { languages, setLanguage, type Language } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { useUI, type Theme } from "@/stores/ui";

export const Route = createFileRoute("/settings/general")({ component: GeneralPage });

const LANGUAGE_NAMES: Record<Language, string> = { vi: "Tiếng Việt", en: "English" };

function GeneralPage() {
  const { t, i18n } = useTranslation();
  const theme = useUI((s) => s.theme);
  const setTheme = useUI((s) => s.setTheme);
  const themes: { value: Theme; icon: ReactNode }[] = [
    { value: "light", icon: <Sun /> },
    { value: "dark", icon: <Moon /> },
    { value: "system", icon: <Monitor /> },
  ];
  const option = (selected: boolean) =>
    cn(
      "flex cursor-pointer flex-col items-center gap-2 rounded-lg border bg-surface px-3 py-4 text-sm hover:bg-surface-2 [&_svg]:size-5",
      selected && "border-primary bg-accent/40 font-medium text-accent-foreground",
    );
  return (
    <div className="mx-auto max-w-3xl space-y-4 p-4 lg:p-6">
      <PageTitle title={t("set.general")} subtitle={t("set.generalText")} />
      <Card>
        <CardHeader>
          <div>
            <CardTitle>{t("theme.label")}</CardTitle>
            <p className="mt-0.5 text-xs text-muted-foreground">{t("set.themeText")}</p>
          </div>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-3 gap-2" role="radiogroup" aria-label={t("theme.label")}>
            {themes.map((th) => (
              <button
                key={th.value}
                type="button"
                role="radio"
                aria-checked={theme === th.value}
                onClick={() => setTheme(th.value)}
                className={option(theme === th.value)}
              >
                {th.icon}
                {t("theme." + th.value)}
              </button>
            ))}
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <div>
            <CardTitle>{t("lang.label")}</CardTitle>
            <p className="mt-0.5 text-xs text-muted-foreground">{t("set.langText")}</p>
          </div>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 gap-2 sm:max-w-sm" role="radiogroup" aria-label={t("lang.label")}>
            {languages.map((l) => (
              <button
                key={l}
                type="button"
                role="radio"
                aria-checked={i18n.language === l}
                onClick={() => setLanguage(l)}
                className={option(i18n.language === l)}
              >
                {LANGUAGE_NAMES[l] ?? l}
              </button>
            ))}
          </div>
        </CardContent>
      </Card>
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
