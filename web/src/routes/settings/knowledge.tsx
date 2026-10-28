import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  CheckCircle2,
  ChevronRight,
  CircleAlert,
  Gauge,
  Layers,
  ListTree,
  Loader2,
  RotateCcw,
  Save,
  Sparkles,
  Split,
  Type,
  Wallet,
} from "lucide-react";
import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ChoiceCard, Switch } from "@/components/ui/choice";
import { Field, Input, Select } from "@/components/ui/form";
import { AdminOnly, ErrorLine, PageTitle, Skeleton } from "@/features/admin/admin-only";
import { adminApi } from "@/lib/api/admin";
import { api } from "@/lib/api/client";
import type { Chunking, Filters, Meta, Mode, RetrievalSettings, SearchConfig } from "@/lib/api/types";

export const Route = createFileRoute("/settings/knowledge")({
  component: () => (
    <AdminOnly>
      <KnowledgePage />
    </AdminOnly>
  ),
});

function KnowledgePage() {
  const { t } = useTranslation();
  const meta = useQuery({ queryKey: ["meta"], queryFn: api.meta, staleTime: 60_000 });
  const settings = useQuery({ queryKey: ["settings", "retrieval"], queryFn: api.settings });
  const error = meta.error ?? settings.error;
  return (
    <div className="mx-auto max-w-3xl space-y-4 p-4 lg:p-6">
      <PageTitle title={t("kb.title")} subtitle={t("kb.subtitle")} />
      <ErrorLine error={error} />
      {meta.data && settings.data ? <Form meta={meta.data} initial={settings.data} /> : !error && <Skeleton rows={8} />}
    </div>
  );
}

const allKinds = ["consolidated", "original", "rescued"];
const DEFAULT_CHUNKING: Required<Chunking> = { mode: "general", max_length: 4000, parent_length: 6000 };

