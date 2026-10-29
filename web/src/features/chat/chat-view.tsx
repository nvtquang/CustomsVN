import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { ArrowUp, Loader2, Square } from "lucide-react";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input, Textarea } from "@/components/ui/form";
import { api } from "@/lib/api/client";
import {
  chatApi,
  streamChat,
  type Card,
  type ChatEvent,
  type Claim,
  type Message,
  type Refusal,
  type Step,
} from "@/lib/api/chat";
import { CardView, StepsView } from "./cards";
import { ClaimView, ErrorView, Feedback, GapsView, RefusalView, RegenerateButton, SourcesList } from "./message";

interface Pending {
  question: string;
  stage?: string;
  sources?: number;
  rewritten?: string;
  claims: Claim[];
  gaps: string[];
  dropped: number;
  refusal?: Refusal;
  clarify?: string;
  steps: Step[];
  cards: Card[];
  error?: { kind: string; message: string };
  regenerate: boolean;
}

/** The chat screen: a conversation, or a new one when id is undefined. */
export function ChatView({ id }: { id?: number }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const meta = useQuery({ queryKey: ["meta"], queryFn: api.meta, staleTime: 60_000 });
  const conv = useQuery({ queryKey: ["conversation", id], queryFn: () => chatApi.conversation(id!), enabled: !!id });
  const [input, setInput] = useState("");
  const [asOf, setAsOf] = useState("");
  const [pending, setPending] = useState<Pending | null>(null);
  const abort = useRef<AbortController | null>(null);
  const bottom = useRef<HTMLDivElement>(null);
  const streaming = !!pending && !pending.error && !pending.refusal && !pending.clarify && pending.stage !== "done";
  // The day the law is applied as of: the conversation's, or the one picked
  // before its first question.
  const convAsOf = conv.data?.conversation.as_of_date ?? "";
  const effectiveAsOf = id ? convAsOf : asOf;
  const changeAsOf = async (v: string) => {
    if (id) {
      await chatApi.setAsOf(id, v);
      await qc.invalidateQueries({ queryKey: ["conversation", id] });
    } else {
      setAsOf(v);
    }
  };

  useEffect(() => {
    bottom.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [pending, conv.data]);
  useEffect(() => () => abort.current?.abort(), []);

  const run = async (body: { question?: string; regenerate?: boolean }) => {
    const ctrl = new AbortController();
    abort.current = ctrl;
    let convID = id;
    setPending({
      question: body.question ?? "",
      claims: [],
      gaps: [],
      steps: [],
      cards: [],
      dropped: 0,
      regenerate: !!body.regenerate,
    });
    const on = (e: ChatEvent) => {
      if (e.type === "meta") convID = e.conversation_id;
      setPending((p) => {
        if (!p) return p;
        switch (e.type) {
          case "meta":
            return { ...p, question: p.question || e.question };
          case "status":
            return { ...p, stage: e.stage, sources: e.sources ?? p.sources };
          case "rewrite":
            return { ...p, rewritten: e.text };
          case "step":
            return { ...p, steps: [...p.steps, e.step] };
          case "card":
            return { ...p, cards: [...p.cards, e.card] };
          case "clarify":
            return { ...p, clarify: e.text };
          case "claim":
            return { ...p, claims: [...p.claims, e.claim] };
          case "gap":
            return { ...p, gaps: [...p.gaps, e.text] };
          case "dropped":
            return { ...p, dropped: p.dropped + 1 };
          case "refusal":
            return { ...p, refusal: e.refusal };
          case "error":
            return { ...p, error: { kind: e.kind, message: e.message } };
          case "done":
            return { ...p, stage: "done" };
        }
        return p;
      });
    };
    try {
      await streamChat({ conversation_id: id, as_of: id ? undefined : asOf || undefined, ...body }, on, ctrl.signal);
    } catch (err) {
      const stopped = ctrl.signal.aborted;
      setPending((p) => (p ? { ...p, error: { kind: stopped ? "canceled" : "offline", message: String(err) } } : p));
    }
    void qc.invalidateQueries({ queryKey: ["conversations"] });
    if (convID && convID !== id) {
      await navigate({ to: "/c/$id", params: { id: String(convID) }, replace: true });
    } else if (convID) {
      await qc.invalidateQueries({ queryKey: ["conversation", convID] });
      setPending((p) => (p && !p.error ? null : p));
    }
  };

  const submit = (e?: FormEvent) => {
    e?.preventDefault();
    const q = input.trim();
    if (!q || streaming) return;
    setInput("");
    void run({ question: q });
  };

  const messages = conv.data?.messages ?? [];
  const shown = pending?.regenerate ? dropLastAnswer(messages) : messages;
  const lastAnswer = [...messages].reverse().find((m) => m.role === "assistant");
  const empty = !id && !pending;

  return (
    <div className="flex h-full flex-col">
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto max-w-3xl space-y-5 p-4 lg:p-6">
          {empty && <Empty onPick={(q) => void run({ question: q })} />}
          {conv.error && <ErrorView kind="request" message={conv.error.message} />}
          {shown.map((m) =>
            m.role === "user" ? (
              <UserBubble key={m.id} text={m.content} />
            ) : (
              <AssistantStored
                key={m.id}
                m={m}
                last={m.id === lastAnswer?.id && !pending}
                onRegenerate={() => void run({ regenerate: true })}
              />
            ),
          )}
          {pending && (
            <>
              {!pending.regenerate && pending.question && <UserBubble text={pending.question} />}
              {/* Read out claim by claim as they are verified. */}
              <div className="space-y-3" aria-live="polite" aria-busy={streaming}>
                {pending.rewritten && (
                  <p className="text-xs text-muted-foreground italic">
                    {t("chat.rewritten", { q: pending.rewritten })}
                  </p>
                )}
                <StepsView steps={pending.steps} open={streaming} />
                {pending.cards.map((c, i) => (
                  <CardView key={i} card={c} />
                ))}
                {pending.clarify && <ClarifyView text={pending.clarify} />}
                {pending.claims.map((c, i) => (
                  <ClaimView key={i} claim={c} />
                ))}
                <GapsView gaps={pending.gaps} />
                {streaming && (
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <Loader2 className="size-3.5 animate-spin" />
                    {t("chat.status." + (pending.stage ?? "retrieve"), { n: pending.sources ?? 0 })}
                  </div>
                )}
                {pending.dropped > 0 && (
                  <p className="text-2xs text-muted-foreground">{t("chat.dropped", { n: pending.dropped })}</p>
                )}
                {pending.refusal && <RefusalView refusal={pending.refusal} />}
                {pending.claims.length > 0 && !streaming && <SourcesList claims={pending.claims} />}
                {pending.error && (
                  <ErrorView
                    kind={pending.error.kind}
                    message={
                      pending.error.kind === "canceled" && pending.claims.length
                        ? t("chat.stopped")
                        : pending.error.message
                    }
                  />
                )}
              </div>
            </>
          )}
          <div ref={bottom} />
        </div>
      </div>
      <form onSubmit={submit} className="shrink-0 border-t bg-surface">
        <div className="mx-auto max-w-3xl space-y-2 p-3 lg:px-6">
          <div className="flex items-end gap-2">
            <Textarea
              id="question"
              aria-label={t("chat.placeholder")}
              placeholder={t("chat.placeholder")}
              className="max-h-48 min-h-11 resize-none"
              rows={1}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) submit(e);
                if (e.key === "Escape" && streaming) abort.current?.abort();
              }}
            />
            {streaming ? (
              <Button
                type="button"
                variant="outline"
                size="icon"
                aria-label={t("chat.stop")}
                onClick={() => abort.current?.abort()}
              >
                <Square />
              </Button>
            ) : (
              <Button type="submit" size="icon" aria-label={t("chat.send")} disabled={!input.trim()}>
                <ArrowUp />
              </Button>
            )}
          </div>
          <div className="flex flex-wrap items-center justify-between gap-2 text-2xs text-muted-foreground">
            <label className="flex items-center gap-1.5">
              {t("chat.asOf")}
              <Input
                type="date"
                className="h-7 w-36 text-xs"
                value={effectiveAsOf || meta.data?.today || ""}
                onChange={(e) => void changeAsOf(e.target.value)}
              />
              {effectiveAsOf && effectiveAsOf !== meta.data?.today && (
                <button type="button" className="cursor-pointer text-primary" onClick={() => void changeAsOf("")}>
                  {t("chat.asOfToday")}
                </button>
              )}
            </label>
            <span>{meta.data && t("chat.dataDate", { date: meta.data.corpus_as_of })}</span>
          </div>
        </div>
      </form>
    </div>
  );
}

