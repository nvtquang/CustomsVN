import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Ban, ChevronDown, ChevronRight, Play } from "lucide-react";
import { Fragment, useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox, Field, Input, Select } from "@/components/ui/form";
import { Table, Td, Th, Tr } from "@/components/ui/table";
import { AdminOnly, ErrorLine, PageTitle, Skeleton } from "@/features/admin/admin-only";
import { LabNav } from "@/features/lab/lab-nav";
import { ClaimView, RefusalView } from "@/features/chat/message";
import { adminApi, type AnswerConfig, type AnswerMetrics, type AnswerResult, type AnswerRun } from "@/lib/api/admin";
import { chatApi, type LLMTarget } from "@/lib/api/chat";
import { api } from "@/lib/api/client";
import { cn, fmt, when } from "@/lib/utils";

interface Search {
  run?: number;
  compare?: number;
}

export const Route = createFileRoute("/lab/answers")({
  validateSearch: (s: Record<string, unknown>): Search => ({
    run: Number(s.run) || undefined,
    compare: Number(s.compare) || undefined,
  }),
  component: () => (
    <AdminOnly>
      <AnswersLab />
    </AdminOnly>
  ),
});

/** The metrics a run is judged on, how to print them, and which way is better. */
const METRICS: { key: keyof AnswerMetrics; kind: "ratio" | "ms" | "usd" | "count"; better: "up" | "down" }[] = [
  { key: "refusal_accuracy", kind: "ratio", better: "up" },
  { key: "false_refusal", kind: "ratio", better: "down" },
  { key: "missed_refusal", kind: "ratio", better: "down" },
  { key: "supported", kind: "ratio", better: "up" },
  { key: "citation_precision", kind: "ratio", better: "up" },
  { key: "expected_cited", kind: "ratio", better: "up" },
  { key: "grounded", kind: "ratio", better: "up" },
  { key: "first_claim_p50_ms", kind: "ms", better: "down" },
  { key: "first_claim_p95_ms", kind: "ms", better: "down" },
  { key: "total_p95_ms", kind: "ms", better: "down" },
  { key: "cost_usd", kind: "usd", better: "down" },
  { key: "errors", kind: "count", better: "down" },
];

function show(kind: string, v: number | undefined) {
  if (v == null) return "—";
  if (kind === "ratio") return fmt(v);
  if (kind === "ms") return Math.round(v).toLocaleString() + " ms";
  if (kind === "usd") return "$" + v.toFixed(4);
  return String(v);
}

function AnswersLab() {
  const { t } = useTranslation();
  const search = Route.useSearch();
  const navigate = useNavigate({ from: Route.fullPath });
  const [picked, setPicked] = useState<number[]>([]);
  const runs = useQuery({
    queryKey: ["answers", "runs"],
    queryFn: adminApi.answerRuns,
    refetchInterval: (q) => (q.state.data?.some((r) => r.status === "running") ? 3000 : false),
  });
  const open = (run?: number, compare?: number) => void navigate({ search: { run, compare } });
  return (
    <div className="mx-auto max-w-7xl space-y-4 p-4 lg:p-6">
      <LabNav />
      <PageTitle title={t("answers.title")} subtitle={t("answers.subtitle")} />
      <StartRun onStarted={(id) => open(id)} />
      <Card>
        <CardHeader>
          <CardTitle>{t("answers.runs")}</CardTitle>
          <Button
            size="sm"
            variant="outline"
            disabled={picked.length !== 2}
            title={t("batch.compareHint")}
            onClick={() => open(picked[0], picked[1])}
          >
            {t("batch.compare")}
          </Button>
        </CardHeader>
        <CardContent>
          <ErrorLine error={runs.error} />
          {runs.isPending ? (
            <Skeleton />
          ) : runs.data?.length ? (
            <RunTable
              runs={runs.data}
              picked={picked}
              onPick={(id) => setPicked((p) => (p.includes(id) ? p.filter((x) => x !== id) : [...p, id].slice(-2)))}
              onOpen={(id) => open(id)}
              current={search.run}
            />
          ) : (
            <p className="text-sm text-muted-foreground">{t("batch.noRuns")}</p>
          )}
        </CardContent>
      </Card>
      {search.run != null && search.compare != null && <Compare a={search.run} b={search.compare} />}
      {search.run != null && search.compare == null && <RunDetail id={search.run} />}
    </div>
  );
}

