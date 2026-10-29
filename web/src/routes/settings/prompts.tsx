import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { CheckCircle2, ChevronRight, RotateCcw, Save } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Textarea } from "@/components/ui/form";
import { AdminOnly, ErrorLine, PageTitle, Skeleton } from "@/features/admin/admin-only";
import { request } from "@/lib/api/client";

export const Route = createFileRoute("/settings/prompts")({
  component: () => (
    <AdminOnly>
      <PromptsPage />
    </AdminOnly>
  ),
});

const KEYS = ["compose", "agent", "router", "rewrite"] as const;
type Key = (typeof KEYS)[number];
type Prompts = Record<Key, string>;

interface PromptsView {
  prompts: Prompts;
  custom: Record<Key, boolean>;
  defaults: Prompts;
  contracts: Prompts;
}

const MAX = 20000;

function PromptsPage() {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ["settings", "prompts"],
    queryFn: () => request<PromptsView>("GET", "/api/settings/prompts"),
  });
  return (
    <div className="mx-auto max-w-4xl space-y-4 p-4 lg:p-6">
      <PageTitle title={t("prompts.title")} subtitle={t("prompts.subtitle")} />
      <ErrorLine error={q.error} />
      {q.data ? <Editor key={JSON.stringify(q.data.prompts)} view={q.data} /> : !q.error && <Skeleton rows={8} />}
    </div>
  );
}

/** Every prompt is saved together; one left as its default is stored empty. */
function Editor({ view }: { view: PromptsView }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [draft, setDraft] = useState<Prompts>(view.prompts);
  const save = useMutation({
    mutationFn: (p: Prompts) => request<PromptsView>("PUT", "/api/settings/prompts", p),
    onSuccess: (v) => qc.setQueryData(["settings", "prompts"], v),
  });
  const dirty = KEYS.some((k) => draft[k] !== view.prompts[k]);
  const tooLong = KEYS.some((k) => draft[k].length > MAX);
  return (
    <>
      {KEYS.map((k) => (
        <Card key={k}>
          <CardHeader>
            <div className="min-w-0 flex-1">
              <CardTitle className="flex items-center gap-2">
                {t("prompts." + k + ".title")}
                {draft[k] !== view.defaults[k] ? (
                  <Badge variant="accent">{t("prompts.custom")}</Badge>
                ) : (
                  <Badge variant="outline">{t("prompts.default")}</Badge>
                )}
              </CardTitle>
              <p className="mt-0.5 text-xs text-muted-foreground">{t("prompts." + k + ".text")}</p>
            </div>
            <Button
              variant="ghost"
              size="sm"
              disabled={draft[k] === view.defaults[k]}
              onClick={() => setDraft({ ...draft, [k]: view.defaults[k] })}
            >
              <RotateCcw /> {t("prompts.reset")}
            </Button>
          </CardHeader>
          <CardContent className="space-y-2">
            <Textarea
              aria-label={t("prompts." + k + ".title")}
              value={draft[k]}
              onChange={(e) => setDraft({ ...draft, [k]: e.target.value })}
              rows={k === "compose" || k === "agent" ? 16 : 7}
              className="font-mono text-xs"
            />
            <div className="flex justify-end text-2xs text-muted-foreground tabular-nums">
              <span className={draft[k].length > MAX ? "text-destructive" : undefined}>
                {draft[k].length.toLocaleString()} / {MAX.toLocaleString()}
              </span>
            </div>
            <details className="group rounded-md border bg-surface-2">
              <summary className="flex cursor-pointer list-none items-center gap-1.5 px-3 py-2 text-xs font-medium text-muted-foreground">
                <ChevronRight className="size-3.5 transition-transform group-open:rotate-90" />
                {t("prompts.contract")}
              </summary>
              <pre className="border-t px-3 py-2 font-mono text-2xs whitespace-pre-wrap text-muted-foreground">
                {view.contracts[k]}
              </pre>
            </details>
          </CardContent>
        </Card>
      ))}
      <div className="sticky bottom-0 flex flex-wrap items-center gap-3 border-t bg-background/95 py-3 backdrop-blur">
        <Button onClick={() => save.mutate(draft)} disabled={!dirty || tooLong || save.isPending}>
          <Save /> {t("prompts.save")}
        </Button>
        {dirty && (
          <Button variant="outline" onClick={() => setDraft(view.prompts)}>
            {t("prompts.discard")}
          </Button>
        )}
        {save.isSuccess && !dirty && (
          <span role="status" className="flex items-center gap-1 text-xs text-force-in">
            <CheckCircle2 className="size-4" /> {t("prompts.saved")}
          </span>
        )}
        <span className="text-2xs text-muted-foreground">{t("prompts.note")}</span>
      </div>
      <ErrorLine error={save.error} />
    </>
  );
}
