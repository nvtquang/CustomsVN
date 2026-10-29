import { useMutation } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { AlertTriangle, BadgeCheck, CircleSlash, RotateCcw, ThumbsDown, ThumbsUp } from "lucide-react";
import { useState } from "react";
import ReactMarkdown from "react-markdown";
import { useTranslation } from "react-i18next";
import { ForceBadge } from "@/components/force-badge";
import { useMe } from "@/features/auth/use-me";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/form";
import { chatApi, type Claim, type Refusal } from "@/lib/api/chat";
import { cn } from "@/lib/utils";
import { useUI } from "@/stores/ui";

/** A claim and the [n] chips of its verified citations. */
export function ClaimView({ claim }: { claim: Claim }) {
  const openChunk = useUI((s) => s.openChunk);
  const selected = useUI((s) => s.panelChunk);
  const seen = new Set<number>();
  const chips = claim.citations.filter((c) => !seen.has(c.n) && seen.add(c.n));
  return (
    <div className="space-y-1">
      <div className="prose-claim">
        {/* The server drops claims with links or images; the renderer does not
            draw them either, so a claim can never make the browser fetch a URL. */}
        <ReactMarkdown disallowedElements={["img", "a"]} unwrapDisallowed skipHtml>
          {claim.text}
        </ReactMarkdown>
      </div>
      <div className="flex flex-wrap gap-1">
        {chips.map((c) => (
          <button
            key={c.n}
            type="button"
            onClick={() => openChunk(c.chunk_id, c.quote)}
            title={c.number + " · " + c.path}
            className={cn(
              "inline-flex cursor-pointer items-center gap-1 rounded-sm border px-1.5 py-0.5 text-2xs hover:bg-accent",
              selected === c.chunk_id && "border-primary bg-accent text-accent-foreground",
            )}
          >
            <span className="font-mono font-semibold">[{c.n}]</span>
            <span className="max-w-48 truncate">
              {c.number} · {c.path.split(" › ").pop()}
            </span>
          </button>
        ))}
      </div>
    </div>
  );
}

/** The sources an answer rests on, once each, with their force. */
export function SourcesList({ claims }: { claims: Claim[] }) {
  const { t } = useTranslation();
  const openChunk = useUI((s) => s.openChunk);
  const byN = new Map<number, Claim["citations"][number]>();
  claims.forEach((c) => c.citations.forEach((ct) => byN.has(ct.n) || byN.set(ct.n, ct)));
  if (byN.size === 0) return null;
  return (
    <div className="space-y-1 border-t pt-2">
      <div className="flex items-center gap-1 text-2xs font-medium text-muted-foreground">
        <BadgeCheck className="size-3.5 text-force-in" /> {t("chat.sources")} · {t("chat.verified")}
      </div>
      <ol className="space-y-0.5 text-xs">
        {[...byN.entries()]
          .sort((a, b) => a[0] - b[0])
          .map(([n, c]) => (
            <li key={n} className="flex flex-wrap items-center gap-1.5">
              <button
                type="button"
                className="cursor-pointer font-mono font-semibold text-primary hover:underline"
                onClick={() => openChunk(c.chunk_id, c.quote)}
              >
                [{n}]
              </button>
              <span className="font-medium">{c.number}</span>
              <span className="text-muted-foreground">{c.path}</span>
              <ForceBadge force={c.force} />
              <Link
                to="/doc/$id"
                params={{ id: c.doc_id }}
                search={{ chunk: c.chunk_id, quote: c.quote }}
                className="text-2xs text-primary hover:underline"
              >
                {t("doc.open")}
              </Link>
            </li>
          ))}
      </ol>
    </div>
  );
}