function targetKey(t?: LLMTarget) {
  return t ? t.provider + "/" + t.model : "";
}

function StartRun({ onStarted }: { onStarted: (id: number) => void }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const sets = useQuery({ queryKey: ["goldsets"], queryFn: api.goldSets });
  const llm = useQuery({ queryKey: ["settings", "llm"], queryFn: chatApi.llm });
  const [set, setSet] = useState(0);
  const [label, setLabel] = useState("");
  const [answerModel, setAnswerModel] = useState("");
  const [routerModel, setRouterModel] = useState("");
  const [loop, setLoop] = useState("");
  const targets = (llm.data?.providers ?? []).flatMap((p) => p.models.map((m) => ({ provider: p.name, model: m })));
  const byKey = (k: string) => targets.find((x) => targetKey(x) === k);
  const start = useMutation({
    mutationFn: () => {
      const config: AnswerConfig = { roles: {} };
      if (answerModel) config.roles!.answer = [byKey(answerModel)!];
      if (routerModel === "off") config.roles!.router = [];
      else if (routerModel) config.roles!.router = [byKey(routerModel)!];
      if (loop) config.tool_loop = loop === "on";
      return adminApi.startAnswerRun({ gold_set_id: set || sets.data![0].id, label, config });
    },
    onSuccess: (r) => {
      onStarted(r.id);
      void qc.invalidateQueries({ queryKey: ["answers", "runs"] });
    },
  });
  const stored = (role: string) =>
    t("answers.stored", { m: (llm.data?.roles[role] ?? []).map(targetKey).join(", ") || t("answers.none") });
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("answers.start")}</CardTitle>
      </CardHeader>
      <CardContent>
        <form
          className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3"
          onSubmit={(e) => {
            e.preventDefault();
            start.mutate();
          }}
        >
          <Field label={t("batch.selectSet")}>
            <Select value={set} onChange={(e) => setSet(Number(e.target.value))} required>
              {(sets.data ?? []).map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name} v{s.version} · {t("batch.rows", { n: s.items })}
                </option>
              ))}
            </Select>
          </Field>
          <Field label={t("batch.label")}>
            <Input value={label} onChange={(e) => setLabel(e.target.value)} placeholder={t("answers.labelHint")} />
          </Field>
          <Field label={t("answers.toolLoop")}>
            <Select value={loop} onChange={(e) => setLoop(e.target.value)}>
              <option value="">{t("answers.loopDefault")}</option>
              <option value="on">{t("answers.loopOn")}</option>
              <option value="off">{t("answers.loopOff")}</option>
            </Select>
          </Field>
          <Field label={t("answers.answerModel")}>
            <Select value={answerModel} onChange={(e) => setAnswerModel(e.target.value)}>
              <option value="">{stored("answer")}</option>
              {targets.map((x) => (
                <option key={targetKey(x)} value={targetKey(x)}>
                  {targetKey(x)}
                </option>
              ))}
            </Select>
          </Field>
          <Field label={t("answers.routerModel")}>
            <Select value={routerModel} onChange={(e) => setRouterModel(e.target.value)}>
              <option value="">{stored("router")}</option>
              <option value="off">{t("answers.routerOff")}</option>
              {targets.map((x) => (
                <option key={targetKey(x)} value={targetKey(x)}>
                  {targetKey(x)}
                </option>
              ))}
            </Select>
          </Field>
          <div className="flex items-end">
            <Button type="submit" disabled={start.isPending || !sets.data?.length}>
              <Play />
              {t("answers.run")}
            </Button>
          </div>
        </form>
        <p className="mt-3 text-2xs text-muted-foreground">{t("answers.costNote")}</p>
        <ErrorLine error={start.error} />
      </CardContent>
    </Card>
  );
}

function StatusBadge({ r }: { r: AnswerRun }) {
  const { t } = useTranslation();
  if (r.status === "running")
    return <Badge variant="accent">{t("batch.running", { done: r.progress, total: r.total })}</Badge>;
  const v = { done: "in_force", failed: "expired", canceled: "unknown" } as const;
  return <Badge variant={v[r.status]}>{t("answers.status." + r.status)}</Badge>;
}

