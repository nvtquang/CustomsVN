import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { Ban, CalendarClock, Play, Save } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox, Field, Input, Select } from "@/components/ui/form";
import { Table, Td, Th, Tr } from "@/components/ui/table";
import { AdminOnly, ErrorLine, PageTitle, Skeleton } from "@/features/admin/admin-only";
import { adminApi, type CorpusAdmin, type CountRow, type CrawlSchedule, type Job } from "@/lib/api/admin";
import { cn, when } from "@/lib/utils";

export const Route = createFileRoute("/admin/data")({
  component: () => (
    <AdminOnly>
      <DataPage />
    </AdminOnly>
  ),
});

function DataPage() {
  const { t } = useTranslation();
  const corpus = useQuery({ queryKey: ["admin", "corpus"], queryFn: adminApi.corpus });
  return (
    <div className="mx-auto max-w-6xl space-y-4 p-4 lg:p-6">
      <PageTitle title={t("data.title")} subtitle={t("data.subtitle")} />
      <ErrorLine error={corpus.error} />
      {corpus.isPending ? <Skeleton rows={6} /> : corpus.data && <Overview c={corpus.data} />}
      <Crawl />
      <Jobs models={corpus.data?.embeddings.map((e) => e.model) ?? []} />
      {corpus.data && <Coverage c={corpus.data} />}
      {corpus.data && <Versions c={corpus.data} />}
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="rounded-md border bg-surface p-3">
      <div className="text-2xs text-muted-foreground">{label}</div>
      <div className="text-lg font-semibold tabular-nums">
        {typeof value === "number" ? value.toLocaleString() : value}
      </div>
    </div>
  );
}

function Overview({ c }: { c: CorpusAdmin }) {
  const { t } = useTranslation();
  const cur = c.current;
  return (
    <>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
        <Stat label={t("data.version")} value={"v" + cur.version} />
        <Stat label={t("data.documents")} value={cur.documents} />
        <Stat label={t("data.chunks")} value={cur.chunks} />
        <Stat label={t("data.registry")} value={c.registry} />
        <Stat label={t("data.edges")} value={c.edges} />
        <Stat label={t("data.hsLines")} value={c.hs_lines} />
      </div>
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <Breakdown
          title={t("data.byForce", { day: cur.today })}
          rows={cur.by_force}
          label={(k) => t("force." + (k || "unknown"))}
        />
        <Breakdown title={t("data.byTier")} rows={cur.by_tier} label={(k) => t("tier." + k)} />
        <Breakdown title={t("data.bySource")} rows={cur.by_source} label={(k) => t("source." + k)} />
        <Breakdown title={t("data.byType")} rows={cur.by_type.slice(0, 8)} label={(k) => k || "—"} />
      </div>
      <Card>
        <CardHeader>
          <CardTitle>{t("data.embeddings")}</CardTitle>
        </CardHeader>
        <CardContent>
          {c.embeddings.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("data.noEmbeddings")}</p>
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>{t("data.model")}</Th>
                  <Th>{t("data.provider")}</Th>
                  <Th className="text-right">{t("data.dims")}</Th>
                  <Th className="text-right">{t("data.chunksEmbedded")}</Th>
                  <Th className="text-right">{t("data.hsEmbedded")}</Th>
                  <Th>{t("data.loaded")}</Th>
                </tr>
              </thead>
              <tbody>
                {c.embeddings.map((e) => (
                  <Tr key={e.model}>
                    <Td className="font-medium">{e.model}</Td>
                    <Td>{e.provider}</Td>
                    <Td className="text-right tabular-nums">{e.dims}</Td>
                    <Td className="text-right tabular-nums">
                      {e.chunks.toLocaleString()} / {cur.chunks.toLocaleString()}
                    </Td>
                    <Td className="text-right tabular-nums">
                      {e.hs.toLocaleString()} / {c.hs_lines.toLocaleString()}
                    </Td>
                    <Td>{e.loaded ? t("common.yes") : t("data.noKey")}</Td>
                  </Tr>
                ))}
              </tbody>
            </Table>
          )}
        </CardContent>
      </Card>
    </>
  );
}

