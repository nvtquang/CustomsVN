import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { ArrowLeft, Check, ClipboardPlus, ThumbsDown, ThumbsUp, X } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox, Field, Input, Select, Textarea } from "@/components/ui/form";
import { AdminOnly, ErrorLine, PageTitle, Skeleton } from "@/features/admin/admin-only";
import { AssistantStored, UserBubble } from "@/features/chat/chat-view";
import { adminApi, type FeedbackDetail, type FeedbackRow, type GoldDraft } from "@/lib/api/admin";
import { api } from "@/lib/api/client";
import { cn, when } from "@/lib/utils";
import { useUI } from "@/stores/ui";

interface Search {
  status: string;
  rating: number;
  id?: number;
}

export const Route = createFileRoute("/admin/feedback")({
  validateSearch: (s: Record<string, unknown>): Search => ({
    status: typeof s.status === "string" ? s.status : "open",
    rating: s.rating === 1 || s.rating === "1" ? 1 : s.rating === 0 || s.rating === "0" ? 0 : -1,
    id: s.id == null ? undefined : Number(s.id) || undefined,
  }),
  component: () => (
    <AdminOnly>
      <FeedbackPage />
    </AdminOnly>
  ),
});

function FeedbackPage() {
  const { t } = useTranslation();
  const search = Route.useSearch();
  const navigate = useNavigate({ from: Route.fullPath });
  const list = useQuery({
    queryKey: ["admin", "feedback", search.status, search.rating],
    queryFn: () => adminApi.feedback(search.status === "all" ? "" : search.status, search.rating),
  });
  const set = (patch: Partial<Search>) => void navigate({ search: (s: Search) => ({ ...s, ...patch }) });
  const tabs = ["open", "resolved", "dismissed", "all"];
  const count = (st: string) => {
    const c = list.data?.counts ?? {};
    const rs = search.rating === 0 ? ["1", "-1"] : [String(search.rating)];
    const sts = st === "all" ? ["open", "resolved", "dismissed"] : [st];
    return sts.reduce((n, s) => n + rs.reduce((m, r) => m + (c[s + ":" + r] ?? 0), 0), 0);
  };
  return (
    <div className="mx-auto max-w-7xl space-y-4 p-4 lg:p-6">
      <PageTitle title={t("review.title")} subtitle={t("review.subtitle")} />
      <div className="flex flex-wrap items-center gap-2">
        <div role="tablist" className="flex flex-wrap rounded-md bg-muted p-0.5">
          {tabs.map((st) => (
            <button
              key={st}
              role="tab"
              type="button"
              aria-selected={search.status === st}
              onClick={() => set({ status: st, id: undefined })}
              className={cn(
                "cursor-pointer rounded-sm px-3 py-1 text-xs font-medium text-muted-foreground",
                search.status === st && "bg-surface text-foreground shadow-xs",
              )}
            >
              {t("review.status." + st)} <span className="tabular-nums">({count(st)})</span>
            </button>
          ))}
        </div>
        <Select
          aria-label={t("review.rating")}
          className="h-8 w-40 text-xs"
          value={search.rating}
          onChange={(e) => set({ rating: Number(e.target.value), id: undefined })}
        >
          <option value={-1}>{t("review.down")}</option>
          <option value={1}>{t("review.up")}</option>
          <option value={0}>{t("review.both")}</option>
        </Select>
      </div>
      <ErrorLine error={list.error} />
      <div className="grid gap-4 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
        <div className={cn(search.id != null && "hidden lg:block")}>
          {list.isPending ? (
            <Skeleton rows={6} />
          ) : list.data?.items.length ? (
            <ul className="space-y-2">
              {list.data.items.map((f) => (
                <li key={f.id}>
                  <FeedbackItem f={f} selected={search.id === f.id} onSelect={() => set({ id: f.id })} />
                </li>
              ))}
            </ul>
          ) : (
            <p className="rounded-md border p-6 text-center text-sm text-muted-foreground">{t("review.empty")}</p>
          )}
        </div>
        <div className={cn(search.id == null && "hidden lg:block")}>
          {search.id != null ? (
            <Detail id={search.id} labels={list.data?.labels ?? []} onBack={() => set({ id: undefined })} />
          ) : (
            <p className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
              {t("review.pick")}
            </p>
          )}
        </div>
      </div>
    </div>
  );
}