function Form({ meta, initial }: { meta: Meta; initial: RetrievalSettings }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  // The as-of day is not a setting: a saved one would freeze the bot in time.
  // Expired documents are never grounds for an answer, whatever is saved.
  const [filters, setFilters] = useState<Filters>({ ...initial.filters, as_of: undefined, include_expired: false });
  const [config, setConfig] = useState<SearchConfig>({ ...meta.defaults, ...initial.config });
  const [topK, setTopK] = useState(initial.top_k || 8);
  const [chunking, setChunking] = useState<Required<Chunking>>({ ...DEFAULT_CHUNKING, ...initial.chunking });
  const models = meta.embedding_models ?? [];
  const rerankers = meta.rerank_models ?? [];
  const [index, setIndex] = useState<"high_quality" | "economy">(
    initial.index ?? (config.mode === "bm25" || !models.length ? "economy" : "high_quality"),
  );
  const [dirty, setDirty] = useState(false);
  const touch =
    <T,>(set: (v: T) => void) =>
    (v: T) => {
      set(v);
      setDirty(true);
    };
  const setF = touch(setFilters);
  const setC = touch(setConfig);
  const setK = touch(setTopK);
  const setCh = touch(setChunking);
  const setIdx = touch(setIndex);

  const body = (): RetrievalSettings => ({
    filters: { ...filters, as_of: undefined, include_expired: false },
    config: { ...config, mode: index === "economy" ? "bm25" : config.mode },
    top_k: topK,
    chunking,
    index,
  });
  const save = useMutation({
    mutationFn: () => api.saveSettings(body()),
    onSuccess: () => {
      setDirty(false);
      void qc.invalidateQueries({ queryKey: ["settings", "retrieval"] });
    },
  });
  // A new chunking needs a new prep and ingest; they run one after the other.
  const rechunk = useMutation({
    mutationFn: async () => {
      await api.saveSettings(body());
      setDirty(false);
      await adminApi.startJob("prep", {});
      await adminApi.startJob("ingest", {});
    },
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["admin", "jobs"] }),
  });

  const prepared = initial.prepared;
  const stale =
    !!prepared &&
    (prepared.mode !== chunking.mode || (chunking.mode === "general" && prepared.max_length !== chunking.max_length));
  const hasVector = models.length > 0;
  const mode = index === "economy" ? "bm25" : config.mode;
  const thresholdOK = config.rerank ? rerankers.length > 0 : mode === "vector";
  const tiers = filters.tiers && filters.tiers.length ? filters.tiers : [1, 2];
  const kinds = filters.source_kinds && filters.source_kinds.length ? filters.source_kinds : allKinds;
  const toggleTier = (n: number, on: boolean) => {
    const next = on ? [...new Set([...tiers, n])] : tiers.filter((x) => x !== n);
    if (next.length) setF({ ...filters, tiers: next.sort() });
  };
  const toggleKind = (k: string, on: boolean) => {
    const next = on ? [...new Set([...kinds, k])] : kinds.filter((x) => x !== k);
    if (next.length) setF({ ...filters, source_kinds: next.length === allKinds.length ? [] : next });
  };
  // One slider for the hybrid mix: the share given to meaning over keywords.
  // Weights stay above zero, since a zero weight means "use the default".
  const share = config.vector_weight / (config.vector_weight + config.bm25_weight || 1);
  const setShare = (v: number) =>
    setC({ ...config, vector_weight: +(2 * v).toFixed(2), bm25_weight: +(2 * (1 - v)).toFixed(2) });

  const modeCard = (m: Mode, icon: ReactNode) => (
    <ChoiceCard
      type="radio"
      name="mode"
      checked={mode === m}
      onChange={() => setC({ ...config, mode: m })}
      disabled={index === "economy" ? m !== "bm25" : m !== "bm25" && !hasVector}
      icon={icon}
      title={t("kb.mode." + m + ".title")}
      description={t("kb.mode." + m + ".text")}
      badge={m === "hybrid" ? <Badge variant="accent">{t("rs.recommended")}</Badge> : undefined}
    />
  );

  return (
    <>
      <p className="rounded-md border bg-surface-2 p-3 text-xs leading-relaxed text-muted-foreground">
        {t("rs.intro")}{" "}
        <Link
          to="/lab/retrieval"
          search={{ tab: "single" }}
          className="font-medium text-primary underline-offset-2 hover:underline"
        >
          {t("rs.tryLab")}
        </Link>
      </p>

      {/* 1. Chunking */}
      <Card>
        <CardHeader>
          <div>
            <CardTitle>{t("kb.chunkTitle")}</CardTitle>
            <p className="mt-0.5 text-xs text-muted-foreground">{t("kb.chunkText")}</p>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="grid gap-2" role="radiogroup" aria-label={t("kb.chunkTitle")}>
            <ChoiceCard
              type="radio"
              name="chunking"
              checked={chunking.mode === "general"}
              onChange={() => setCh({ ...chunking, mode: "general" })}
              icon={<ListTree />}
              title={t("kb.general.title")}
              description={t("kb.general.text")}
              badge={<Badge variant="accent">{t("rs.recommended")}</Badge>}
            />
            {chunking.mode === "general" && (
              <Length
                label={t("kb.maxLength")}
                help={t("kb.maxLengthText")}
                value={chunking.max_length}
                onChange={(v) => setCh({ ...chunking, max_length: v })}
              />
            )}
            <ChoiceCard
              type="radio"
              name="chunking"
              checked={chunking.mode === "parent_child"}
              onChange={() => setCh({ ...chunking, mode: "parent_child" })}
              icon={<Split />}
              title={t("kb.parentChild.title")}
              description={t("kb.parentChild.text")}
            />
            {chunking.mode === "parent_child" && (
              <Length
                label={t("kb.parentLength")}
                help={t("kb.parentLengthText")}
                value={chunking.parent_length}
                onChange={(v) => setCh({ ...chunking, parent_length: v })}
              />
            )}
          </div>
          <div className="flex flex-wrap gap-x-2 text-xs">
            <span className="text-muted-foreground">{t("kb.separator")}:</span>
            <span>{t("kb.separatorValue")}</span>
          </div>
          {prepared && (
            <p className="text-xs text-muted-foreground">
              {t("kb.prepared", {
                how: t("kb.preparedHow." + prepared.mode, { n: (prepared.max_length ?? 4000).toLocaleString() }),
              })}
            </p>
          )}
          {stale && (
            <div className="space-y-2 rounded-md border border-force-partly/40 p-3 text-xs">
              <p className="flex items-start gap-1.5">
                <CircleAlert className="mt-0.5 size-3.5 shrink-0 text-force-partly" /> {t("kb.rechunk")}
              </p>
              {rechunk.isSuccess ? (
                <p className="flex flex-wrap items-center gap-2 text-force-in">
                  <CheckCircle2 className="size-3.5" /> {t("kb.rechunkQueued")}
                  <Link to="/admin/data" className="font-medium text-primary hover:underline">
                    {t("kb.viewJobs")}
                  </Link>
                </p>
              ) : (
                <Button size="sm" variant="outline" onClick={() => rechunk.mutate()} disabled={rechunk.isPending}>
                  {rechunk.isPending ? <Loader2 className="animate-spin" /> : <RotateCcw />} {t("kb.rechunkRun")}
                </Button>
              )}
              <ErrorLine error={rechunk.error} />
            </div>
          )}
        </CardContent>
      </Card>

      {/* 2. Index method */}
      <Card>
        <CardHeader>
          <CardTitle>{t("kb.indexTitle")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="grid gap-2 sm:grid-cols-2" role="radiogroup" aria-label={t("kb.indexTitle")}>
            <ChoiceCard
              type="radio"
              name="index"
              checked={index === "high_quality"}
              onChange={() => {
                setIdx("high_quality");
                if (config.mode === "bm25" && hasVector) setC({ ...config, mode: "hybrid" });
              }}
              disabled={!hasVector}
              icon={<Gauge />}
              title={t("kb.high.title")}
              description={t("kb.high.text")}
              badge={<Badge variant="accent">{t("rs.recommended")}</Badge>}
              note={
                !hasVector ? (
                  <>
                    {t("kb.noEmbedding")}{" "}
                    <Link to="/settings/models" className="font-medium text-primary hover:underline">
                      {t("kb.addEmbedding")}
                    </Link>
                  </>
                ) : undefined
              }
            />
            <ChoiceCard
              type="radio"
              name="index"
              checked={index === "economy"}
              onChange={() => setIdx("economy")}
              icon={<Wallet />}
              title={t("kb.economy.title")}
              description={t("kb.economy.text")}
            />
          </div>
          {index === "high_quality" && hasVector && (
            <EmbeddingPicker
              models={models}
              value={config.model || meta.default_model || models[0]}
              onChange={(m) => setC({ ...config, model: m })}
            />
          )}
        </CardContent>
      </Card>

      {/* 3. Retrieval */}
      <Card>
        <CardHeader>
          <CardTitle>{t("kb.retrievalTitle")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-2" role="radiogroup" aria-label={t("kb.retrievalTitle")}>
            {modeCard("vector", <Sparkles />)}
            {modeCard("bm25", <Type />)}
            {modeCard("hybrid", <Layers />)}
          </div>
          {index === "economy" && <p className="text-xs text-muted-foreground">{t("kb.economyOnly")}</p>}
          {mode === "hybrid" && (
            <div className="space-y-2 rounded-md border p-3">
              <div className="flex items-center justify-between text-xs">
                <span className="font-medium">{t("rs.mix")}</span>
                <span className="text-muted-foreground tabular-nums">
                  {t("rs.mixValue", { meaning: Math.round(share * 100), words: 100 - Math.round(share * 100) })}
                </span>
              </div>
              <input
                type="range"
                min={0.1}
                max={0.9}
                step={0.05}
                value={share}
                onChange={(e) => setShare(Number(e.target.value))}
                aria-label={t("rs.mix")}
                className="w-full accent-[var(--primary)]"
              />
              <div className="flex justify-between text-2xs text-muted-foreground">
                <span>{t("rs.mixWords")}</span>
                <span>{t("rs.mixMeaning")}</span>
              </div>
            </div>
          )}

          <div className="space-y-3 rounded-md border p-3">
            <Switch
              label={t("kb.rerank")}
              description={t("kb.rerankText")}
              checked={config.rerank && rerankers.length > 0}
              disabled={!rerankers.length}
              onChange={(v) => setC({ ...config, rerank: v })}
            />
            {!rerankers.length && (
              <p className="text-xs text-muted-foreground">
                {t("kb.rerankNone")}{" "}
                <Link to="/settings/models" className="font-medium text-primary hover:underline">
                  {t("kb.addRerank")}
                </Link>
              </p>
            )}
            {config.rerank && rerankers.length > 0 && (
              <Field label={t("kb.rerankModel")}>
                <Select
                  value={config.rerank_model || rerankers[0]}
                  onChange={(e) => setC({ ...config, rerank_model: e.target.value })}
                >
                  {rerankers.map((m) => (
                    <option key={m} value={m}>
                      {m}
                    </option>
                  ))}
                </Select>
              </Field>
            )}
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <Slider
              label={t("kb.topK")}
              help={t("kb.topKText")}
              min={1}
              max={20}
              step={1}
              value={topK}
              onChange={setK}
            />
            <div className="space-y-2">
              <Switch
                label={t("kb.threshold")}
                description={thresholdOK ? t("kb.thresholdText") : t("kb.thresholdOff")}
                checked={thresholdOK && (config.min_score ?? 0) > 0}
                disabled={!thresholdOK}
                onChange={(v) => setC({ ...config, min_score: v ? 0.5 : 0 })}
              />
              {thresholdOK && (config.min_score ?? 0) > 0 && (
                <Slider
                  label={t("kb.threshold")}
                  min={0.05}
                  max={0.95}
                  step={0.05}
                  value={config.min_score ?? 0.5}
                  format={(v) => v.toFixed(2)}
                  onChange={(v) => setC({ ...config, min_score: v })}
                />
              )}
            </div>
          </div>
        </CardContent>
      </Card>

      {/* 4. Scope and handling */}
      <details className="group rounded-lg border bg-surface">
        <summary className="flex cursor-pointer list-none items-center gap-2 px-4 py-3 text-sm font-semibold">
          <ChevronRight className="size-4 transition-transform group-open:rotate-90" />
          {t("kb.scopeTitle")}
        </summary>
        <div className="space-y-5 border-t p-4">
          <fieldset className="space-y-2">
            <legend className="mb-2 text-xs font-medium text-muted-foreground">{t("rs.groups")}</legend>
            <ChoiceCard
              type="checkbox"
              checked={tiers.includes(1)}
              onChange={(v) => toggleTier(1, v)}
              title={t("rs.tier1.title")}
              description={t("rs.tier1.text")}
              badge={<Badge variant="accent">{t("rs.recommended")}</Badge>}
            />
            <ChoiceCard
              type="checkbox"
              checked={tiers.includes(2)}
              onChange={(v) => toggleTier(2, v)}
              title={t("rs.tier2.title")}
              description={t("rs.tier2.text")}
            />
          </fieldset>
          <fieldset className="space-y-2">
            <legend className="mb-2 text-xs font-medium text-muted-foreground">{t("rs.versions")}</legend>
            {allKinds.map((k) => (
              <ChoiceCard
                key={k}
                type="checkbox"
                checked={kinds.includes(k)}
                onChange={(v) => toggleKind(k, v)}
                title={t("rs.kind." + k + ".title")}
                description={t("rs.kind." + k + ".text")}
              />
            ))}
          </fieldset>
          <p className="text-2xs text-muted-foreground">{t("rs.alwaysInForce")}</p>
          <div className="space-y-4">
            <Switch
              label={t("rs.preferConsolidated")}
              description={t("rs.preferConsolidatedText")}
              checked={!config.keep_originals}
              onChange={(v) => setC({ ...config, keep_originals: !v })}
            />
            <Switch
              label={t("rs.directRefs")}
              description={t("rs.directRefsText")}
              checked={!config.ignore_refs}
              onChange={(v) => setC({ ...config, ignore_refs: !v })}
            />
          </div>
        </div>
      </details>

      <details className="group rounded-lg border bg-surface">
        <summary className="flex cursor-pointer list-none items-center gap-2 px-4 py-3 text-sm font-semibold">
          <ChevronRight className="size-4 transition-transform group-open:rotate-90" />
          {t("rs.advanced")}
          <span className="text-xs font-normal text-muted-foreground">{t("rs.advancedHint")}</span>
        </summary>
        <div className="grid gap-4 border-t p-4 sm:grid-cols-2">
          <Num
            label={t("rs.adv.bm25k")}
            help={t("rs.adv.bm25kText")}
            value={config.bm25_k}
            onChange={(v) => setC({ ...config, bm25_k: v })}
          />
          <Num
            label={t("rs.adv.vectork")}
            help={t("rs.adv.vectorkText")}
            value={config.vector_k}
            onChange={(v) => setC({ ...config, vector_k: v })}
          />
          <Num
            label={t("rs.adv.rrfk")}
            help={t("rs.adv.rrfkText")}
            value={config.rrf_k}
            onChange={(v) => setC({ ...config, rrf_k: v })}
          />
          <Num
            label={t("rs.adv.k")}
            help={t("rs.adv.kText")}
            value={config.k}
            onChange={(v) => setC({ ...config, k: v })}
          />
        </div>
      </details>

      <div className="sticky bottom-0 -mx-4 flex flex-wrap items-center gap-2 border-t bg-background/95 px-4 py-3 backdrop-blur lg:-mx-6 lg:px-6">
        <Button onClick={() => save.mutate()} disabled={save.isPending || !dirty}>
          <Save /> {t("settings.save")}
        </Button>
        <Button
          variant="outline"
          onClick={() => {
            setC(meta.defaults);
            setF({ tiers: [1, 2] });
            setK(8);
            setCh(DEFAULT_CHUNKING);
            setIdx(hasVector ? "high_quality" : "economy");
          }}
        >
          <RotateCcw /> {t("settings.reset")}
        </Button>
        {dirty && <span className="text-xs text-muted-foreground">{t("rs.unsaved")}</span>}
        {save.isSuccess && !dirty && <span className="text-xs text-force-in">{t("settings.saved")}</span>}
        <ErrorLine error={save.error} />
      </div>
    </>
  );
}

/** The embedding model, with how much of the corpus has its vectors and a way to compute the rest. */
function EmbeddingPicker({
  models,
  value,
  onChange,
}: {
  models: string[];
  value: string;
  onChange: (m: string) => void;
}) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const corpus = useQuery({ queryKey: ["admin", "corpus"], queryFn: adminApi.corpus, staleTime: 30_000 });
  const embed = useMutation({
    mutationFn: () => adminApi.startJob("embed", { model: value, target: "chunks" }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["admin", "jobs"] }),
  });
  const total = corpus.data?.current.chunks ?? 0;
  const done = corpus.data?.embeddings.find((e) => e.model === value)?.chunks ?? 0;
  return (
    <div className="space-y-2 rounded-md border p-3">
      <Field label={t("kb.embeddingModel")}>
        <Select value={value} onChange={(e) => onChange(e.target.value)}>
          {models.map((m) => (
            <option key={m} value={m}>
              {m}
            </option>
          ))}
        </Select>
      </Field>
      {corpus.data && (
        <div className="space-y-1.5">
          <div className="h-1.5 overflow-hidden rounded-full bg-muted">
            <div
              className="h-full bg-primary"
              style={{ width: `${total ? Math.min(100, (done / total) * 100) : 0}%` }}
            />
          </div>
          <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
            <span className="text-muted-foreground">
              {t("kb.coverage", { done: done.toLocaleString(), total: total.toLocaleString() })}
            </span>
            {embed.isSuccess ? (
              <span className="flex items-center gap-2 text-force-in">
                <CheckCircle2 className="size-3.5" /> {t("kb.embedQueued")}
                <Link to="/admin/data" className="font-medium text-primary hover:underline">
                  {t("kb.viewJobs")}
                </Link>
              </span>
            ) : (
              done < total && (
                <Button size="sm" variant="outline" onClick={() => embed.mutate()} disabled={embed.isPending}>
                  {embed.isPending && <Loader2 className="animate-spin" />} {t("kb.embedRun")}
                </Button>
              )
            )}
          </div>
          <ErrorLine error={embed.error} />
        </div>
      )}
    </div>
  );
}

