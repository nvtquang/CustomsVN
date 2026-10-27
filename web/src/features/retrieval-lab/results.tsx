import { CornerDownRight, Link2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { ForceBadge, SourceBadge } from "@/components/force-badge";
import { Badge } from "@/components/ui/badge";
import type { Hit, SearchResponse } from "@/lib/api/types";
import { cn, fmt } from "@/lib/utils";
import { useUI } from "@/stores/ui";

/** What the search did before it ranked anything: scope, references, notes, timing. */
export function ResponseSummary({ res }: { res: SearchResponse }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2 text-xs">
      <div className="flex flex-wrap gap-x-4 gap-y-1 text-muted-foreground">
        <span>{t("lab.inScope", { docs: res.in_scope_docs, chunks: res.in_scope_chunks.toLocaleString() })}</span>
        <span>
          {t("lab.timing")}:{" "}
          {Object.entries(res.timing_ms)
            .map(([k, v]) => k + " " + v.toFixed(1))
            .join(" · ")}{" "}
          ms
        </span>
        <span className={cn(res.expired_in_top_k > 0 && "font-medium text-force-expired")}>
          {t("lab.expiredInTop", { n: res.expired_in_top_k })}
        </span>
      </div>
      {res.resolved && res.resolved.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="text-muted-foreground">{t("lab.refs")}:</span>
          {res.resolved.map((r, i) => (
            <Badge
              key={i}
              variant={r.chunk_id && !r.note ? "accent" : "outline"}
              title={r.note && t("note." + r.note, r.note)}
            >
              <Link2 className="size-3" />
              {r.ref.span}
              {r.note ? " — " + t("note." + r.note, r.note) : ""}
            </Badge>
          ))}
        </div>
      )}
      {res.notes && res.notes.length > 0 && (
        <ul className="list-disc pl-4 text-force-partly">
          {res.notes.map((n) => (
            <li key={n}>{t("note." + n, n)}</li>
          ))}
        </ul>
      )}
    </div>
  );
}

/** The ranked hits. `mark` highlights hits by chunk id (used by A/B). */
export function ResultsList({ hits, mark }: { hits: Hit[]; mark?: (h: Hit) => string | undefined }) {
  const { t } = useTranslation();
  const openChunk = useUI((s) => s.openChunk);
  const selected = useUI((s) => s.panelChunk);
  if (hits.length === 0) return <p className="py-6 text-center text-muted-foreground">{t("lab.noResults")}</p>;
  return (
    <ol className="divide-y">
      {hits.map((h) => {
        const tag = mark?.(h);
        return (
          <li key={h.chunk_id}>
            <button
              type="button"
              onClick={() => openChunk(h.chunk_id)}
              className={cn(
                "flex w-full cursor-pointer gap-3 px-3 py-2.5 text-left hover:bg-surface-2",
                selected === h.chunk_id && "bg-accent/60",
              )}
            >
              <span className="w-6 shrink-0 pt-0.5 text-right font-mono text-xs text-muted-foreground">{h.rank}</span>
              <div className="min-w-0 flex-1 space-y-1">
                <div className="flex flex-wrap items-center gap-1.5">
                  <span className="text-sm font-semibold">{h.doc.official_number}</span>
                  <ForceBadge force={h.force} />
                  <SourceBadge kind={h.chunk.source_kind} />
                  {h.chunk.chunk_kind === "table" && <Badge variant="outline">{t("chunk.table")}</Badge>}
                  {h.direct && <Badge variant="accent">{t("lab.direct")}</Badge>}
                  {tag && <Badge variant="outline">{tag}</Badge>}
                </div>
                <div className="truncate text-xs text-muted-foreground">{h.chunk.path.join(" › ") || h.doc.title}</div>
                <p className="line-clamp-2 text-xs">{h.chunk.text}</p>
                {h.replaced && (
                  <div className="flex items-center gap-1 text-2xs text-muted-foreground">
                    <CornerDownRight className="size-3" />
                    {t("lab.replaced")}: <span className="font-mono">{h.replaced}</span>
                  </div>
                )}
                {h.duplicates && h.duplicates.length > 0 && (
                  <div className="text-2xs text-muted-foreground">
                    {t("lab.duplicates", { n: h.duplicates.length })}
                  </div>
                )}
              </div>
              <ScoreColumn h={h} />
            </button>
          </li>
        );
      })}
    </ol>
  );
}

function ScoreColumn({ h }: { h: Hit }) {
  const { t } = useTranslation();
  const row = (label: string, v?: { rank: number; score: number }) =>
    v && (
      <div className="flex justify-between gap-2">
        <span className="text-muted-foreground">{label}</span>
        <span>
          #{v.rank} · {fmt(v.score, 2)}
        </span>
      </div>
    );
  return (
    <div className="hidden w-36 shrink-0 space-y-0.5 font-mono text-2xs sm:block">
      <div className="flex justify-between gap-2 font-semibold">
        <span>{t("lab.score")}</span>
        <span>{fmt(h.score, 4)}</span>
      </div>
      {row(t("lab.bm25"), h.bm25)}
      {row(t("lab.vector"), h.vector)}
      {row(t("lab.named"), h.named)}
      <div className="flex justify-between gap-2">
        <span className="text-muted-foreground">{t("lab.rrf")}</span>
        <span>{fmt(h.rrf, 4)}</span>
      </div>
      {h.rerank != null && (
        <div className="flex justify-between gap-2">
          <span className="text-muted-foreground">{t("lab.rerankScore")}</span>
          <span>{fmt(h.rerank, 3)}</span>
        </div>
      )}
    </div>
  );
}