function Breakdown({ title, rows, label }: { title: string; rows: CountRow[]; label: (k: string) => string }) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-xs">{title}</CardTitle>
      </CardHeader>
      <CardContent className="p-0">
        <Table>
          <thead>
            <tr>
              <Th />
              <Th className="text-right">{t("data.documents")}</Th>
              <Th className="text-right">{t("data.chunks")}</Th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <Tr key={r.key}>
                <Td>{label(r.key)}</Td>
                <Td className="text-right tabular-nums">{r.documents.toLocaleString()}</Td>
                <Td className="text-right tabular-nums">{r.chunks.toLocaleString()}</Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      </CardContent>
    </Card>
  );
}

const active = (j: Job) => j.status === "queued" || j.status === "running";

/** The weekly update from vbpl.vn: when it runs, and a button to run it now. */
function Crawl() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["admin", "crawl"], queryFn: adminApi.crawl });
  const [draft, setDraft] = useState<CrawlSchedule | null>(null);
  const s = draft ?? q.data;
  const save = useMutation({
    mutationFn: (v: CrawlSchedule) => adminApi.saveCrawl(v),
    onSuccess: (v) => {
      qc.setQueryData(["admin", "crawl"], v);
      setDraft(null);
    },
  });
  const run = useMutation({
    mutationFn: () => adminApi.startJob("crawl", { max_pages: s?.max_pages }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["admin", "jobs"] }),
  });
  const set = (p: Partial<CrawlSchedule>) => s && setDraft({ ...s, ...p });
  const days = [1, 2, 3, 4, 5, 6, 0];
  return (
    <Card>
      <CardHeader>
        <div>
          <CardTitle className="flex items-center gap-2">
            <CalendarClock className="size-4 text-muted-foreground" /> {t("crawl.title")}
          </CardTitle>
          <p className="mt-0.5 text-xs text-muted-foreground">{t("crawl.text")}</p>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        <ErrorLine error={q.error} />
        {s && !s.installed && (
          <p className="rounded-md border border-force-partly/40 p-2.5 text-xs text-force-partly">
            {t("crawl.missing")}
          </p>
        )}
        {s && (
          <form
            className="flex flex-wrap items-end gap-3"
            onSubmit={(e) => {
              e.preventDefault();
              save.mutate(s);
            }}
          >
            <Checkbox
              label={t("crawl.enabled")}
              checked={s.enabled}
              onChange={(e) => set({ enabled: e.target.checked })}
              className="h-10"
            />
            <Field label={t("crawl.every")}>
              <Select
                value={s.every}
                onChange={(e) => set({ every: e.target.value as CrawlSchedule["every"] })}
                className="w-36"
              >
                <option value="monthly">{t("crawl.monthly")}</option>
                <option value="weekly">{t("crawl.weekly")}</option>
              </Select>
            </Field>
            {s.every === "weekly" ? (
              <Field label={t("crawl.weekday")}>
                <Select value={s.weekday} onChange={(e) => set({ weekday: Number(e.target.value) })} className="w-36">
                  {days.map((d) => (
                    <option key={d} value={d}>
                      {t("crawl.days." + d)}
                    </option>
                  ))}
                </Select>
              </Field>
            ) : (
              <Field label={t("crawl.day")}>
                <Select value={s.day} onChange={(e) => set({ day: Number(e.target.value) })} className="w-28">
                  {Array.from({ length: 28 }, (_, i) => i + 1).map((d) => (
                    <option key={d} value={d}>
                      {d}
                    </option>
                  ))}
                </Select>
              </Field>
            )}
            <Field label={t("crawl.hour")}>
              <Select value={s.hour} onChange={(e) => set({ hour: Number(e.target.value) })} className="w-28">
                {Array.from({ length: 24 }, (_, h) => (
                  <option key={h} value={h}>
                    {String(h).padStart(2, "0")}:00
                  </option>
                ))}
              </Select>
            </Field>
            <Field label={t("crawl.maxPages")}>
              <Input
                type="number"
                min={1}
                max={200}
                value={s.max_pages}
                onChange={(e) => set({ max_pages: Number(e.target.value) })}
                className="w-28"
              />
            </Field>
            <Button type="submit" variant="outline" disabled={!draft || save.isPending}>
              <Save /> {t("crawl.save")}
            </Button>
            <Button type="button" disabled={!s.installed || run.isPending} onClick={() => run.mutate()}>
              <Play /> {t("crawl.runNow")}
            </Button>
          </form>
        )}
        <p className="text-2xs text-muted-foreground">{t("crawl.help." + (s?.every ?? "monthly"))}</p>
        <ErrorLine error={save.error ?? run.error} />
      </CardContent>
    </Card>
  );
}