function modelsOf(c: AnswerConfig) {
  const a = (c.roles?.answer ?? []).map(targetKey).join(", ") || "—";
  const r = (c.roles?.router ?? []).map(targetKey).join(", ");
  return a + (r ? " · router " + r : "") + (c.tool_loop === false ? " · no loop" : "");
}

function RunTable({
  runs,
  picked,
  onPick,
  onOpen,
  current,
}: {
  runs: AnswerRun[];
  picked: number[];
  onPick: (id: number) => void;
  onOpen: (id: number) => void;
  current?: number;
}) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const cancel = useMutation({
    mutationFn: adminApi.cancelAnswerRun,
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["answers"] }),
  });
  return (
    <Table>
      <thead>
        <tr>
          <Th className="w-8">
            <span className="sr-only">{t("batch.compare")}</span>
          </Th>
          <Th>#</Th>
          <Th>{t("batch.label")}</Th>
          <Th>{t("batch.goldSets")}</Th>
          <Th>{t("answers.models")}</Th>
          <Th>{t("data.status")}</Th>
          <Th className="text-right">{t("answers.m.refusal_accuracy")}</Th>
          <Th className="text-right">{t("answers.m.supported")}</Th>
          <Th className="text-right">{t("answers.m.grounded")}</Th>
          <Th className="text-right">{t("answers.m.first_claim_p95_ms")}</Th>
          <Th className="text-right">{t("answers.m.cost_usd")}</Th>
          <Th />
        </tr>
      </thead>
      <tbody>
        {runs.map((r) => (
          <Tr key={r.id} className={cn(current === r.id && "bg-accent/50")}>
            <Td>
              <input
                type="checkbox"
                className="size-4 accent-[var(--primary)]"
                aria-label={t("answers.pick", { id: r.id })}
                checked={picked.includes(r.id)}
                onChange={() => onPick(r.id)}
              />
            </Td>
            <Td className="tabular-nums">{r.id}</Td>
            <Td className="font-medium">{r.label || "—"}</Td>
            <Td>{r.gold_set_name}</Td>
            <Td className="max-w-56 truncate" title={modelsOf(r.config)}>
              {modelsOf(r.config)}
            </Td>
            <Td>
              <StatusBadge r={r} />
            </Td>
            <Td className="text-right tabular-nums">{show("ratio", r.metrics?.refusal_accuracy)}</Td>
            <Td className="text-right tabular-nums">{show("ratio", r.metrics?.supported)}</Td>
            <Td className="text-right tabular-nums">{show("ratio", r.metrics?.grounded)}</Td>
            <Td className="text-right tabular-nums">{show("ms", r.metrics?.first_claim_p95_ms)}</Td>
            <Td className="text-right tabular-nums">{show("usd", r.metrics?.cost_usd)}</Td>
            <Td className="text-right whitespace-nowrap">
              <Button size="sm" variant="ghost" onClick={() => onOpen(r.id)}>
                {t("batch.openRun")}
              </Button>
              {r.status === "running" && (
                <Button size="sm" variant="ghost" onClick={() => cancel.mutate(r.id)} title={t("jobs.cancel")}>
                  <Ban />
                  <span className="sr-only">{t("jobs.cancel")}</span>
                </Button>
              )}
            </Td>
          </Tr>
        ))}
      </tbody>
    </Table>
  );
}

