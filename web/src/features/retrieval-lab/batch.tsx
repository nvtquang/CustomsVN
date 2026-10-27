import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Play, Upload } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox, Field, Input, Select, Textarea } from "@/components/ui/form";
import { Table, Td, Th, Tr } from "@/components/ui/table";
import { api } from "@/lib/api/client";
import type { Filters, ImportResult, Metrics, Run, RunResult, SearchConfig } from "@/lib/api/types";
import { cn, fmt } from "@/lib/utils";
import { useUI } from "@/stores/ui";

export function Batch({ filters, config }: { filters: Filters; config: SearchConfig }) {
  const [open, setOpen] = useState<number | null>(null);
  const [compare, setCompare] = useState<number[]>([]);
  return (
    <div className="space-y-4">
      <div className="grid gap-4 xl:grid-cols-2">
        <GoldSetImport />
        <StartRun filters={filters} config={config} onStarted={setOpen} />
      </div>
      <RunsTable open={open} onOpen={setOpen} selected={compare} onSelect={setCompare} />
      {compare.length === 2 && <RunCompare a={compare[0]} b={compare[1]} />}
      {open != null && <RunDetail id={open} />}
    </div>
  );
}

function GoldSetImport() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [note, setNote] = useState("");
  const [content, setContent] = useState("");
  const [format, setFormat] = useState("");
  const [preview, setPreview] = useState<ImportResult | null>(null);
  const call = useMutation({
    mutationFn: (opts: { dry: boolean; force?: boolean }) =>
      api.importGoldSet({ name, note, content, format, dry_run: opts.dry, force: opts.force }),
    onSuccess: (r, opts) => {
      setPreview(r);
      if (!opts.dry) void qc.invalidateQueries({ queryKey: ["goldsets"] });
    },
  });
  const errors = preview?.errors ?? [];
  const onFile = async (f: File | undefined) => {
    if (!f) return;
    setContent(await f.text());
    setFormat(f.name.toLowerCase().endsWith(".json") ? "json" : "csv");
    if (!name) setName(f.name.replace(/\.(csv|json)$/i, ""));
    setPreview(null);
  };
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("batch.import")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label={t("batch.name")}>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </Field>
          <Field label={t("batch.file")}>
            <Input
              type="file"
              accept=".csv,.json,text/csv,application/json"
              onChange={(e) => onFile(e.target.files?.[0])}
            />
          </Field>
          <Field label={t("batch.note")} className="sm:col-span-2">
            <Input value={note} onChange={(e) => setNote(e.target.value)} />
          </Field>
          <Field label={t("batch.paste")} className="sm:col-span-2">
            <Textarea
              className="min-h-24 font-mono text-2xs"
              value={content}
              onChange={(e) => {
                setContent(e.target.value);
                setPreview(null);
              }}
            />
          </Field>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" disabled={!content || call.isPending} onClick={() => call.mutate({ dry: true })}>
            {t("batch.preview")}
          </Button>
          <Button disabled={!content || !name || call.isPending} onClick={() => call.mutate({ dry: false })}>
            <Upload /> {t("batch.save")}
          </Button>
          {errors.length > 0 && (
            <Button
              variant="secondary"
              disabled={!name || call.isPending}
              onClick={() => call.mutate({ dry: false, force: true })}
            >
              {t("batch.saveAnyway")}
            </Button>
          )}
        </div>
        {call.error && <p className="text-xs text-destructive">{t("common.error", { msg: call.error.message })}</p>}
        {preview && (
          <div className="space-y-2">
            <div className="flex gap-2 text-xs">
              <Badge>{t("batch.rows", { n: preview.count })}</Badge>
              <Badge variant={errors.length ? "expired" : "in_force"}>
                {errors.length ? t("batch.rowErrors", { n: errors.length }) : t("batch.noErrors")}
              </Badge>
              {preview.gold_set && (
                <Badge variant="accent">
                  {preview.gold_set.name} v{preview.gold_set.version}
                </Badge>
              )}
            </div>
            {errors.length > 0 && (
              <Table>
                <thead>
                  <tr>
                    <Th>{t("batch.row")}</Th>
                    <Th>ID</Th>
                    <Th>{t("batch.field")}</Th>
                    <Th>{t("batch.message")}</Th>
                  </tr>
                </thead>
                <tbody>
                  {errors.map((e, i) => (
                    <Tr key={i}>
                      <Td>{e.row}</Td>
                      <Td>{e.id}</Td>
                      <Td>{e.field}</Td>
                      <Td>{e.message}</Td>
                    </Tr>
                  ))}
                </tbody>
              </Table>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function StartRun({
  filters,
  config,
  onStarted,
}: {
  filters: Filters;
  config: SearchConfig;
  onStarted: (id: number) => void;
}) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const sets = useQuery({ queryKey: ["goldsets"], queryFn: api.goldSets });
  const [setID, setSetID] = useState<number | null>(null);
  const [label, setLabel] = useState("");
  const chosen = setID ?? sets.data?.[0]?.id ?? null;
  const start = useMutation({
    mutationFn: () => api.startRun({ gold_set_id: chosen!, label, request: { query: "", filters, config } }),
    onSuccess: (r) => {
      onStarted(r.id);
      void qc.invalidateQueries({ queryKey: ["runs"] });
    },
  });
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("batch.goldSets")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {sets.data && sets.data.length === 0 && <p className="text-xs text-muted-foreground">{t("batch.empty")}</p>}
        {sets.data && sets.data.length > 0 && (
          <Table>
            <thead>
              <tr>
                <Th>{t("batch.name")}</Th>
                <Th>{t("batch.version")}</Th>
                <Th>{t("batch.items")}</Th>
                <Th>{t("batch.created")}</Th>
              </tr>
            </thead>
            <tbody>
              {sets.data.map((g) => (
                <Tr
                  key={g.id}
                  className={cn("cursor-pointer", chosen === g.id && "bg-accent/60")}
                  onClick={() => setSetID(g.id)}
                >
                  <Td className="font-medium">{g.name}</Td>
                  <Td>v{g.version}</Td>
                  <Td>{g.items}</Td>
                  <Td>{new Date(g.created_at).toLocaleString()}</Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )}
        <div className="grid gap-3 sm:grid-cols-[1fr_1fr_auto] sm:items-end">
          <Field label={t("batch.selectSet")}>
            <Select value={chosen ?? ""} onChange={(e) => setSetID(Number(e.target.value))}>
              {sets.data?.map((g) => (
                <option key={g.id} value={g.id}>
                  {g.name} v{g.version}
                </option>
              ))}
            </Select>
          </Field>
          <Field label={t("batch.label")}>
            <Input value={label} onChange={(e) => setLabel(e.target.value)} placeholder={config.mode} />
          </Field>
          <Button disabled={chosen == null || start.isPending} onClick={() => start.mutate()}>
            <Play /> {t("batch.startRun")}
          </Button>
        </div>
        <p className="font-mono text-2xs text-muted-foreground">
          mode={config.mode} k={config.k} rrf_k={config.rrf_k} keep_originals={String(config.keep_originals)}{" "}
          ignore_refs=
          {String(config.ignore_refs)} as_of={filters.as_of}
        </p>
        {start.error && <p className="text-xs text-destructive">{t("common.error", { msg: start.error.message })}</p>}
      </CardContent>
    </Card>
  );
}

function RunsTable({
  open,
  onOpen,
  selected,
  onSelect,
}: {
  open: number | null;
  onOpen: (id: number) => void;
  selected: number[];
  onSelect: (ids: number[]) => void;
}) {
  const { t } = useTranslation();
  const runs = useQuery({
    queryKey: ["runs"],
    queryFn: api.runs,
    refetchInterval: (q) => (q.state.data?.some((r) => r.status === "running") ? 1500 : false),
  });
  const toggle = (id: number) =>
    onSelect(selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id].slice(-2));
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("batch.runs")}</CardTitle>
        <span className="text-2xs text-muted-foreground">{t("batch.compareHint")}</span>
      </CardHeader>
      <CardContent className="p-0">
        {runs.data && runs.data.length === 0 && (
          <p className="p-4 text-xs text-muted-foreground">{t("batch.noRuns")}</p>
        )}
        {runs.data && runs.data.length > 0 && (
          <Table>
            <thead>
              <tr>
                <Th />
                <Th>#</Th>
                <Th>{t("batch.label")}</Th>
                <Th>{t("batch.goldSets")}</Th>
                <Th>{t("batch.corpus")}</Th>
                <Th>{t("metric.recall10")}</Th>
                <Th>{t("metric.mrr")}</Th>
                <Th>{t("metric.ndcg10")}</Th>
                <Th>{t("metric.expired")}</Th>
                <Th>{t("metric.p95")}</Th>
                <Th />
              </tr>
            </thead>
            <tbody>
              {runs.data.map((r) => (
                <Tr key={r.id} className={cn(open === r.id && "bg-accent/40")}>
                  <Td>
                    <input
                      type="checkbox"
                      aria-label={t("batch.compare")}
                      className="size-4 accent-[var(--primary)]"
                      checked={selected.includes(r.id)}
                      onChange={() => toggle(r.id)}
                    />
                  </Td>
                  <Td className="font-mono">{r.id}</Td>
                  <Td>
                    {r.label || r.request.config.mode}
                    <RunStatus run={r} />
                  </Td>
                  <Td>{r.gold_set_name}</Td>
                  <Td>v{r.corpus_version}</Td>
                  <Td className="font-mono">{fmt(r.metrics?.recall["10"])}</Td>
                  <Td className="font-mono">{fmt(r.metrics?.mrr)}</Td>
                  <Td className="font-mono">{fmt(r.metrics?.ndcg10)}</Td>
                  <Td className={cn("font-mono", (r.metrics?.expired_rate ?? 0) > 0 && "text-force-expired")}>
                    {fmt(r.metrics?.expired_rate)}
                  </Td>
                  <Td className="font-mono">{fmt(r.metrics?.latency_p95_ms, 0)}</Td>
                  <Td>
                    <Button variant="link" size="sm" onClick={() => onOpen(r.id)}>
                      {t("batch.openRun")}
                    </Button>
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

function RunStatus({ run }: { run: Run }) {
  const { t } = useTranslation();
  if (run.status === "running") {
    const pct = run.total ? (100 * run.progress) / run.total : 0;
    return (
      <div className="mt-1 w-32">
        <div className="h-1.5 overflow-hidden rounded-full bg-muted">
          <div className="h-full bg-primary transition-all" style={{ width: pct + "%" }} />
        </div>
        <span className="text-2xs text-muted-foreground">
          {t("batch.running", { done: run.progress, total: run.total })}
        </span>
      </div>
    );
  }
  if (run.status === "failed")
    return (
      <Badge variant="expired" className="ml-1.5" title={run.error}>
        {t("batch.failed")}
      </Badge>
    );
  return null;
}

const metricRows: { key: string; get: (m: Metrics) => number; digits?: number; lowerIsBetter?: boolean }[] = [
  { key: "recall1", get: (m) => m.recall["1"] },
  { key: "recall3", get: (m) => m.recall["3"] },
  { key: "recall5", get: (m) => m.recall["5"] },
  { key: "recall10", get: (m) => m.recall["10"] },
  { key: "hit10", get: (m) => m.hit_at_10 },
  { key: "mrr", get: (m) => m.mrr },
  { key: "ndcg10", get: (m) => m.ndcg10 },
  { key: "expired", get: (m) => m.expired_rate, lowerIsBetter: true },
  { key: "p50", get: (m) => m.latency_p50_ms, digits: 0, lowerIsBetter: true },
  { key: "p95", get: (m) => m.latency_p95_ms, digits: 0, lowerIsBetter: true },
];

function MetricGrid({ m }: { m: Metrics }) {
  const { t } = useTranslation();
  return (
    <div className="grid grid-cols-2 gap-2 sm:grid-cols-5">
      {metricRows.map((r) => (
        <div key={r.key} className="rounded-md border bg-surface-2 px-3 py-2">
          <div className="text-2xs text-muted-foreground">{t("metric." + r.key)}</div>
          <div className="font-mono text-lg font-semibold">{fmt(r.get(m), r.digits ?? 3)}</div>
        </div>
      ))}
    </div>
  );
}

function RunDetail({ id }: { id: number }) {
  const { t } = useTranslation();
  const openChunk = useUI((s) => s.openChunk);
  const [missedOnly, setMissedOnly] = useState(false);
  const q = useQuery({
    queryKey: ["run", id],
    queryFn: () => api.run(id),
    refetchInterval: (q) => (q.state.data?.run.status === "running" ? 1500 : false),
  });
  if (!q.data) return null;
  const { run } = q.data;
  const results = (q.data.results ?? []).filter((r) => !missedOnly || missed(r));
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          #{run.id} {run.label} · {run.gold_set_name} · {t("batch.corpus")} v{run.corpus_version}
        </CardTitle>
        <Checkbox
          label={t("batch.missedOnly")}
          checked={missedOnly}
          onChange={(e) => setMissedOnly(e.target.checked)}
        />
      </CardHeader>
      <CardContent className="space-y-4">
        {run.metrics && <MetricGrid m={run.metrics} />}
        <Table>
          <thead>
            <tr>
              <Th>ID</Th>
              <Th>{t("batch.question")}</Th>
              <Th>{t("batch.expected")}</Th>
              <Th>{t("batch.found")}</Th>
              <Th>R@10</Th>
              <Th>RR</Th>
              <Th>Top 3</Th>
            </tr>
          </thead>
          <tbody>
            {results.map((r) => (
              <Tr key={r.item.db_id} className={cn(missed(r) && "bg-force-expired-bg/40")}>
                <Td className="font-mono">{r.item.id}</Td>
                <Td className="max-w-80">
                  {r.item.question}
                  {r.item.should_refuse && (
                    <Badge variant="outline" className="ml-1.5">
                      {t("batch.refuse")}
                    </Badge>
                  )}
                </Td>
                <Td className="max-w-64 text-muted-foreground">{r.item.expected_provisions}</Td>
                <Td className="font-mono">{r.ranks.length ? r.ranks.map((x) => (x ? x : "—")).join(", ") : "—"}</Td>
                <Td className="font-mono">{fmt(r.metrics.recall?.["10"], 2)}</Td>
                <Td className="font-mono">{fmt(r.metrics.rr, 2)}</Td>
                <Td className="space-y-0.5">
                  {(r.hits ?? []).slice(0, 3).map((h) => (
                    <button
                      key={h.chunk_id}
                      type="button"
                      onClick={() => openChunk(h.chunk_id)}
                      className={cn(
                        "block max-w-64 cursor-pointer truncate text-left hover:underline",
                        h.relevant ? "text-force-in" : "text-muted-foreground",
                      )}
                    >
                      {h.rank}. {h.number} {h.path}
                    </button>
                  ))}
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      </CardContent>
    </Card>
  );
}

function missed(r: RunResult) {
  return !r.item.should_refuse && (r.metrics.recall?.["10"] ?? 0) === 0;
}

function RunCompare({ a, b }: { a: number; b: number }) {
  const { t } = useTranslation();
  const qa = useQuery({ queryKey: ["run", a], queryFn: () => api.run(a) });
  const qb = useQuery({ queryKey: ["run", b], queryFn: () => api.run(b) });
  if (!qa.data?.run.metrics || !qb.data?.run.metrics) return null;
  const ma = qa.data.run.metrics;
  const mb = qb.data.run.metrics;
  const byItem = new Map((qa.data.results ?? []).map((r) => [r.item.id, r]));
  const changed = (qb.data.results ?? [])
    .map((r) => ({ id: r.item.id, q: r.item.question, a: byItem.get(r.item.id)?.metrics.rr ?? 0, b: r.metrics.rr }))
    .filter((x) => x.a !== x.b);
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {t("batch.compare")}: #{a} → #{b}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <Table>
          <thead>
            <tr>
              <Th>{t("batch.metrics")}</Th>
              <Th>#{a}</Th>
              <Th>#{b}</Th>
              <Th>{t("batch.delta")}</Th>
            </tr>
          </thead>
          <tbody>
            {metricRows.map((r) => {
              const d = r.get(mb) - r.get(ma);
              const better = r.lowerIsBetter ? d < 0 : d > 0;
              return (
                <Tr key={r.key}>
                  <Td>{t("metric." + r.key)}</Td>
                  <Td className="font-mono">{fmt(r.get(ma), r.digits ?? 3)}</Td>
                  <Td className="font-mono">{fmt(r.get(mb), r.digits ?? 3)}</Td>
                  <Td className={cn("font-mono", d !== 0 && (better ? "text-force-in" : "text-force-expired"))}>
                    {d > 0 ? "+" : ""}
                    {fmt(d, r.digits ?? 3)}
                  </Td>
                </Tr>
              );
            })}
          </tbody>
        </Table>
        {changed.length > 0 && (
          <Table>
            <thead>
              <tr>
                <Th>ID</Th>
                <Th>{t("batch.question")}</Th>
                <Th>RR #{a}</Th>
                <Th>RR #{b}</Th>
              </tr>
            </thead>
            <tbody>
              {changed.map((x) => (
                <Tr key={x.id}>
                  <Td className="font-mono">{x.id}</Td>
                  <Td>{x.q}</Td>
                  <Td className="font-mono">{fmt(x.a, 2)}</Td>
                  <Td className={cn("font-mono", x.b > x.a ? "text-force-in" : "text-force-expired")}>{fmt(x.b, 2)}</Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}
