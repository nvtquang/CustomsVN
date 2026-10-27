import type {
  Chunk,
  Doc,
  GoldItem,
  GoldSet,
  ImportResult,
  Meta,
  RetrievalSettings,
  Run,
  RunResult,
  SearchRequest,
  SearchResponse,
} from "./types";

export class ApiError extends Error {
  status: number;
  body: unknown;
  constructor(status: number, message: string, body: unknown) {
    super(message);
    this.status = status;
    this.body = body;
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  let data: unknown = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = text;
  }
  if (!res.ok) {
    const msg = data && typeof data === "object" && "error" in data ? String(data.error) : res.statusText;
    throw new ApiError(res.status, msg, data);
  }
  return data as T;
}

export const api = {
  meta: () => request<Meta>("GET", "/api/meta"),
  search: (req: SearchRequest) => request<SearchResponse>("POST", "/api/retrieval/test", req),
  chunk: (id: string) =>
    request<{ chunk: Chunk; doc: Doc; force_today: string }>("GET", "/api/chunks/" + encodeURIComponent(id)),
  settings: () => request<RetrievalSettings>("GET", "/api/settings/retrieval"),
  saveSettings: (s: RetrievalSettings) => request<RetrievalSettings>("PUT", "/api/settings/retrieval", s),
  goldSets: () => request<GoldSet[]>("GET", "/api/goldsets"),
  goldSet: (id: number) =>
    request<{ gold_set: GoldSet; items: (GoldItem & { db_id: number })[] }>("GET", "/api/goldsets/" + id),
  importGoldSet: (body: {
    name: string;
    format: string;
    content: string;
    note?: string;
    dry_run: boolean;
    force?: boolean;
  }) => request<ImportResult>("POST", "/api/goldsets", body),
  runs: () => request<Run[]>("GET", "/api/retrieval/runs"),
  run: (id: number) => request<{ run: Run; results: RunResult[] | null }>("GET", "/api/retrieval/runs/" + id),
  startRun: (body: { gold_set_id: number; label: string; request: SearchRequest }) =>
    request<{ id: number }>("POST", "/api/retrieval/runs", body),
};
