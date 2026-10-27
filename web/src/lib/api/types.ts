// Types of the customsvn HTTP API (internal/httpapi, internal/search, internal/evalx).

export type Mode = "hybrid" | "bm25" | "vector";

export interface Filters {
  tiers?: number[];
  as_of?: string;
  doc_types?: string[];
  source_kinds?: string[];
  docs?: string[];
  include_expired?: boolean;
}

export interface SearchConfig {
  mode: Mode;
  k: number;
  bm25_k: number;
  vector_k: number;
  rrf_k: number;
  bm25_weight: number;
  vector_weight: number;
  model?: string;
  rerank: boolean;
  rerank_k: number;
  keep_originals: boolean;
  ignore_refs: boolean;
  duplicates: number;
}

export interface SearchRequest {
  query: string;
  filters: Filters;
  config: Partial<SearchConfig>;
}

export interface Doc {
  id: string;
  official_number: string;
  doc_type: string;
  title: string;
  issuing_body: string;
  signed_on?: string;
  effective_from?: string;
  expired_on?: string;
  force_status: string;
  status: string;
  tier: number;
  source_kind: string;
  source_url: string;
  consolidates?: string;
  consolidated_in?: string;
}

export interface Chunk {
  id: string;
  doc_id: string;
  path: string[];
  text: string;
  chunk_kind: string;
  tier: number;
  source_kind: string;
  amendment_notes?: string[];
  needs_review?: boolean;
  text_source?: string;
}

export interface Component {
  rank: number;
  score: number;
}

export type Force = "in_force" | "partly_expired" | "not_yet_in_force" | "";

export interface Hit {
  rank: number;
  chunk_id: string;
  score: number;
  direct?: boolean;
  bm25?: Component;
  vector?: Component;
  named?: Component;
  rrf: number;
  rerank?: number;
  replaced?: string;
  duplicates?: string[];
  chunk: Chunk;
  doc: Doc;
  force: Force;
  usable: boolean;
}

export interface ProvisionRef {
  point?: string;
  clause?: string;
  article: string;
  number?: string;
  name?: string;
  span: string;
}

export interface SearchResponse {
  query: string;
  filters: Filters;
  config: SearchConfig;
  corpus_version: number;
  refs: { provisions?: ProvisionRef[]; numbers?: string[]; hs_codes?: string[]; dates?: string[] };
  resolved?: { ref: ProvisionRef; doc_id?: string; chunk_id?: string; note?: string }[];
  in_scope_docs: number;
  in_scope_chunks: number;
  hits: Hit[];
  notes?: string[];
  timing_ms: Record<string, number>;
  expired_in_top_k: number;
}

export interface Meta {
  corpus_version: number;
  corpus_as_of: string;
  ingested_at: string;
  documents: number;
  chunks: number;
  embedding_models: string[] | null;
  default_model: string;
  rerank: boolean;
  defaults: SearchConfig;
  today: string;
  llm_ready: boolean;
}

export interface RetrievalSettings {
  filters: Filters;
  config: SearchConfig;
}

export interface GoldItem {
  id: string;
  question: string;
  expected_provisions: string;
  expected_ids?: string[];
  as_of_date?: string;
  category?: string;
  should_refuse: boolean;
  notes?: string;
}

export interface RowError {
  row: number;
  id?: string;
  field: string;
  message: string;
}

export interface GoldSet {
  id: number;
  name: string;
  version: number;
  format: string;
  note: string;
  items: number;
  created_at: string;
}

export interface ImportResult {
  items: GoldItem[];
  errors: RowError[] | null;
  count: number;
  gold_set?: GoldSet;
}

export interface Metrics {
  questions: number;
  scored: number;
  refuse: number;
  errors: number;
  recall: Record<string, number>;
  hit_at_10: number;
  mrr: number;
  ndcg10: number;
  expired_rate: number;
  latency_p50_ms: number;
  latency_p95_ms: number;
}

export interface Run {
  id: number;
  label: string;
  gold_set_id: number | null;
  gold_set_name?: string;
  corpus_version: number;
  request: SearchRequest;
  status: "running" | "done" | "failed";
  progress: number;
  total: number;
  metrics?: Metrics;
  error?: string;
  created_at: string;
  finished_at?: string;
}

export interface HitBrief {
  rank: number;
  chunk_id: string;
  number: string;
  path: string;
  score: number;
  usable: boolean;
  relevant: boolean;
}

export interface RunResult {
  item: GoldItem & { db_id: number };
  ranks: number[];
  metrics: { recall: Record<string, number>; rr: number; ndcg10: number; expired_in_top10: number; error?: string };
  hits: HitBrief[] | null;
  latency_ms: number;
}