function FeedbackItem({ f, selected, onSelect }: { f: FeedbackRow; selected: boolean; onSelect: () => void }) {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-current={selected}
      className={cn(
        "w-full cursor-pointer space-y-1 rounded-md border bg-surface p-3 text-left text-sm hover:bg-muted",
        selected && "border-primary bg-accent",
      )}
    >
      <div className="flex items-center gap-2 text-2xs text-muted-foreground">
        {f.rating < 0 ? (
          <ThumbsDown className="size-3.5 text-destructive" aria-label={t("review.down")} />
        ) : (
          <ThumbsUp className="size-3.5 text-primary" aria-label={t("review.up")} />
        )}
        <span>{when(f.created_at)}</span>
        <span className="truncate">{f.user}</span>
        {f.label && <Badge variant="outline">{t("review.labels." + f.label)}</Badge>}
        {f.gold_item_id && <Badge variant="accent">{t("review.inGold")}</Badge>}
      </div>
      <div className="line-clamp-2 font-medium">{f.question || "—"}</div>
      {f.reason && <div className="line-clamp-2 text-xs text-muted-foreground">“{f.reason}”</div>}
    </button>
  );
}

function Detail({ id, labels, onBack }: { id: number; labels: string[]; onBack: () => void }) {
  const { t } = useTranslation();
  const q = useQuery({ queryKey: ["admin", "feedback", "detail", id], queryFn: () => adminApi.feedbackDetail(id) });
  if (q.error) return <ErrorLine error={q.error} />;
  if (!q.data) return <Skeleton rows={8} />;
  const d = q.data;
  return (
    <div className="space-y-4">
      <Button variant="ghost" size="sm" className="lg:hidden" onClick={onBack}>
        <ArrowLeft />
        {t("review.back")}
      </Button>
      <ReviewForm d={d} labels={labels.length ? labels : d.labels} />
      <Card>
        <CardHeader>
          <CardTitle>{t("review.conversation")}</CardTitle>
          <Link to="/c/$id" params={{ id: String(d.conversation.id) }} className="text-2xs underline">
            {d.conversation.title}
          </Link>
        </CardHeader>
        <CardContent className="space-y-4">
          {d.messages.map((m) => (
            <div
              key={m.id}
              className={cn(m.id === d.feedback.message_id && "-mx-2 rounded-md p-2 ring-2 ring-destructive/40")}
            >
              {m.role === "user" ? <UserBubble text={m.content} /> : <AssistantStored m={m} readOnly />}
            </div>
          ))}
        </CardContent>
      </Card>
      <TraceView d={d} />
      <GoldForm d={d} />
    </div>
  );
}

