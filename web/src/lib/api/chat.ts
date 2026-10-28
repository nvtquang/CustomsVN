// The chat half of the API: conversations, the answer stream, documents,
// feedback and LLM settings.

import { sessionEnded } from "../query";

export interface Citation {
  n: number;
  chunk_id: string;
  doc_id: string;
  number: string;
  title: string;
  path: string;
  quote: string;
  force: string;
  verified: boolean;
}

export interface Claim {
  text: string;
  citations: Citation[];
}

export interface Refusal {
  code: string;
  reason: string;
  suggestions?: string[];
}

export interface RegDoc {
  id: string;
  number: string;
  title: string;
  doc_type?: string;
  issuing_body?: string;
  signed_on?: string;
  effective_from?: string;
  expired_on?: string;
  force_status?: string;
  source_url?: string;
  in_corpus: boolean;
}

export interface DocLink extends RegDoc {
  relation: string;
  label?: string;
}

export interface StatusCard extends RegDoc {
  as_of: string;
  force: string;
  consolidated_in?: RegDoc;
  replaced_by?: DocLink[];
  note?: string;
}

export interface TimelineCard {
  doc: RegDoc;
  as_of: string;
  force: string;
  events: { date: string; kind: string; doc: RegDoc; label?: string; undated?: boolean }[];
}

export interface HSStep {
  code?: string;
  description_vi: string;
  level: number;
}

export interface HSLine {
  position: number;
  code: string;
  level: number;
  description_vi: string;
  description_en: string;
  unit?: string;
  chapter: string;
  path?: HSStep[];
  children?: HSStep[];
  score?: number;
}

export interface HSCard {
  query: string;
  by_code: boolean;
  lines: HSLine[];
  source_doc_id: string;
  source: string;
}

export interface GraphCard {
  query: string;
  doc: RegDoc;
  rows: DocLink[];
}

export type Card =
  | { type: "status"; data: StatusCard }
  | { type: "amendments"; data: TimelineCard }
  | { type: "hs"; data: HSCard }
  | { type: "graph"; data: GraphCard };

export interface Step {
  n: number;
  tool: string;
  args: Record<string, unknown>;
  summary?: string;
  error?: string;
  by: "plan" | "model";
  ms: number;
}

export interface Intent {
  kind: string;
  clarify?: string;
  by: string;
}

export interface Message {
  id: number;
  role: "user" | "assistant";
  content: string;
  refused: boolean;
  claims?: Claim[];
  refusal?: Refusal;
  clarify?: string;
  intent?: Intent;
  steps?: Step[];
  cards?: Card[];
  dropped?: number;
  as_of_date?: string;
  corpus_version?: number;
  error?: string;
  rating?: number;
  created_at: string;
}

export interface Conversation {
  id: number;
  title: string;
  as_of_date?: string;
  messages: number;
  created_at: string;
  updated_at: string;
}

export type ChatEvent =
  | { type: "meta"; conversation_id: number; as_of: string; question: string }
  | { type: "status"; stage: "rewrite" | "retrieve" | "agent" | "compose"; sources?: number }
  | { type: "intent"; intent: Intent }
  | { type: "step"; step: Step }
  | { type: "card"; card: Card }
  | { type: "clarify"; text: string }
  | { type: "rewrite"; text: string }
  | { type: "claim"; claim: Claim }
  | { type: "dropped"; dropped: { text: string; reason: string } }
  | { type: "refusal"; refusal: Refusal }
  | {
      type: "done";
      message_id: number;
      conversation_id: number;
      refused: boolean;
      dropped: number;
      first_claim_ms: number;
      corpus_version: number;
      model: string;
    }
  | { type: "error"; kind: string; message: string };

/**
 * Streams an answer. Events arrive as server-sent events over a POST, which
 * EventSource cannot send, so the stream is read by hand.
 */
