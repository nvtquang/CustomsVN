import { useMutation } from "@tanstack/react-query";
import { Copy, Search } from "lucide-react";
import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Textarea } from "@/components/ui/form";
import { api } from "@/lib/api/client";
import type { Filters, SearchConfig, SearchResponse } from "@/lib/api/types";
import { ConfigEditor, FiltersEditor } from "./editors";
import { ResponseSummary, ResultsList } from "./results";

/** The same question under two configurations, side by side. */
export function ABCompare({
  initialFilters,
  initialConfig,
  models,
  rerank,
}: {
  initialFilters: Filters;
  initialConfig: SearchConfig;
  models: string[];
  rerank: boolean;
}) {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const [filters, setFilters] = useState(initialFilters);
  const [a, setA] = useState(initialConfig);
  const [b, setB] = useState<SearchConfig>({ ...initialConfig, keep_originals: !initialConfig.keep_originals });
  const run = useMutation({
    mutationFn: async () => {
      const [ra, rb] = await Promise.all([
        api.search({ query, filters, config: a }),
        api.search({ query, filters, config: b }),
      ]);
      return { ra, rb };
    },
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (query.trim()) run.mutate();
  };
  const ids = (r?: SearchResponse) => new Set(r?.hits.map((h) => h.chunk_id) ?? []);
  const inA = ids(run.data?.ra);
  const inB = ids(run.data?.rb);
  const overlap = [...inA].filter((x) => inB.has(x)).length;

  return (
    <div className="space-y-4">
      <form onSubmit={submit}>
        <Card>
          <CardContent className="grid gap-3 lg:grid-cols-[1fr_auto]">
            <Textarea
              aria-label={t("lab.query")}
              placeholder={t("lab.queryPlaceholder")}
              className="min-h-12"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
            <Button type="submit" disabled={run.isPending || !query.trim()} className="lg:h-auto">
              <Search /> {run.isPending ? t("lab.searching") : t("lab.search")}
            </Button>
          </CardContent>
        </Card>
      </form>
      <Card>
        <CardHeader>
          <CardTitle>{t("lab.filters")}</CardTitle>
        </CardHeader>
        <CardContent>
          <FiltersEditor value={filters} onChange={setFilters} />
        </CardContent>
      </Card>
      {run.error && <p className="text-destructive">{t("common.error", { msg: run.error.message })}</p>}
      {run.data && (
        <p className="text-xs text-muted-foreground">
          {t("lab.overlap", { n: overlap, k: Math.max(inA.size, inB.size) })}
        </p>
      )}
      <div className="grid gap-4 lg:grid-cols-2">
        {(["a", "b"] as const).map((side) => {
          const cfg = side === "a" ? a : b;
          const set = side === "a" ? setA : setB;
          const res = side === "a" ? run.data?.ra : run.data?.rb;
          const other = side === "a" ? inB : inA;
          return (
            <Card key={side} className="min-w-0">
              <CardHeader>
                <CardTitle>{t("lab." + side)}</CardTitle>
                {side === "b" && (
                  <Button variant="ghost" size="sm" onClick={() => setB({ ...a })}>
                    <Copy /> {t("lab.copyAtoB")}
                  </Button>
                )}
              </CardHeader>
              <CardContent className="space-y-3 p-0">
                <details className="px-4 pt-3">
                  <summary className="cursor-pointer text-xs font-medium">{t("lab.config")}</summary>
                  <div className="pt-3">
                    <ConfigEditor value={cfg} onChange={set} models={models} rerankAvailable={rerank} />
                  </div>
                </details>
                {res && (
                  <>
                    <div className="px-4">
                      <ResponseSummary res={res} />
                    </div>
                    <ResultsList
                      hits={res.hits}
                      mark={(h) => (other.has(h.chunk_id) ? undefined : t("lab.onlyIn", { side: side.toUpperCase() }))}
                    />
                  </>
                )}
              </CardContent>
            </Card>
          );
        })}
      </div>
    </div>
  );
}