function Jobs({ models }: { models: string[] }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [selected, setSelected] = useState<number | null>(null);
  const jobs = useQuery({
    queryKey: ["admin", "jobs"],
    queryFn: adminApi.jobs,
    refetchInterval: (q) => (q.state.data?.some(active) ? 2000 : false),
  });
  // When a job finishes, the corpus figures above may have changed.
  const running = jobs.data?.some(active) ?? false;
  const was = useRef(running);
  useEffect(() => {
    if (was.current && !running) void qc.invalidateQueries({ queryKey: ["admin", "corpus"] });
    was.current = running;
  }, [running, qc]);
  const shown = selected ?? jobs.data?.[0]?.id ?? null;
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("jobs.title")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <StartJob models={models} onStarted={(id) => setSelected(id)} />
        <ErrorLine error={jobs.error} />
        {jobs.isPending ? (
          <Skeleton rows={3} />
        ) : jobs.data && jobs.data.length > 0 ? (
          <div className="grid gap-4 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
            <ul className="max-h-96 space-y-1 overflow-y-auto" aria-label={t("jobs.list")}>
              {jobs.data.map((j) => (
                <li key={j.id}>
                  <button
                    type="button"
                    onClick={() => setSelected(j.id)}
                    aria-current={shown === j.id}
                    className={cn(
                      "w-full cursor-pointer rounded-md border p-2 text-left text-xs hover:bg-muted",
                      shown === j.id && "border-primary bg-accent",
                    )}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-medium">
                        #{j.id} {t("jobs.kinds." + j.kind)}
                      </span>
                      <JobStatusBadge status={j.status} />
                    </div>
                    <div className="mt-1 truncate text-muted-foreground">{j.progress || when(j.created_at)}</div>
                  </button>
                </li>
              ))}
            </ul>
            {shown != null && <JobLog id={shown} />}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">{t("jobs.none")}</p>
        )}
      </CardContent>
    </Card>
  );
}

function JobStatusBadge({ status }: { status: Job["status"] }) {
  const { t } = useTranslation();
  const variant = {
    queued: "outline",
    running: "accent",
    done: "in_force",
    failed: "expired",
    canceled: "unknown",
  } as const;
  return <Badge variant={variant[status]}>{t("jobs.status." + status)}</Badge>;
}

