import { useMutation } from "@tanstack/react-query";
import { Search } from "lucide-react";
import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Textarea } from "@/components/ui/form";
import { api } from "@/lib/api/client";
import type { Filters, SearchConfig } from "@/lib/api/types";
import { ConfigEditor, FiltersEditor } from "./editors";
import { ResponseSummary, ResultsList } from "./results";

export function SingleQuery({
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
  const [config, setConfig] = useState(initialConfig);
  const search = useMutation({ mutationFn: api.search });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (query.trim()) search.mutate({ query, filters, config });
  };
  return (
    <div className="grid gap-4 xl:grid-cols-[22rem_1fr]">
      <form onSubmit={submit} className="space-y-4">
        <Card>
          <CardContent className="space-y-3">
            <Textarea
              aria-label={t("lab.query")}
              placeholder={t("lab.queryPlaceholder")}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) submit(e);
              }}
            />
            <Button type="submit" className="w-full" disabled={search.isPending || !query.trim()}>
              <Search /> {search.isPending ? t("lab.searching") : t("lab.search")}
            </Button>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>{t("lab.filters")}</CardTitle>
          </CardHeader>
          <CardContent>
            <FiltersEditor value={filters} onChange={setFilters} />
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>{t("lab.config")}</CardTitle>
          </CardHeader>
          <CardContent>
            <ConfigEditor value={config} onChange={setConfig} models={models} rerankAvailable={rerank} />
          </CardContent>
        </Card>
      </form>
      <Card className="min-w-0">
        <CardHeader>
          <CardTitle>{t("lab.results")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3 p-0">
          {search.error && <p className="p-4 text-destructive">{t("common.error", { msg: search.error.message })}</p>}
          {search.data && (
            <>
              <div className="px-4 pt-3">
                <ResponseSummary res={search.data} />
              </div>
              <ResultsList hits={search.data.hits} />
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
