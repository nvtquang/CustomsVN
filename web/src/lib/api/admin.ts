import { request } from "./client";
import type { Claim, Message } from "./chat";
import type { LLMTarget } from "./chat";

export interface User {
  id: number;
  email: string;
  name: string;
  role: "admin" | "user";
  disabled: boolean;
  created_at: string;
  last_login?: string;
}

export interface CountRow {
  key: string;
  documents: number;
  chunks: number;
}

export interface CorpusVersion {
  id: number;
  created_at: string;
  as_of: string;
  source_revision: string;
  documents: number;
  chunks: number;
  added: number;
  changed: number;
  removed: number;
}

export interface CorpusAdmin {
  versions: CorpusVersion[];
  current: {
    version: number;
    documents: number;
    chunks: number;
    today: string;
    by_tier: CountRow[];
    by_type: CountRow[];
    by_source: CountRow[];
    by_force: CountRow[];
  };
  embeddings: { model: string; provider: string; dims: number; chunks: number; hs: number; loaded: boolean }[];
  registry: number;
  edges: number;
  hs_lines: number;
  models: string[] | null;
  coverage: {
    available: boolean;
    path?: string;
    written_at?: string;
    counts?: Record<string, number>;
    rows?: Record<string, string>[];
  };
}

export type JobStatus = "queued" | "running" | "done" | "failed" | "canceled";

export interface Job {
  id: number;
  kind: "prep" | "ingest" | "embed";
  params: Record<string, unknown>;
  status: JobStatus;
  progress: string;
  log?: string;
  user?: string;
  created_at: string;
  started_at?: string;
  finished_at?: string;
}

export interface FeedbackRow {
  id: number;
  rating: 1 | -1;
  reason: string;
  label: string;
  status: "open" | "resolved" | "dismissed";
  review_note: string;
  created_at: string;
  reviewed_at?: string;
  gold_item_id?: number;
  message_id: number;
  conversation_id: number;
  answer: string;
  refused: boolean;
  question: string;
  user: string;
  reviewer: string;
}

export interface Trace {
  standalone?: string;
  retrieved?: string[];
  dropped?: { text: string; reason: string }[];
  refusal?: { code: string; reason: string };
  provider?: string;
  model?: string;
  timing_ms?: Record<string, number>;
  first_claim_ms?: number;
  error?: string;
  clarify?: string;
  intent?: { kind: string; by: string };
  steps?: { n: number; tool: string; by: string; summary?: string; error?: string }[];
}

export interface FeedbackDetail {
  feedback: FeedbackRow;
  conversation: { id: number; title: string };
  messages: Message[];
  trace: Trace;
  gold?: {
    db_id: number;
    id: string;
    question: string;
    expected_provisions: string;
    expected_answer?: string;
    category?: string;
    should_refuse: boolean;
    as_of_date?: string;
  } | null;
  labels: string[];
}

export interface DayStats {
  day: string;
  questions: number;
  answers: number;
  refused: number;
  clarified: number;
  errors: number;
  users: number;
  first_claim_p50_ms: number;
  first_claim_p95_ms: number;
  total_p50_ms: number;
  total_p95_ms: number;
  cost_usd: number;
  eval_cost_usd: number;
  input_tokens: number;
  output_tokens: number;
  up: number;
  down: number;
}

export interface Dashboard {
  days: DayStats[];
  total: DayStats;
  unpriced: string[];
  open_down: number;
  zone: string;
}

export interface Limits {
  questions_per_minute: number;
  questions_per_day: number;
}

export interface AnswerConfig {
  roles?: Record<string, LLMTarget[]>;
  tool_loop?: boolean;
}

export interface AnswerMetrics {
  questions: number;
  answerable: number;
  refuse: number;
  answered: number;
  cards: number;
  refused: number;
  clarified: number;
  errors: number;
  refusal_accuracy: number;
  false_refusal: number;
  missed_refusal: number;
  supported: number;
  citation_precision: number;
  expected_cited: number;
  grounded: number;
  first_claim_p50_ms: number;
  first_claim_p95_ms: number;
  total_p50_ms: number;
  total_p95_ms: number;
  input_tokens: number;
  output_tokens: number;
  cost_usd: number;
}

export interface AnswerRun {
  id: number;
  label: string;
  gold_set_id?: number;
  gold_set_name?: string;
  corpus_version: number;
  config: AnswerConfig;
  status: "running" | "done" | "failed" | "canceled";
  progress: number;
  total: number;
  metrics?: AnswerMetrics;
  error?: string;
  created_at: string;
  finished_at?: string;
}

