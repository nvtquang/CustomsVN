import { Link } from "@tanstack/react-router";
import { ChevronRight, Database, ExternalLink, GitBranch, History, ListTree, ScrollText } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ForceBadge } from "@/components/force-badge";
import { Badge } from "@/components/ui/badge";
import type { Card, GraphCard, HSCard, HSLine, RegDoc, StatusCard, Step, TimelineCard } from "@/lib/api/chat";
import { cn } from "@/lib/utils";

/** A card is data from the database, shown as is; the frame says so. */
function Frame({
  icon,
  title,
  children,
}: {
  icon: React.ReactNode;
  title: React.ReactNode;
  children: React.ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <div className="overflow-hidden rounded-md border bg-surface">
      <div className="flex items-center gap-2 border-b bg-surface-2 px-3 py-2 text-xs">
        <span className="text-muted-foreground [&_svg]:size-3.5">{icon}</span>
        <span className="font-medium">{title}</span>
        <span
          className="ml-auto inline-flex items-center gap-1 text-2xs text-muted-foreground"
          title={t("cards.fromData")}
        >
          <Database className="size-3" /> {t("cards.fromData")}
        </span>
      </div>
      <div className="p-3 text-sm">{children}</div>
    </div>
  );
}

function DocRef({ d }: { d: RegDoc }) {
  return d.in_corpus ? (
    <Link to="/doc/$id" params={{ id: d.id }} search={{}} className="font-medium text-primary hover:underline">
      {d.number}
    </Link>
  ) : (
    <span className="font-medium">{d.number}</span>
  );
}

function StatusView({ s }: { s: StatusCard }) {
  const { t } = useTranslation();
  return (
    <Frame icon={<ScrollText />} title={t("cards.status")}>
      <div className="space-y-2">
        <div className="flex flex-wrap items-center gap-2">
          <DocRef d={s} />
          <ForceBadge force={s.force} />
          <span className="text-2xs text-muted-foreground">{t("cards.asOf", { date: s.as_of })}</span>
        </div>
        <p className="text-xs">{s.title}</p>
        <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-xs">
          {s.effective_from && (
            <>
              <dt className="text-muted-foreground">{t("chunk.effective")}</dt>
              <dd>{s.effective_from}</dd>
            </>
          )}
          {s.expired_on && (
            <>
              <dt className="text-muted-foreground">{t("chunk.expired")}</dt>
              <dd>{s.expired_on}</dd>
            </>
          )}
          <dt className="text-muted-foreground">{t("chunk.status")}</dt>
          <dd>{s.force_status || "—"}</dd>
          {s.consolidated_in && (
            <>
              <dt className="text-muted-foreground">{t("chunk.consolidatedIn")}</dt>
              <dd>
                <DocRef d={s.consolidated_in} />
              </dd>
            </>
          )}
        </dl>
        {s.note && <p className="text-xs text-force-partly">{s.note}</p>}
        {s.replaced_by && s.replaced_by.length > 0 && (
          <div className="text-xs">
            <span className="text-muted-foreground">{t("cards.replacedBy")}: </span>
            {s.replaced_by.map((r, i) => (
              <span key={r.id}>
                {i > 0 && ", "}
                <DocRef d={r} /> <span className="text-muted-foreground">({t("rel." + r.relation, r.relation)})</span>
              </span>
            ))}
          </div>
        )}
        {s.source_url && (
          <a
            href={s.source_url}
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 text-2xs text-primary"
          >
            <ExternalLink className="size-3" /> vbpl.vn
          </a>
        )}
      </div>
    </Frame>
  );
}

const eventColor: Record<string, string> = {
  issued: "bg-muted-foreground",
  effective: "bg-force-in",
  amends: "bg-primary",
  corrects: "bg-primary",
  consolidates: "bg-accent-foreground",
  replaces: "bg-force-expired",
  repeals: "bg-force-expired",
  expires: "bg-force-expired",
  ended: "bg-force-expired",
  suspends: "bg-force-partly",
};

function TimelineView({ tl }: { tl: TimelineCard }) {
  const { t } = useTranslation();
  return (
    <Frame
      icon={<History />}
      title={
        <span className="inline-flex items-center gap-2">
          {t("cards.timeline")} <DocRef d={tl.doc} /> <ForceBadge force={tl.force} />
        </span>
      }
    >
      <ol className="relative space-y-2.5 border-l pl-4">
        {tl.events.map((e, i) => (
          <li key={i} className="relative">
            <span
              className={cn(
                "absolute top-1.5 -left-[1.36rem] size-2.5 rounded-full ring-2 ring-surface",
                eventColor[e.kind] ?? "bg-muted-foreground",
              )}
            />
            <div className="flex flex-wrap items-baseline gap-x-2 text-xs">
              <span className="font-mono text-muted-foreground">{e.date || "—"}</span>
              <Badge variant="outline">{t("event." + e.kind, e.kind)}</Badge>
              {e.kind !== "issued" && e.kind !== "effective" && e.kind !== "ended" && <DocRef d={e.doc} />}
            </div>
            {e.kind !== "issued" && e.kind !== "effective" && e.kind !== "ended" && (
              <p className="line-clamp-2 text-2xs text-muted-foreground">{e.doc.title}</p>
            )}
          </li>
        ))}
      </ol>
    </Frame>
  );
}

