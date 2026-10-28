import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { LabNav } from "@/features/lab/lab-nav";
import { useTranslation } from "react-i18next";
import { Tabs } from "@/components/ui/tabs";
import { ABCompare } from "@/features/retrieval-lab/ab";
import { Batch } from "@/features/retrieval-lab/batch";
import { useRetrievalDefaults } from "@/features/retrieval-lab/use-defaults";
import { SingleQuery } from "@/features/retrieval-lab/single";

type Tab = "single" | "ab" | "batch";

export const Route = createFileRoute("/lab/retrieval")({
  validateSearch: (s: Record<string, unknown>): { tab: Tab } => ({
    tab: s.tab === "ab" || s.tab === "batch" ? s.tab : "single",
  }),
  component: RetrievalLab,
});

function RetrievalLab() {
  const { t } = useTranslation();
  const { tab } = Route.useSearch();
  const navigate = useNavigate({ from: Route.fullPath });
  const d = useRetrievalDefaults();
  const models = d.meta?.embedding_models ?? [];
  return (
    <div className="mx-auto max-w-[110rem] space-y-4 p-4 lg:p-6">
      <LabNav />
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold">{t("lab.title")}</h1>
          <p className="text-xs text-muted-foreground">{t("lab.subtitle")}</p>
        </div>
        <Tabs<Tab>
          value={tab}
          onChange={(v) => void navigate({ search: { tab: v } })}
          items={[
            { value: "single", label: t("lab.tabs.single") },
            { value: "ab", label: t("lab.tabs.ab") },
            { value: "batch", label: t("lab.tabs.batch") },
          ]}
        />
      </div>
      {d.error && <p className="text-destructive">{t("common.error", { msg: d.error.message })}</p>}
      {!d.ready && !d.error && <p className="text-muted-foreground">{t("common.loading")}</p>}
      {d.ready && tab === "single" && (
        <SingleQuery initialFilters={d.filters!} initialConfig={d.config!} models={models} rerank={!!d.meta?.rerank} />
      )}
      {d.ready && tab === "ab" && (
        <ABCompare initialFilters={d.filters!} initialConfig={d.config!} models={models} rerank={!!d.meta?.rerank} />
      )}
      {d.ready && tab === "batch" && <Batch filters={d.filters!} config={d.config!} />}
    </div>
  );
}
