import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { ExternalLink } from "lucide-react";
import { useEffect, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { ForceBadge, SourceBadge } from "@/components/force-badge";
import { chatApi, type ProvisionNode } from "@/lib/api/chat";
import { highlight, highlightPart } from "@/lib/highlight";
import { cn } from "@/lib/utils";

export const Route = createFileRoute("/doc/$id")({
  validateSearch: (s: Record<string, unknown>): { chunk?: string; quote?: string } => ({
    chunk: typeof s.chunk === "string" ? s.chunk : undefined,
    quote: typeof s.quote === "string" ? s.quote : undefined,
  }),
  component: DocumentViewer,
});

const kindLabel: Record<string, string> = {
  chapter: "Chương",
  section: "Mục",
  article: "Điều",
  clause: "",
  point: "",
  annex: "Phụ lục",
};

function label(p: ProvisionNode) {
  if (p.kind === "clause") return p.number + ".";
  if (p.kind === "point") return p.number + ")";
  return (kindLabel[p.kind] ?? p.kind) + " " + p.number + (p.heading ? ". " + p.heading : "");
}

/** The whole document: its tree on the left, its text on the right, scrolled to a citation. */
function DocumentViewer() {
  const { t } = useTranslation();
  const { id } = Route.useParams();
  const { chunk, quote } = Route.useSearch();
  const q = useQuery({ queryKey: ["document", id], queryFn: () => chatApi.document(id) });

  // The provision a chunk stands for: its own identifier without the part
  // suffix, or the nearest ancestor present in the tree.
  const target = useMemo(() => {
    if (!chunk || !q.data) return undefined;
    const ids = new Set(q.data.provisions.map((p) => p.id));
    let c = chunk.split("#")[0];
    if (!c.startsWith(id + ":")) return undefined;
    while (c && !ids.has(c)) {
      const i = c.lastIndexOf(":");
      if (i < 0) return undefined;
      c = c.slice(0, i);
    }
    return c;
  }, [chunk, id, q.data]);

  useEffect(() => {
    if (!q.data) return;
    const el = document.querySelector("[data-quote]") ?? (target ? document.getElementById("p-" + target) : null);
    el?.scrollIntoView({ block: "center" });
  }, [q.data, target]);

  if (q.error) return <p className="p-6 text-destructive">{t("doc.notFound")}</p>;
  if (!q.data) return <p className="p-6 text-muted-foreground">{t("common.loading")}</p>;
  const { doc, provisions, related } = q.data;
  const toc = provisions.filter((p) => ["chapter", "section", "article", "annex"].includes(p.kind));
  const inTarget = (p: ProvisionNode) => !!target && (p.id === target || p.id.startsWith(target + ":"));

  return (
    <div className="mx-auto grid max-w-[90rem] gap-6 p-4 lg:grid-cols-[18rem_1fr] lg:p-6">
      <aside className="hidden lg:block">
        <div className="sticky top-0 max-h-[calc(100dvh-8rem)] space-y-1 overflow-y-auto text-xs">
          <div className="pb-1 text-2xs font-medium tracking-wide text-muted-foreground uppercase">{t("doc.toc")}</div>
          {toc.map((p) => (
            <a
              key={p.id}
              href={"#p-" + p.id}
              className={cn(
                "block truncate rounded px-2 py-1 hover:bg-muted",
                p.kind === "article" && "pl-4",
                inTarget(p) && "bg-accent font-medium text-accent-foreground",
              )}
              title={label(p)}
            >
              {label(p)}
            </a>
          ))}
        </div>
      </aside>
      <article className="min-w-0 space-y-4">
        <header className="space-y-2 border-b pb-4">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-lg font-semibold">{doc.official_number}</h1>
            <ForceBadge force={q.data.force_today} />
            <SourceBadge kind={doc.source_kind} />
          </div>
          <p className="text-sm">{doc.title}</p>
          <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
            {doc.issuing_body && <span>{doc.issuing_body}</span>}
            {doc.signed_on && (
              <span>
                {t("doc.signed")} {doc.signed_on}
              </span>
            )}
            {doc.effective_from && (
              <span>
                {t("chunk.effective")} {doc.effective_from}
              </span>
            )}
            {doc.expired_on && (
              <span>
                {t("chunk.expired")} {doc.expired_on}
              </span>
            )}
            {doc.source_url && (
              <a
                href={doc.source_url}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-1 text-primary"
              >
                <ExternalLink className="size-3" /> vbpl.vn
              </a>
            )}
          </div>
          {related.consolidated_in && (
            <p className="text-xs">
              {t("chunk.consolidatedIn")}:{" "}
              <Link
                to="/doc/$id"
                params={{ id: related.consolidated_in.id }}
                search={{}}
                className="text-primary hover:underline"
              >
                {related.consolidated_in.official_number}
              </Link>
            </p>
          )}
          {related.consolidates && (
            <p className="text-xs">
              {t("chunk.consolidates")}:{" "}
              <Link
                to="/doc/$id"
                params={{ id: related.consolidates.id }}
                search={{}}
                className="text-primary hover:underline"
              >
                {related.consolidates.official_number}
              </Link>
            </p>
          )}
        </header>
        {provisions.length > 0 ? (
          <div className="space-y-1.5 text-sm">
            {provisions.map((p) => (
              <section
                key={p.id}
                id={"p-" + p.id}
                className={cn(
                  "scroll-mt-4 rounded-sm",
                  p.kind === "chapter" || p.kind === "section" ? "pt-4 text-center font-semibold" : "",
                  p.kind === "article" && "pt-3",
                  p.kind === "point" && "pl-6",
                  inTarget(p) && "bg-accent/40",
                )}
              >
                {p.kind === "article" || p.kind === "annex" ? (
                  <h3 className="font-semibold">{label(p)}</h3>
                ) : p.kind === "chapter" || p.kind === "section" ? (
                  <div>{label(p)}</div>
                ) : null}
                {p.text && (
                  <p className="legal-text">
                    {(p.kind === "clause" || p.kind === "point") && <span className="font-medium">{label(p)} </span>}
                    {inTarget(p) ? highlightPart(p.text, quote) : p.text}
                  </p>
                )}
              </section>
            ))}
          </div>
        ) : (
          <div className="space-y-3 text-sm">
            {q.data.chunks.map((c) => (
              <p key={c.id} id={"p-" + c.id} className={cn("legal-text", c.id === chunk && "bg-accent/40")}>
                {c.id === chunk ? highlight(q.data.chunk_text?.[c.id] ?? "", quote) : q.data.chunk_text?.[c.id]}
              </p>
            ))}
          </div>
        )}
      </article>
    </div>
  );
}