function dropLastAnswer(ms: Message[]) {
  const i = ms.map((m) => m.role).lastIndexOf("assistant");
  return i === ms.length - 1 ? ms.slice(0, i) : ms;
}

export function UserBubble({ text }: { text: string }) {
  return (
    <div className="flex justify-end">
      <div className="max-w-[85%] rounded-lg bg-accent px-3.5 py-2 text-sm whitespace-pre-wrap text-accent-foreground">
        {text}
      </div>
    </div>
  );
}

/** A stored answer. readOnly leaves out rating and regenerating, for review. */
export function AssistantStored({
  m,
  last,
  onRegenerate,
  readOnly,
}: {
  m: Message;
  last?: boolean;
  onRegenerate?: () => void;
  readOnly?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      {m.steps && <StepsView steps={m.steps} />}
      {m.cards?.map((c, i) => <CardView key={i} card={c} />)}
      {m.clarify && <ClarifyView text={m.clarify} />}
      {m.refusal ? <RefusalView refusal={m.refusal} /> : m.claims?.map((c, i) => <ClaimView key={i} claim={c} />)}
      {!m.refusal && m.gaps && <GapsView gaps={m.gaps} />}
      {!!m.dropped && <p className="text-2xs text-muted-foreground">{t("chat.dropped", { n: m.dropped })}</p>}
      {m.claims && m.claims.length > 0 && <SourcesList claims={m.claims} />}
      {m.error && <ErrorView kind="server" message={m.error} />}
      {!readOnly && (
        <div className="flex items-center gap-2">
          <Feedback messageId={m.id} initial={m.rating} />
          {last && onRegenerate && <RegenerateButton onClick={onRegenerate} />}
        </div>
      )}
    </div>
  );
}