function StartJob({ models, onStarted }: { models: string[]; onStarted: (id: number) => void }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [kind, setKind] = useState("ingest");
  const [asOf, setAsOf] = useState("");
  const [legacy, setLegacy] = useState(true);
  const [model, setModel] = useState("");
  const [target, setTarget] = useState("chunks");
  const [limit, setLimit] = useState(0);
  const start = useMutation({
    mutationFn: () => {
      const params: Record<string, unknown> =
        kind === "prep"
          ? { as_of: asOf || undefined, legacy }
          : kind === "embed"
            ? { model: model || models[0], target, limit: limit || undefined }
            : {};
      return adminApi.startJob(kind, params);
    },
    onSuccess: (r) => {
      onStarted(r.id);
      void qc.invalidateQueries({ queryKey: ["admin", "jobs"] });
    },
  });
  return (
    <form
      className="flex flex-wrap items-end gap-3 rounded-md border bg-surface-2 p-3"
      onSubmit={(e) => {
        e.preventDefault();
        start.mutate();
      }}
    >
      <Field label={t("jobs.kind")}>
        <Select value={kind} onChange={(e) => setKind(e.target.value)} className="w-40">
          <option value="prep">{t("jobs.kinds.prep")}</option>
          <option value="ingest">{t("jobs.kinds.ingest")}</option>
          <option value="embed" disabled={models.length === 0}>
            {t("jobs.kinds.embed")}
          </option>
          <option value="crawl">{t("jobs.kinds.crawl")}</option>
        </Select>
      </Field>
      {kind === "prep" && (
        <>
          <Field label={t("jobs.asOf")}>
            <Input type="date" value={asOf} onChange={(e) => setAsOf(e.target.value)} className="w-40" />
          </Field>
          <Checkbox
            label={t("jobs.legacy")}
            checked={legacy}
            onChange={(e) => setLegacy(e.target.checked)}
            className="h-9"
          />
        </>
      )}
      {kind === "embed" && (
        <>
          <Field label={t("data.model")}>
            <Select value={model || models[0]} onChange={(e) => setModel(e.target.value)} className="w-48">
              {models.map((m) => (
                <option key={m} value={m}>
                  {m}
                </option>
              ))}
            </Select>
          </Field>
          <Field label={t("jobs.target")}>
            <Select value={target} onChange={(e) => setTarget(e.target.value)} className="w-40">
              <option value="chunks">{t("jobs.targets.chunks")}</option>
              <option value="hs">{t("jobs.targets.hs")}</option>
            </Select>
          </Field>
          <Field label={t("jobs.limit")}>
            <Input
              type="number"
              min={0}
              value={limit}
              onChange={(e) => setLimit(Number(e.target.value))}
              className="w-28"
            />
          </Field>
        </>
      )}
      <Button type="submit" disabled={start.isPending}>
        <Play />
        {t("jobs.start")}
      </Button>
      <p className="basis-full text-2xs text-muted-foreground">{t("jobs.help." + kind)}</p>
      <ErrorLine error={start.error} />
    </form>
  );
}

function JobLog({ id }: { id: number }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const job = useQuery({
    queryKey: ["admin", "job", id],
    queryFn: () => adminApi.job(id),
    refetchInterval: (q) => (q.state.data && active(q.state.data) ? 1500 : false),
  });
  const cancel = useMutation({
    mutationFn: () => adminApi.cancelJob(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["admin"] }),
  });
  const pre = useRef<HTMLPreElement>(null);
  useEffect(() => {
    if (pre.current) pre.current.scrollTop = pre.current.scrollHeight;
  }, [job.data?.log]);
  if (!job.data) return <Skeleton rows={3} />;
  const j = job.data;
  return (
    <div className="min-w-0 space-y-2">
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <span className="font-medium">
          #{j.id} {t("jobs.kinds." + j.kind)}
        </span>
        <JobStatusBadge status={j.status} />
        <span className="text-muted-foreground">
          {t("jobs.by", { user: j.user || "—" })} · {when(j.started_at ?? j.created_at)}
          {j.finished_at && " → " + when(j.finished_at)}
        </span>
        {active(j) && (
          <Button
            size="sm"
            variant="outline"
            className="ml-auto"
            onClick={() => cancel.mutate()}
            disabled={cancel.isPending}
          >
            <Ban />
            {t("jobs.cancel")}
          </Button>
        )}
      </div>
      {Object.keys(j.params ?? {}).length > 0 && (
        <code className="block truncate text-2xs text-muted-foreground">{JSON.stringify(j.params)}</code>
      )}
      <pre
        ref={pre}
        tabIndex={0}
        aria-label={t("jobs.log")}
        className="max-h-96 overflow-auto rounded-md border bg-surface-2 p-3 font-mono text-2xs leading-relaxed whitespace-pre-wrap"
      >
        {j.log || t("jobs.noLog")}
      </pre>
      <ErrorLine error={cancel.error} />
    </div>
  );
}