/** The parts of the question no source answers, said as such. */
export function GapsView({ gaps }: { gaps: string[] }) {
  const { t } = useTranslation();
  if (gaps.length === 0) return null;
  return (
    <div className="space-y-1 rounded-md border border-dashed p-3 text-sm">
      <div className="flex items-center gap-2 font-medium text-force-partly">
        <CircleSlash className="size-4" /> {t("chat.gaps")}
      </div>
      <ul className="list-disc space-y-0.5 pl-5 text-muted-foreground">
        {gaps.map((g) => (
          <li key={g}>{g}</li>
        ))}
      </ul>
    </div>
  );
}

export function RefusalView({ refusal }: { refusal: Refusal }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2 rounded-md border border-dashed bg-surface-2 p-3">
      <div className="flex items-center gap-2 text-sm font-medium">
        <CircleSlash className="size-4 text-muted-foreground" /> {t("chat.refused")}
      </div>
      <p className="text-sm">{refusal.reason}</p>
      {refusal.suggestions && refusal.suggestions.length > 0 && (
        <div className="text-xs text-muted-foreground">
          {t("chat.refusalTry")}
          <ul className="mt-1 list-disc pl-4">
            {refusal.suggestions.map((s) => (
              <li key={s}>{s}</li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

export function ErrorView({ kind, message }: { kind: string; message: string }) {
  const { t } = useTranslation();
  const admin = useMe().data?.role === "admin";
  const known = ["no_model", "quota", "auth", "network", "server", "request", "canceled", "offline", "rate_limit"];
  return (
    <div
      className={cn(
        "flex flex-wrap items-center gap-2 rounded-md border p-3 text-sm",
        kind === "canceled" ? "text-muted-foreground" : "border-destructive/40 text-destructive",
      )}
    >
      <AlertTriangle className="size-4" />
      <span>{known.includes(kind) ? t("chat.errors." + kind, { msg: message }) : message}</span>
      {(kind === "no_model" || kind === "auth") && admin && (
        <Link to="/settings/models" className="font-medium underline">
          {t("chat.errors.goSettings")}
        </Link>
      )}
    </div>
  );
}

export function Feedback({ messageId, initial }: { messageId: number; initial?: number }) {
  const { t } = useTranslation();
  const [rating, setRating] = useState(initial ?? 0);
  const [asking, setAsking] = useState(false);
  const [reason, setReason] = useState("");
  const send = useMutation({
    mutationFn: (v: { rating: number; reason: string }) => chatApi.feedback(messageId, v.rating, v.reason),
    onSuccess: (_, v) => {
      setRating(v.rating);
      setAsking(false);
    },
  });
  return (
    <div className="flex flex-wrap items-center gap-1">
      <Button
        variant="ghost"
        size="icon"
        className={cn("size-7", rating === 1 && "text-force-in")}
        aria-label={t("feedback.up")}
        title={t("feedback.up")}
        onClick={() => send.mutate({ rating: 1, reason: "" })}
      >
        <ThumbsUp />
      </Button>
      <Button
        variant="ghost"
        size="icon"
        className={cn("size-7", rating === -1 && "text-force-expired")}
        aria-label={t("feedback.down")}
        title={t("feedback.down")}
        onClick={() => setAsking(true)}
      >
        <ThumbsDown />
      </Button>
      {asking && (
        <form
          className="flex items-center gap-1"
          onSubmit={(e) => {
            e.preventDefault();
            send.mutate({ rating: -1, reason });
          }}
        >
          <Input
            autoFocus
            className="h-7 w-64 text-xs"
            placeholder={t("feedback.reasonPlaceholder")}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
          <Button type="submit" size="sm" variant="outline">
            {t("feedback.send")}
          </Button>
        </form>
      )}
      {send.isSuccess && !asking && <span className="text-2xs text-muted-foreground">{t("feedback.thanks")}</span>}
    </div>
  );
}

export function RegenerateButton({ onClick, disabled }: { onClick: () => void; disabled?: boolean }) {
  const { t } = useTranslation();
  return (
    <Button variant="ghost" size="sm" className="h-7 text-muted-foreground" onClick={onClick} disabled={disabled}>
      <RotateCcw /> {t("chat.regenerate")}
    </Button>
  );
}