function Empty({ onPick }: { onPick: (q: string) => void }) {
  const { t } = useTranslation();
  const meta = useQuery({ queryKey: ["meta"], queryFn: api.meta, staleTime: 60_000 });
  const keys = ["s1", "s2", "s3", "s4"];
  return (
    <div className="space-y-6 py-10 text-center">
      <div className="space-y-2">
        <h1 className="text-xl font-semibold">{t("chat.emptyTitle")}</h1>
        <p className="mx-auto max-w-xl text-sm text-muted-foreground">{t("chat.emptySubtitle")}</p>
      </div>
      {meta.data && !meta.data.llm_ready && <ErrorView kind="no_model" message="" />}
      <div className="space-y-2">
        <div className="text-2xs font-medium tracking-wide text-muted-foreground uppercase">
          {t("chat.suggestionsTitle")}
        </div>
        <div className="grid gap-2 sm:grid-cols-2">
          {keys.map((k) => (
            <button
              key={k}
              type="button"
              disabled={!meta.data?.llm_ready}
              onClick={() => onPick(t("chat.suggestions." + k))}
              className="cursor-pointer rounded-md border bg-surface p-3 text-left text-sm hover:bg-muted disabled:cursor-not-allowed disabled:opacity-60"
            >
              {t("chat.suggestions." + k)}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}

function ClarifyView({ text }: { text: string }) {
  const { t } = useTranslation();
  return (
    <div className="rounded-md border border-primary/40 bg-accent/40 p-3 text-sm">
      <div className="mb-1 text-2xs font-medium text-accent-foreground">{t("chat.clarify")}</div>
      {text}
    </div>
  );
}
