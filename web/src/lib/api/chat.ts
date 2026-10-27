// The chat half of the API: conversations, the answer stream, documents,
// feedback and LLM settings.

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

export interface Message {
  id: number;
  role: "user" | "assistant";
  content: string;
  refused: boolean;
  claims?: Claim[];
  refusal?: Refusal;
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
  | { type: "status"; stage: "rewrite" | "retrieve" | "compose"; sources?: number }
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
  if (!res.ok || !res.body) {
    let message = res.statusText;
    try {
      message = (await res.json()).error ?? message;
    } catch {
      // not JSON
    }
    onEvent({ type: "error", kind: res.status >= 500 ? "server" : "request", message });
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

export interface LLMProvider {
  name: string;
  kind: "anthropic" | "openai" | "luatdo";
  base_url: string;
  api_key_env: string;
  models: string[];
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

export interface LLMSettings {
  providers: LLMProvider[];
  roles: Record<string, LLMTarget[]>;
  master_key: boolean;
}

export interface LLMTest {
  ok: boolean;
  text?: string;
  model?: string;
  latency_ms: number;
  kind?: string;
  error?: string;
}

export const chatApi = {
  conversations: (q = "") => json<Conversation[]>("GET", "/api/conversations?q=" + encodeURIComponent(q)),
  conversation: (id: number) =>
    json<{ conversation: Conversation; messages: Message[] }>("GET", "/api/conversations/" + id),
  rename: (id: number, title: string) => json("PATCH", "/api/conversations/" + id, { title }),
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
  testLLM: (provider: string, model: string) => json<LLMTest>("POST", "/api/settings/llm/test", { provider, model }),
};