export interface AnswerScore {
  outcome: "answered" | "cards" | "refused" | "clarified" | "error";
  refusal_code?: string;
  refusal_correct: boolean;
  claims: number;
  dropped: number;
  citations: number;
  citations_correct: number;
  expected_cited: number;
  grounded: boolean;
  first_claim_ms?: number;
  total_ms: number;
  input_tokens: number;
  output_tokens: number;
  cost_usd: number;
  error?: string;
}

export interface AnswerResult {
  item: {
    db_id: number;
    id: string;
    question: string;
    expected_provisions: string;
    expected_ids?: string[];
    category?: string;
    should_refuse: boolean;
    as_of_date?: string;
  };
  score: AnswerScore;
  result: {
    claims?: Claim[];
    dropped?: { text: string; reason: string }[];
    refusal?: { code: string; reason: string };
    clarify?: string;
    model?: string;
    cards?: { type: string }[];
  };
}

export interface GoldDraft {
  gold_set_id: number;
  question: string;
  expected_provisions: string;
  expected_answer: string;
  as_of_date: string;
  category: string;
  should_refuse: boolean;
  notes: string;
}

export const authApi = {
  me: () => request<User>("GET", "/api/auth/me"),
  login: (email: string, password: string) => request<User>("POST", "/api/auth/login", { email, password }),
  logout: () => request<void>("POST", "/api/auth/logout"),
  changePassword: (current: string, next: string) =>
    request<{ ok: boolean }>("POST", "/api/auth/password", { current, new: next }),
};

/** The weekly crawl's schedule; installed says whether this server has the crawler. */
export interface CrawlSchedule {
  enabled: boolean;
  every: "weekly" | "monthly";
  weekday: number;
  day: number;
  hour: number;
  max_pages: number;
  installed?: boolean;
}

export const adminApi = {
  corpus: () => request<CorpusAdmin>("GET", "/api/admin/corpus"),
  jobs: () => request<Job[]>("GET", "/api/admin/jobs"),
  job: (id: number) => request<Job>("GET", "/api/admin/jobs/" + id),
  startJob: (kind: string, params: Record<string, unknown>) =>
    request<{ id: number }>("POST", "/api/admin/jobs", { kind, params }),
  cancelJob: (id: number) => request<{ ok: boolean }>("POST", "/api/admin/jobs/" + id + "/cancel"),
  crawl: () => request<CrawlSchedule>("GET", "/api/settings/crawl"),
  saveCrawl: (s: CrawlSchedule) => request<CrawlSchedule>("PUT", "/api/settings/crawl", s),
  feedback: (status: string, rating: number) =>
    request<{ items: FeedbackRow[]; counts: Record<string, number>; labels: string[] }>(
      "GET",
      `/api/admin/feedback?status=${encodeURIComponent(status)}&rating=${rating}`,
    ),
  feedbackDetail: (id: number) => request<FeedbackDetail>("GET", "/api/admin/feedback/" + id),
  review: (id: number, body: { status?: string; label?: string; review_note?: string }) =>
    request<{ ok: boolean }>("PATCH", "/api/admin/feedback/" + id, body),
  toGold: (id: number, body: GoldDraft) =>
    request<{ gold_set_id: number; gold_item_id: number; expected_ids: string[] }>(
      "POST",
      "/api/admin/feedback/" + id + "/gold",
      body,
    ),
  dashboard: (days: number) => request<Dashboard>("GET", "/api/admin/dashboard?days=" + days),
  users: () => request<User[]>("GET", "/api/admin/users"),
  createUser: (body: { email: string; name: string; role: string; password: string }) =>
    request<{ user: User; password?: string }>("POST", "/api/admin/users", body),
  updateUser: (id: number, body: { name?: string; role?: string; disabled?: boolean; password?: string }) =>
    request<User>("PATCH", "/api/admin/users/" + id, body),
  deleteUser: (id: number) => request<void>("DELETE", "/api/admin/users/" + id),
  limits: () => request<Limits>("GET", "/api/settings/limits"),
  saveLimits: (l: Limits) => request<Limits>("PUT", "/api/settings/limits", l),
  exportConfig: () => request<Record<string, unknown>>("GET", "/api/settings/export"),
  importConfig: (file: unknown) =>
    request<{ applied: string[]; providers_without_key: string[] }>("POST", "/api/settings/import", file),
  answerRuns: () => request<AnswerRun[]>("GET", "/api/answers/runs"),
  answerRun: (id: number) => request<{ run: AnswerRun; results: AnswerResult[] }>("GET", "/api/answers/runs/" + id),
  startAnswerRun: (body: { gold_set_id: number; label: string; config: AnswerConfig }) =>
    request<{ id: number }>("POST", "/api/answers/runs", body),
  cancelAnswerRun: (id: number) => request<{ ok: boolean }>("POST", "/api/answers/runs/" + id + "/cancel"),
};
