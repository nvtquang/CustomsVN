import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { RotateCcw, Save } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { api } from "@/lib/api/client";
import type { Filters, Meta, SearchConfig } from "@/lib/api/types";
import { ConfigEditor, FiltersEditor } from "@/features/retrieval-lab/editors";
import { useRetrievalDefaults } from "@/features/retrieval-lab/use-defaults";

export const Route = createFileRoute("/settings/retrieval")({ component: RetrievalSettingsPage });

function RetrievalSettingsPage() {
  const { t } = useTranslation();
  const d = useRetrievalDefaults();
  return (
    <div className="mx-auto max-w-3xl space-y-4 p-4 lg:p-6">
      <div>
        <h1 className="text-xl font-semibold">{t("settings.title")}</h1>
        <p className="text-xs text-muted-foreground">{t("settings.subtitle")}</p>
      </div>
      {d.error && <p className="text-destructive">{t("common.error", { msg: d.error.message })}</p>}
      {d.ready && <SettingsForm meta={d.meta!} filters={d.filters!} config={d.config!} />}
    </div>
  );
}

function SettingsForm({ meta, filters: f0, config: c0 }: { meta: Meta; filters: Filters; config: SearchConfig }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  // The as-of day is not a setting: a saved one would freeze the bot in time.
  const [filters, setFilters] = useState<Filters>({ ...f0, as_of: undefined });
  const [config, setConfig] = useState(c0);
  const save = useMutation({
    mutationFn: () => api.saveSettings({ filters: { ...filters, as_of: undefined }, config }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["settings", "retrieval"] }),
  });
  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>{t("lab.filters")}</CardTitle>
        </CardHeader>
        <CardContent>
          <FiltersEditor
            value={{ ...filters, as_of: meta.today }}
            onChange={(f) => setFilters({ ...f, as_of: undefined })}
          />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t("lab.config")}</CardTitle>
        </CardHeader>
        <CardContent>
          <ConfigEditor
            value={config}
            onChange={setConfig}
            models={meta.embedding_models ?? []}
            rerankAvailable={meta.rerank}
          />
        </CardContent>
      </Card>
      <div className="flex items-center gap-2">
        <Button onClick={() => save.mutate()} disabled={save.isPending}>
          <Save /> {t("settings.save")}
        </Button>
        <Button
          variant="outline"
          onClick={() => {
            setConfig(meta.defaults);
            setFilters({ tiers: [1, 2] });
          }}
        >
          <RotateCcw /> {t("settings.reset")}
        </Button>
        {save.isSuccess && <span className="text-xs text-force-in">{t("settings.saved")}</span>}
        {save.error && (
          <span className="text-xs text-destructive">{t("common.error", { msg: save.error.message })}</span>
        )}
      </div>
    </>
  );
}