function MetricGrid({ m }: { m?: AnswerMetrics }) {
  const { t } = useTranslation();
  if (!m) return null;
  return (
    <div className="space-y-2">
      <p className="text-xs text-muted-foreground">
        {t("answers.counts", {
          q: m.questions,
          answered: m.answered,
          cards: m.cards,
          refused: m.refused,
          clarified: m.clarified,
          errors: m.errors,
        })}
      </p>
      <dl className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-6">
        {METRICS.map((x) => (
          <div key={x.key} className="rounded-md border p-2.5" title={t("answers.help." + x.key)}>
            <dt className="text-2xs text-muted-foreground">{t("answers.m." + x.key)}</dt>
            <dd className="text-base font-semibold tabular-nums">{show(x.kind, m[x.key] as number)}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

function useRun(id: number) {
  return useQuery({
    queryKey: ["answers", "run", id],
    queryFn: () => adminApi.answerRun(id),
    refetchInterval: (q) => (q.state.data?.run.status === "running" ? 3000 : false),
  });
}

const problem = (r: AnswerResult) =>
  r.score.outcome === "error" ||
  !r.score.refusal_correct ||
  r.score.dropped > 0 ||
  (!r.item.should_refuse && (r.item.expected_ids?.length ?? 0) > 0 && !r.score.grounded);

function RunDetail({ id }: { id: number }) {
  const { t } = useTranslation();
  const q = useRun(id);
  const [only, setOnly] = useState(true);
  const [openRow, setOpenRow] = useState<number | null>(null);
  if (q.error) return <ErrorLine error={q.error} />;
  if (!q.data) return <Skeleton rows={6} />;
  const { run, results } = q.data;
  const rows = results.filter((r) => !only || problem(r));
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {t("answers.run")} #{run.id} {run.label && "· " + run.label}
        </CardTitle>
        <span className="text-2xs text-muted-foreground">
          {run.gold_set_name} · corpus v{run.corpus_version} · {when(run.created_at)}
        </span>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-xs">
          <span className="text-muted-foreground">{t("answers.models")}:</span> {modelsOf(run.config)}
        </p>
        {run.error && <p className="text-sm text-destructive">{run.error}</p>}
        <MetricGrid m={run.metrics} />
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-semibold">{t("batch.perQuestion")}</h3>
          <Checkbox label={t("answers.problemsOnly")} checked={only} onChange={(e) => setOnly(e.target.checked)} />
        </div>
        <Table>
          <thead>
            <tr>
              <Th className="w-6" />
              <Th>ID</Th>
              <Th>{t("batch.question")}</Th>
              <Th>{t("answers.outcome")}</Th>
              <Th className="text-right">{t("answers.claims")}</Th>
              <Th className="text-right">{t("answers.cited")}</Th>
              <Th className="text-right">{t("answers.firstClaim")}</Th>
              <Th className="text-right">{t("answers.m.cost_usd")}</Th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <Fragment key={r.item.db_id}>
                <Tr
                  className="cursor-pointer"
                  onClick={() => setOpenRow(openRow === r.item.db_id ? null : r.item.db_id)}
                >
                  <Td>
                    {openRow === r.item.db_id ? (
                      <ChevronDown className="size-3.5" />
                    ) : (
                      <ChevronRight className="size-3.5" />
                    )}
                  </Td>
                  <Td className="font-mono">{r.item.id}</Td>
                  <Td className="min-w-64">
                    {r.item.question}
                    {r.item.should_refuse && (
                      <Badge variant="outline" className="ml-1.5">
                        {t("batch.refuse")}
                      </Badge>
                    )}
                  </Td>
                  <Td>
                    <OutcomeBadge r={r} />
                  </Td>
                  <Td className="text-right tabular-nums">
                    {r.score.claims}
                    {r.score.dropped > 0 && <span className="text-destructive"> −{r.score.dropped}</span>}
                  </Td>
                  <Td className="text-right tabular-nums">
                    {r.item.expected_ids?.length ? `${r.score.citations_correct}/${r.score.citations}` : "—"}
                  </Td>
                  <Td className="text-right tabular-nums">{show("ms", r.score.first_claim_ms || undefined)}</Td>
                  <Td className="text-right tabular-nums">{show("usd", r.score.cost_usd)}</Td>
                </Tr>
                {openRow === r.item.db_id && (
                  <tr>
                    <td colSpan={8} className="border-b bg-surface-2 p-3">
                      <ResultBody r={r} />
                    </td>
                  </tr>
                )}
              </Fragment>
            ))}
          </tbody>
        </Table>
        {rows.length === 0 && <p className="text-sm text-muted-foreground">{t("answers.noProblems")}</p>}
      </CardContent>
    </Card>
  );
}

function OutcomeBadge({ r }: { r: AnswerResult }) {
  const { t } = useTranslation();
  const ok = r.score.outcome !== "error" && r.score.refusal_correct;
  return (
    <Badge variant={ok ? "in_force" : "expired"}>
      {t("answers.outcomes." + r.score.outcome)}
      {r.score.refusal_code && ` (${r.score.refusal_code})`}
    </Badge>
  );
}

function ResultBody({ r }: { r: AnswerResult }) {
  const { t } = useTranslation();
  const res = r.result;
  return (
    <div className="space-y-3 text-sm">
      {r.item.expected_provisions && (
        <p className="text-xs">
          <span className="text-muted-foreground">{t("batch.expected")}:</span> {r.item.expected_provisions}
        </p>
      )}
      {r.score.error && <p className="text-destructive">{r.score.error}</p>}
      {res.clarify && <p className="italic">{res.clarify}</p>}
      {res.refusal && <RefusalView refusal={res.refusal} />}
      {res.claims?.map((c, i) => <ClaimView key={i} claim={c} />)}
      {res.dropped && res.dropped.length > 0 && (
        <ul className="space-y-1 text-xs">
          {res.dropped.map((d, i) => (
            <li key={i} className="rounded border p-2">
              <Badge variant="expired" className="mr-1.5">
                {d.reason}
              </Badge>
              {d.text}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function Compare({ a, b }: { a: number; b: number }) {
  const { t } = useTranslation();
  const qa = useRun(a);
  const qb = useRun(b);
  if (qa.error || qb.error) return <ErrorLine error={qa.error ?? qb.error} />;
  if (!qa.data || !qb.data) return <Skeleton rows={6} />;
  const A = qa.data;
  const B = qb.data;
  const byItem = new Map(B.results.map((r) => [r.item.db_id, r]));
  const changed = A.results
    .map((ra) => [ra, byItem.get(ra.item.db_id)] as const)
    .filter(([ra, rb]) => rb && (ra.score.outcome !== rb.score.outcome || ra.score.grounded !== rb.score.grounded));
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("answers.compareTitle", { a, b })}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <Table>
          <thead>
            <tr>
              <Th>{t("batch.metrics")}</Th>
              <Th className="text-right">
                #{a} <span className="font-normal normal-case">{modelsOf(A.run.config)}</span>
              </Th>
              <Th className="text-right">
                #{b} <span className="font-normal normal-case">{modelsOf(B.run.config)}</span>
              </Th>
              <Th className="text-right">{t("batch.delta")}</Th>
            </tr>
          </thead>
          <tbody>
            {METRICS.map((x) => {
              const va = A.run.metrics?.[x.key] as number | undefined;
              const vb = B.run.metrics?.[x.key] as number | undefined;
              const d = va != null && vb != null ? vb - va : undefined;
              const good = d == null || d === 0 ? undefined : d > 0 === (x.better === "up");
              return (
                <Tr key={x.key}>
                  <Td>{t("answers.m." + x.key)}</Td>
                  <Td className="text-right tabular-nums">{show(x.kind, va)}</Td>
                  <Td className="text-right tabular-nums">{show(x.kind, vb)}</Td>
                  <Td
                    className={cn(
                      "text-right tabular-nums",
                      good === true && "text-force-in",
                      good === false && "text-destructive",
                    )}
                  >
                    {d == null ? "—" : (d > 0 ? "+" : "") + show(x.kind, d)}
                  </Td>
                </Tr>
              );
            })}
          </tbody>
        </Table>
        <h3 className="text-sm font-semibold">{t("answers.changed", { n: changed.length })}</h3>
        {changed.length > 0 && (
          <Table>
            <thead>
              <tr>
                <Th>ID</Th>
                <Th>{t("batch.question")}</Th>
                <Th>#{a}</Th>
                <Th>#{b}</Th>
              </tr>
            </thead>
            <tbody>
              {changed.map(([ra, rb]) => (
                <Tr key={ra.item.db_id}>
                  <Td className="font-mono">{ra.item.id}</Td>
                  <Td className="min-w-64">{ra.item.question}</Td>
                  <Td>
                    <OutcomeBadge r={ra} />
                  </Td>
                  <Td>
                    <OutcomeBadge r={rb!} />
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}