export async function streamChat(
  body: { conversation_id?: number; question?: string; as_of?: string; regenerate?: boolean },
  onEvent: (e: ChatEvent) => void,
  signal: AbortSignal,
): Promise<void> {
  const res = await fetch("/api/chat", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    signal,
  });
  if (res.status === 401) sessionEnded();
  if (!res.ok || !res.body) {
    let message = res.statusText;
    try {
      message = (await res.json()).error ?? message;
    } catch {
      // not JSON
    }
    onEvent({
      type: "error",
      kind: res.status === 429 ? "rate_limit" : res.status >= 500 ? "server" : "request",
      message,
    });
    return;
  }
  const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
  let buf = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buf += value;
    let i;
    while ((i = buf.indexOf("\n\n")) >= 0) {
      const block = buf.slice(0, i);
      buf = buf.slice(i + 2);
      let event = "";
      let data = "";
      for (const line of block.split("\n")) {
        if (line.startsWith("event:")) event = line.slice(6).trim();
        else if (line.startsWith("data:")) data += line.slice(5).trim();
      }
      if (!event || !data) continue;
      onEvent({ ...JSON.parse(data), type: event } as ChatEvent);
    }
  }
}

async function json<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (res.status === 204) return undefined as T;
  if (res.status === 401) sessionEnded();
  const data = await res.json().catch(() => null);
  if (!res.ok) throw new Error((data && data.error) || res.statusText);
  return data as T;
}

export interface ProvisionNode {
  id: string;
  parent_id?: string;
  kind: string;
  number: string;
  heading?: string;
  text?: string;
  position: number;
}

export type ProviderKind = "anthropic" | "openai" | "luatdo" | "voyage" | "jina" | "cohere";
export type ModelType = "chat" | "embedding" | "rerank";

export interface EmbeddingModel {
  model: string;
  dims: number;
}

export interface LLMProvider {
  name: string;
  kind: ProviderKind;
  base_url: string;
  api_key_env: string;
  /** Chat models. */
  models: string[];
  embeddings?: EmbeddingModel[];
  rerankers?: string[];
  has_key?: boolean;
  key_hint?: string;
  env_key_set?: boolean;
  api_key?: string;
  clear_key?: boolean;
}

export interface LLMTarget {
  provider: string;
  model: string;
}

export interface LLMPrice {
  input: number;
  output: number;
}

export interface LLMSettings {
  providers: LLMProvider[];
  roles: Record<string, LLMTarget[]>;
  /** USD per million tokens, keyed by "provider/model". */
  prices: Record<string, LLMPrice>;
  master_key: boolean;
}

export interface LLMTest {
  ok: boolean;
  text?: string;
  model?: string;
  latency_ms: number;
  kind?: string;
  error?: string;
  dims?: number;
  scores?: number[];
}

export const chatApi = {
  conversations: (q = "") => json<Conversation[]>("GET", "/api/conversations?q=" + encodeURIComponent(q)),
  conversation: (id: number) =>
    json<{ conversation: Conversation; messages: Message[] }>("GET", "/api/conversations/" + id),
  rename: (id: number, title: string) => json("PATCH", "/api/conversations/" + id, { title }),
  setAsOf: (id: number, as_of_date: string) => json("PATCH", "/api/conversations/" + id, { as_of_date }),
  hs: (q: string) => json<HSCard>("GET", "/api/hs?q=" + encodeURIComponent(q)),
  tool: <T>(name: string, args: Record<string, unknown>, as_of = "") =>
    json<{ text: string; summary: string; card?: { type: string; data: T } }>("POST", "/api/tools/" + name, {
      args,
      as_of,
    }),
  remove: (id: number) => json("DELETE", "/api/conversations/" + id),
  feedback: (message_id: number, rating: number, reason: string) =>
    json("POST", "/api/feedback", { message_id, rating, reason }),
  document: (id: string) =>
    json<{
      doc: import("./types").Doc;
      force_today: string;
      provisions: ProvisionNode[];
      chunks: { id: string; path: string[]; chunk_kind: string }[];
      chunk_text?: Record<string, string>;
      related: { consolidated_in?: import("./types").Doc; consolidates?: import("./types").Doc };
    }>("GET", "/api/documents/" + encodeURIComponent(id)),
  llm: () => json<LLMSettings>("GET", "/api/settings/llm"),
  saveLLM: (s: LLMSettings) => json<{ settings: LLMSettings; problems: string[] }>("PUT", "/api/settings/llm", s),
  /** Tries a model of a stored provider (given by name) or of one being set up. */
  testModel: (provider: Partial<LLMProvider> & { name: string }, type: ModelType, model: string) =>
    json<LLMTest>("POST", "/api/settings/llm/test", { provider, type, model }),
};