function Slider({
  label,
  help,
  min,
  max,
  step,
  value,
  onChange,
  format = String,
}: {
  label: string;
  help?: string;
  min: number;
  max: number;
  step: number;
  value: number;
  onChange: (v: number) => void;
  format?: (v: number) => string;
}) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between gap-2 text-sm font-medium">
        {label}
        <span className="rounded-md border px-2 py-0.5 font-mono text-xs tabular-nums">{format(value)}</span>
      </div>
      <input
        type="range"
        min={min}
        max={max}
        step={step}
        value={value}
        aria-label={label}
        onChange={(e) => onChange(Number(e.target.value))}
        className="w-full accent-[var(--primary)]"
      />
      {help && <p className="text-xs text-muted-foreground">{help}</p>}
    </div>
  );
}

function Length({
  label,
  help,
  value,
  onChange,
}: {
  label: string;
  help: string;
  value: number;
  onChange: (v: number) => void;
}) {
  return (
    <div className="ml-8 rounded-md border bg-surface-2 p-3">
      <Field label={label}>
        <Input
          type="number"
          min={1000}
          max={12000}
          step={500}
          value={String(value)}
          onChange={(e) => onChange(Number(e.target.value) || 0)}
          className="w-36"
        />
        <span className="text-2xs font-normal text-muted-foreground">{help}</span>
      </Field>
    </div>
  );
}

function Num({
  label,
  help,
  value,
  onChange,
}: {
  label: string;
  help: string;
  value: number;
  onChange: (v: number) => void;
}) {
  return (
    <Field label={label}>
      <Input
        type="number"
        min={1}
        value={String(value ?? "")}
        onChange={(e) => onChange(Number(e.target.value) || 0)}
      />
      <span className="text-2xs font-normal text-muted-foreground">{help}</span>
    </Field>
  );
}