function Coverage({ c }: { c: CorpusAdmin }) {
  const { t } = useTranslation();
  const [all, setAll] = useState(false);
  const cov = c.coverage;
  if (!cov.available) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t("data.coverage")}</CardTitle>
        </CardHeader>
        <CardContent className="text-sm text-muted-foreground">{t("data.noCoverage")}</CardContent>
      </Card>
    );
  }
  const rows = (cov.rows ?? []).filter((r) => all || r.status !== "ok");
  const variant = { ok: "in_force", expired: "expired", missing: "partly_expired", check: "not_yet_in_force" } as const;
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("data.coverage")}</CardTitle>
        <span className="text-2xs text-muted-foreground">{t("data.coverageAt", { at: when(cov.written_at) })}</span>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap items-center gap-2 text-xs">
          {(["ok", "expired", "missing", "check"] as const).map((s) => (
            <Badge key={s} variant={variant[s]}>
              {t("data.cover." + s)}: {cov.counts?.[s] ?? 0}
            </Badge>
          ))}
          <Checkbox
            className="ml-auto text-xs"
            label={t("data.showAll")}
            checked={all}
            onChange={(e) => setAll(e.target.checked)}
          />
        </div>
        {rows.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("data.allCovered")}</p>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>{t("data.number")}</Th>
                <Th>{t("data.group")}</Th>
                <Th>{t("data.status")}</Th>
                <Th className="text-right">{t("data.chunks")}</Th>
                <Th>{t("data.reason")}</Th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <Tr key={r.number + r.doc_id}>
                  <Td className="font-medium whitespace-nowrap">{r.number}</Td>
                  <Td>{r.group}</Td>
                  <Td>
                    <Badge variant={variant[r.status as keyof typeof variant] ?? "default"}>
                      {t("data.cover." + r.status)}
                    </Badge>
                  </Td>
                  <Td className="text-right tabular-nums">{r.chunks}</Td>
                  <Td className="min-w-64">{r.reason}</Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}

function Versions({ c }: { c: CorpusAdmin }) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("data.versions")}</CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <thead>
            <tr>
              <Th>{t("data.version")}</Th>
              <Th>{t("data.ingested")}</Th>
              <Th>{t("data.asOf")}</Th>
              <Th className="text-right">{t("data.documents")}</Th>
              <Th className="text-right">{t("data.chunks")}</Th>
              <Th className="text-right">{t("data.added")}</Th>
              <Th className="text-right">{t("data.changed")}</Th>
              <Th className="text-right">{t("data.removed")}</Th>
              <Th>{t("data.revision")}</Th>
            </tr>
          </thead>
          <tbody>
            {c.versions.map((v) => (
              <Tr key={v.id}>
                <Td className="font-medium">
                  v{v.id}
                  {v.id === c.current.version && (
                    <Badge variant="accent" className="ml-1.5">
                      {t("data.serving")}
                    </Badge>
                  )}
                </Td>
                <Td className="whitespace-nowrap">{when(v.created_at)}</Td>
                <Td>{v.as_of}</Td>
                <Td className="text-right tabular-nums">{v.documents.toLocaleString()}</Td>
                <Td className="text-right tabular-nums">{v.chunks.toLocaleString()}</Td>
                <Td className="text-right tabular-nums">{v.added.toLocaleString()}</Td>
                <Td className="text-right tabular-nums">{v.changed.toLocaleString()}</Td>
                <Td className="text-right tabular-nums">{v.removed.toLocaleString()}</Td>
                <Td className="font-mono text-2xs">{v.source_revision.slice(0, 12)}</Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      </CardContent>
    </Card>
  );
}