function ReviewForm({ d, labels }: { d: FeedbackDetail; labels: string[] }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const f = d.feedback;
  const [label, setLabel] = useState(f.label);
  const [note, setNote] = useState(f.review_note);
  const review = useMutation({
    mutationFn: (status?: string) => adminApi.review(f.id, { status, label, review_note: note }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["admin", "feedback"] }),
  });
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {f.rating < 0 ? (
            <ThumbsDown className="size-4 text-destructive" />
          ) : (
            <ThumbsUp className="size-4 text-primary" />
          )}
          {t("review.feedback")}
        </CardTitle>
        <Badge variant={f.status === "open" ? "partly_expired" : f.status === "resolved" ? "in_force" : "unknown"}>
          {t("review.status." + f.status)}
        </Badge>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="text-xs text-muted-foreground">
          {f.user || "—"} · {when(f.created_at)}
          {f.reviewer && " · " + t("review.reviewedBy", { who: f.reviewer, at: when(f.reviewed_at) })}
        </div>
        <blockquote className="border-l-2 pl-3 text-sm">{f.reason || t("review.noReason")}</blockquote>
        <div className="grid gap-3 sm:grid-cols-[12rem_1fr]">
          <Field label={t("review.label")}>
            <Select value={label} onChange={(e) => setLabel(e.target.value)}>
              <option value="">—</option>
              {labels.map((l) => (
                <option key={l} value={l}>
                  {t("review.labels." + l)}
                </option>
              ))}
            </Select>
          </Field>
          <Field label={t("review.note")}>
            <Textarea className="min-h-9" rows={2} value={note} onChange={(e) => setNote(e.target.value)} />
          </Field>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button size="sm" onClick={() => review.mutate("resolved")} disabled={review.isPending}>
            <Check />
            {t("review.resolve")}
          </Button>
          <Button size="sm" variant="outline" onClick={() => review.mutate("dismissed")} disabled={review.isPending}>
            <X />
            {t("review.dismiss")}
          </Button>
          <Button size="sm" variant="ghost" onClick={() => review.mutate(undefined)} disabled={review.isPending}>
            {t("review.saveNote")}
          </Button>
          {f.status !== "open" && (
            <Button size="sm" variant="ghost" onClick={() => review.mutate("open")} disabled={review.isPending}>
              {t("review.reopen")}
            </Button>
          )}
        </div>
        {review.isSuccess && <p className="text-xs text-primary">{t("common.saved")}</p>}
        <ErrorLine error={review.error} />
      </CardContent>
    </Card>
  );
}

