import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api/client";
import type { Filters, SearchConfig } from "@/lib/api/types";

/** The filters and configuration a search starts from: saved settings over server defaults. */
export function useRetrievalDefaults() {
  const meta = useQuery({ queryKey: ["meta"], queryFn: api.meta, staleTime: 60_000 });
  const settings = useQuery({ queryKey: ["settings", "retrieval"], queryFn: api.settings });
  const ready = !!meta.data && !!settings.data;
  const config: SearchConfig | undefined = ready ? { ...meta.data!.defaults, ...settings.data!.config } : undefined;
  const filters: Filters | undefined = ready
    ? { tiers: [1, 2], as_of: meta.data!.today, ...settings.data!.filters }
    : undefined;
  return { meta: meta.data, filters, config, ready, error: meta.error ?? settings.error };
}
