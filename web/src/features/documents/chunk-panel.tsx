import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ExternalLink, FileText, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { ForceBadge, SourceBadge } from "@/components/force-badge";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api/client";
import { highlight } from "@/lib/highlight";
import { useUI } from "@/stores/ui";

/** The right panel: the full text of a chunk with its document and force. */
export function ChunkPanel() {
  const { t } = useTranslation();
  const id = useUI((s) => s.panelChunk);
  const quote = useUI((s) => s.panelQuote);
  const close = useUI((s) => s.openChunk);
  const q = useQuery({ queryKey: ["chunk", id], queryFn: () => api.chunk(id!), enabled: !!id });
  if (!id) return null;
  const c = q.data?.chunk;
  const d = q.data?.doc;
  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b px-4 py-3">
        <h2 className="text-sm font-semibold">{t("chunk.title")}</h2>
        <Button variant="ghost" size="icon" aria-label={t("nav.close")} onClick={() => close(null)}>
          <X />
        </Button>
      </div>
      <div className="flex-1 space-y-4 overflow-y-auto p-4">
        {q.isLoading && <p className="text-muted-foreground">{t("common.loading")}</p>}
        {q.error && <p className="text-destructive">{t("common.error", { msg: q.error.message })}</p>}
        {c && d && (
          <>
            <div className="space-y-1.5">
              <div className="flex flex-wrap items-center gap-1.5">
                <span className="font-semibold">{d.official_number}</span>
                <ForceBadge force={q.data!.force_today} />
                <SourceBadge kind={c.source_kind} />
                <Badge variant="outline">{t("tier." + c.tier)}</Badge>
                {c.chunk_kind === "table" && <Badge variant="outline">{t("chunk.table")}</Badge>}
                {c.needs_review && <Badge variant="partly_expired">{t("chunk.review")}</Badge>}
              </div>
              <p className="text-xs text-muted-foreground">{d.title}</p>
            </div>
            <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
              <dt className="text-muted-foreground">{t("chunk.path")}</dt>
              <dd>{c.path.join(" › ") || "—"}</dd>
              <dt className="text-muted-foreground">{t("chunk.effective")}</dt>
              <dd>{d.effective_from || "—"}</dd>
              {d.expired_on && (
                <>
                  <dt className="text-muted-foreground">{t("chunk.expired")}</dt>
                  <dd>{d.expired_on}</dd>
                </>
              )}
              <dt className="text-muted-foreground">{t("chunk.status")}</dt>
              <dd>{d.force_status || "—"}</dd>
              {d.consolidated_in && (
                <>
                  <dt className="text-muted-foreground">{t("chunk.consolidatedIn")}</dt>
                  <dd className="break-all">{d.consolidated_in}</dd>
                </>
              )}
              {d.consolidates && (
                <>
                  <dt className="text-muted-foreground">{t("chunk.consolidates")}</dt>
                  <dd className="break-all">{d.consolidates}</dd>
                </>
              )}
            </dl>
            {c.text_source && <p className="text-xs text-force-partly">{t("chunk.legacy")}</p>}
            <div className="legal-text rounded-md border bg-surface-2 p-3 text-sm">{highlight(c.text, quote)}</div>
            {quote && <p className="text-2xs text-force-in">{t("chat.verified")}</p>}
            <Link
              to="/doc/$id"
              params={{ id: d.id }}
              search={{ chunk: c.id, quote: quote ?? undefined }}
              className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
            >
              <FileText className="size-3.5" /> {t("doc.open")}
            </Link>
            {c.amendment_notes && c.amendment_notes.length > 0 && (
              <div className="space-y-1">
                <h3 className="text-xs font-semibold">{t("chunk.notes")}</h3>
                <ul className="list-disc space-y-1 pl-4 text-xs text-muted-foreground">
                  {c.amendment_notes.map((n, i) => (
                    <li key={i}>{n}</li>
                  ))}
                </ul>
              </div>
            )}
            <p className="break-all font-mono text-2xs text-muted-foreground">{c.id}</p>
            {d.source_url && (
              <a
                href={d.source_url}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
              >
                <ExternalLink className="size-3.5" /> {t("chunk.openSource")}
              </a>
            )}
          </>
        )}
      </div>
    </div>
  );
}