export function HSLineView({ l, open }: { l: HSLine; open?: boolean }) {
  const [more, setMore] = useState(!!open);
  return (
    <div className="space-y-1">
      {l.path && l.path.length > 0 && (
        <div className="flex flex-wrap items-center gap-1 text-2xs text-muted-foreground">
          {l.path.map((p, i) => (
            <span key={i} className="inline-flex items-center gap-1">
              {i > 0 && <ChevronRight className="size-3" />}
              {p.code && <span className="font-mono">{p.code}</span>}
              <span className="max-w-56 truncate">{p.description_vi}</span>
            </span>
          ))}
        </div>
      )}
      <div className="flex flex-wrap items-baseline gap-2">
        <span className="font-mono font-semibold">{l.code || "—"}</span>
        <span>{l.description_vi}</span>
        {l.unit && <Badge variant="outline">{l.unit}</Badge>}
      </div>
      {l.description_en && <p className="text-xs text-muted-foreground italic">{l.description_en}</p>}
      {l.children && l.children.length > 0 && (
        <div>
          <button type="button" className="cursor-pointer text-2xs text-primary" onClick={() => setMore(!more)}>
            {more ? "▾" : "▸"} {l.children.length}
          </button>
          {more && (
            <ul className="mt-1 space-y-0.5 border-l pl-3 text-xs">
              {l.children.map((c, i) => (
                <li key={i} style={{ paddingLeft: (c.level - l.level - 1) * 12 }}>
                  <span className="font-mono">{c.code || "·"}</span> {c.description_vi}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}

function HSView({ hs }: { hs: HSCard }) {
  const { t } = useTranslation();
  return (
    <Frame
      icon={<ListTree />}
      title={
        <span>
          {t("cards.hs")} ·{" "}
          <Link to="/doc/$id" params={{ id: hs.source_doc_id }} search={{}} className="text-primary hover:underline">
            {hs.source}
          </Link>
        </span>
      }
    >
      {hs.lines.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t("hs.none")}</p>
      ) : (
        <ul className="space-y-3">
          {hs.lines.slice(0, 6).map((l) => (
            <li key={l.position}>
              <HSLineView l={l} />
            </li>
          ))}
        </ul>
      )}
      <Link to="/hs" search={{ q: hs.query }} className="mt-2 inline-block text-2xs text-primary hover:underline">
        {t("hs.openLookup")}
      </Link>
    </Frame>
  );
}

function GraphView({ g }: { g: GraphCard }) {
  const { t } = useTranslation();
  return (
    <Frame
      icon={<GitBranch />}
      title={
        <span>
          {t("graph." + g.query, g.query)} <DocRef d={g.doc} />
        </span>
      }
    >
      <ul className="space-y-1 text-xs">
        {g.rows.map((r) => (
          <li key={r.id} className="flex flex-wrap gap-x-2">
            <DocRef d={r} />
            <span className="line-clamp-1 text-muted-foreground">{r.title}</span>
          </li>
        ))}
        {g.rows.length === 0 && <li className="text-muted-foreground">—</li>}
      </ul>
    </Frame>
  );
}

export function CardView({ card }: { card: Card }) {
  switch (card.type) {
    case "status":
      return <StatusView s={card.data} />;
    case "amendments":
      return <TimelineView tl={card.data} />;
    case "hs":
      return <HSView hs={card.data} />;
    case "graph":
      return <GraphView g={card.data} />;
  }
}

/** The agent's tool calls, folded by default. */
export function StepsView({ steps, open }: { steps: Step[]; open?: boolean }) {
  const { t } = useTranslation();
  if (steps.length === 0) return null;
  return (
    <details className="group rounded-md border bg-surface-2 text-xs" open={open}>
      <summary className="cursor-pointer list-none px-3 py-1.5 text-muted-foreground select-none">
        <span className="group-open:hidden">▸</span>
        <span className="hidden group-open:inline">▾</span> {t("steps.title", { n: steps.length })}
      </summary>
      <ol className="space-y-1 px-3 pb-2">
        {steps.map((s) => (
          <li key={s.n} className="flex flex-wrap items-baseline gap-x-2">
            <span className="font-mono text-muted-foreground">{s.n}.</span>
            <span className="font-mono">{s.tool}</span>
            <span className="max-w-80 truncate text-muted-foreground">{argsText(s.args)}</span>
            {s.error ? <span className="text-destructive">{s.error}</span> : <span>→ {s.summary}</span>}
            <Badge variant="outline">{t("steps." + s.by)}</Badge>
            <span className="text-muted-foreground">{Math.round(s.ms)} ms</span>
          </li>
        ))}
      </ol>
    </details>
  );
}

function argsText(a: Record<string, unknown>) {
  return Object.values(a ?? {})
    .map((v) => String(v))
    .join(" · ");
}