function TraceView({ d }: { d: FeedbackDetail }) {
  const { t } = useTranslation();
  const openChunk = useUI((s) => s.openChunk);
  const tr = d.trace;
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("review.trace")}</CardTitle>
        <span className="text-2xs text-muted-foreground">
          {[tr.provider, tr.model].filter(Boolean).join(" / ") || "—"}
        </span>
      </CardHeader>
      <CardContent className="space-y-3 text-xs">
        <dl className="grid grid-cols-[9rem_1fr] gap-x-3 gap-y-1.5">
          {tr.standalone && (
            <>
              <dt className="text-muted-foreground">{t("review.standalone")}</dt>
              <dd>{tr.standalone}</dd>
            </>
          )}
          {tr.intent && (
            <>
              <dt className="text-muted-foreground">{t("review.intent")}</dt>
              <dd>
                {tr.intent.kind} ({tr.intent.by})
              </dd>
            </>
          )}
          {tr.timing_ms && (
            <>
              <dt className="text-muted-foreground">{t("review.timing")}</dt>
              <dd className="tabular-nums">
                {Object.entries(tr.timing_ms)
                  .map(([k, v]) => `${k} ${Math.round(v)} ms`)
                  .join(" · ")}
                {tr.first_claim_ms ? ` · ${t("review.firstClaim")} ${Math.round(tr.first_claim_ms)} ms` : ""}
              </dd>
            </>
          )}
          {tr.refusal && (
            <>
              <dt className="text-muted-foreground">{t("review.refusal")}</dt>
              <dd>
                {tr.refusal.code}: {tr.refusal.reason}
              </dd>
            </>
          )}
          {tr.error && (
            <>
              <dt className="text-muted-foreground">{t("review.error")}</dt>
              <dd className="text-destructive">{tr.error}</dd>
            </>
          )}
        </dl>
        {tr.dropped && tr.dropped.length > 0 && (
          <div>
            <div className="mb-1 font-medium">{t("review.dropped", { n: tr.dropped.length })}</div>
            <ul className="space-y-1">
              {tr.dropped.map((x, i) => (
                <li key={i} className="rounded border bg-surface-2 p-2">
                  <Badge variant="expired" className="mr-1.5">
                    {x.reason}
                  </Badge>
                  {x.text}
                </li>
              ))}
            </ul>
          </div>
        )}
        {tr.retrieved && tr.retrieved.length > 0 && (
          <div>
            <div className="mb-1 font-medium">{t("review.retrieved", { n: tr.retrieved.length })}</div>
            <ol className="list-decimal space-y-0.5 pl-5">
              {tr.retrieved.map((id) => (
                <li key={id}>
                  <button
                    type="button"
                    className="cursor-pointer font-mono text-2xs underline"
                    onClick={() => openChunk(id)}
                  >
                    {id}
                  </button>
                </li>
              ))}
            </ol>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function GoldForm({ d }: { d: FeedbackDetail }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const sets = useQuery({ queryKey: ["goldsets"], queryFn: api.goldSets });
  const g = d.gold;
  const [draft, setDraft] = useState<GoldDraft>({
    gold_set_id: 0,
    question: g?.question ?? d.trace.standalone ?? d.feedback.question,
    expected_provisions: g?.expected_provisions ?? "",
    expected_answer: g?.expected_answer ?? "",
    as_of_date: g?.as_of_date ?? "",
    category: g?.category ?? "feedback",
    should_refuse: g?.should_refuse ?? false,
    notes: d.feedback.reason,
  });
  const save = useMutation({
    mutationFn: () => adminApi.toGold(d.feedback.id, draft),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["admin", "feedback"] }),
  });
  const set = (p: Partial<GoldDraft>) => setDraft({ ...draft, ...p });
  return (
    <Card>
      <CardHeader>
        <CardTitle>{g ? t("review.goldUpdate") : t("review.toGold")}</CardTitle>
        {g && <Badge variant="accent">{g.id}</Badge>}
      </CardHeader>
      <CardContent>
        <form
          className="grid gap-3 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            save.mutate();
          }}
        >
          <Field label={t("review.goldSet")} className="sm:col-span-2">
            <Select value={draft.gold_set_id} onChange={(e) => set({ gold_set_id: Number(e.target.value) })}>
              <option value={0}>{t("review.feedbackSet")}</option>
              {sets.data
                ?.filter((s) => s.name !== "feedback")
                .map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.name} v{s.version}
                  </option>
                ))}
            </Select>
          </Field>
          <Field label={t("review.fields.question")} className="sm:col-span-2">
            <Textarea rows={2} required value={draft.question} onChange={(e) => set({ question: e.target.value })} />
          </Field>
          <Field label={t("review.fields.expected_provisions")} className="sm:col-span-2">
            <Input
              placeholder={t("review.provisionsHint")}
              value={draft.expected_provisions}
              disabled={draft.should_refuse}
              onChange={(e) => set({ expected_provisions: e.target.value })}
            />
          </Field>
          <Field label={t("review.fields.expected_answer")} className="sm:col-span-2">
            <Textarea
              rows={2}
              value={draft.expected_answer}
              onChange={(e) => set({ expected_answer: e.target.value })}
            />
          </Field>
          <Field label={t("review.fields.as_of_date")}>
            <Input type="date" value={draft.as_of_date} onChange={(e) => set({ as_of_date: e.target.value })} />
          </Field>
          <Field label={t("review.fields.category")}>
            <Input value={draft.category} onChange={(e) => set({ category: e.target.value })} />
          </Field>
          <Checkbox
            className="sm:col-span-2"
            label={t("review.shouldRefuse")}
            checked={draft.should_refuse}
            onChange={(e) =>
              set({
                should_refuse: e.target.checked,
                expected_provisions: e.target.checked ? "" : draft.expected_provisions,
              })
            }
          />
          <div className="flex flex-wrap items-center gap-3 sm:col-span-2">
            <Button type="submit" disabled={save.isPending}>
              <ClipboardPlus />
              {g ? t("review.goldUpdate") : t("review.toGold")}
            </Button>
            {save.data && (
              <span role="status" className="text-xs text-primary">
                {t("review.goldSaved", { ids: save.data.expected_ids.join(", ") || "—" })}
              </span>
            )}
          </div>
          <div className="sm:col-span-2">
            <ErrorLine error={save.error} />
          </div>
        </form>
      </CardContent>
    </Card>
  );
}
