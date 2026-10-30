import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  AudioLines,
  Boxes,
  CheckCircle2,
  ChevronRight,
  CircleAlert,
  Cloud,
  ExternalLink,
  Eye,
  FileCode2,
  HardDrive,
  ListOrdered,
  Loader2,
  MessageSquare,
  Plus,
  RefreshCw,
  Save,
  Search,
  Shapes,
  Trash2,
  Zap,
} from "lucide-react";
import { useMemo, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input, Select } from "@/components/ui/form";
import { AdminOnly, ErrorLine, PageTitle, Skeleton } from "@/features/admin/admin-only";
import { api, request } from "@/lib/api/client";
import {
  chatApi,
  type LLMProvider,
  type LLMSettings,
  type LLMTarget,
  type LLMTest,
  type ModelType,
  type ProviderKind,
} from "@/lib/api/chat";
import { cn } from "@/lib/utils";

export const Route = createFileRoute("/settings/models")({
  component: () => (
    <AdminOnly>
      <ModelsPage />
    </AdminOnly>
  ),
});

/** Providers offered ready to fill in: the id is the provider's name in the settings. */
interface Preset {
  id: string;
  label: string;
  kind: ProviderKind;
  base: string;
  env: string;
  keyUrl?: string;
  noKey?: boolean;
  types: ModelType[];
  /** Model names to offer, per type. */
  suggest?: Partial<Record<ModelType, string[]>>;
}

const PRESETS: Preset[] = [
  {
    id: "anthropic",
    label: "Anthropic (Claude)",
    kind: "anthropic",
    base: "",
    env: "ANTHROPIC_API_KEY",
    keyUrl: "https://console.anthropic.com/settings/keys",
    types: ["chat"],
  },
  {
    id: "openai",
    label: "OpenAI",
    kind: "openai",
    base: "",
    env: "OPENAI_API_KEY",
    keyUrl: "https://platform.openai.com/api-keys",
    types: ["chat", "embedding"],
    suggest: { embedding: ["text-embedding-3-large", "text-embedding-3-small"] },
  },
  {
    id: "gemini",
    label: "Google Gemini",
    kind: "openai",
    base: "https://generativelanguage.googleapis.com/v1beta/openai",
    env: "GEMINI_API_KEY",
    keyUrl: "https://aistudio.google.com/apikey",
    types: ["chat", "embedding"],
    suggest: { embedding: ["gemini-embedding-001"] },
  },
  {
    id: "deepseek",
    label: "DeepSeek",
    kind: "openai",
    base: "https://api.deepseek.com/v1",
    env: "DEEPSEEK_API_KEY",
    keyUrl: "https://platform.deepseek.com/api_keys",
    types: ["chat"],
  },
  {
    id: "openrouter",
    label: "OpenRouter",
    kind: "openai",
    base: "https://openrouter.ai/api/v1",
    env: "OPENROUTER_API_KEY",
    keyUrl: "https://openrouter.ai/keys",
    // OpenRouter serves /embeddings and a Cohere-style /rerank under the same base.
    types: ["chat", "embedding", "rerank"],
    suggest: {
      embedding: ["openai/text-embedding-3-large", "openai/text-embedding-3-small"],
      rerank: ["cohere/rerank-v3.5"],
    },
  },
  {
    id: "voyage",
    label: "Voyage AI",
    kind: "voyage",
    base: "",
    env: "VOYAGE_API_KEY",
    keyUrl: "https://dashboard.voyageai.com/api-keys",
    types: ["embedding", "rerank"],
    suggest: { embedding: ["voyage-3-large", "voyage-law-2"], rerank: ["rerank-2", "rerank-2-lite"] },
  },
  {
    id: "jina",
    label: "Jina AI",
    kind: "jina",
    base: "",
    env: "JINA_API_KEY",
    keyUrl: "https://jina.ai/api-dashboard",
    types: ["embedding", "rerank"],
    suggest: { embedding: ["jina-embeddings-v3"], rerank: ["jina-reranker-v2-base-multilingual"] },
  },
  {
    id: "cohere",
    label: "Cohere",
    kind: "cohere",
    base: "",
    env: "COHERE_API_KEY",
    keyUrl: "https://dashboard.cohere.com/api-keys",
    types: ["rerank"],
    suggest: { rerank: ["rerank-v3.5", "rerank-multilingual-v3.0"] },
  },
  {
    id: "ollama",
    label: "Ollama",
    kind: "openai",
    base: "http://host.docker.internal:11434/v1",
    env: "",
    noKey: true,
    types: ["chat", "embedding"],
    suggest: { embedding: ["bge-m3", "nomic-embed-text"] },
  },
];

const CUSTOM = "__custom";
const ROLES = ["answer", "agent", "router", "rewrite"] as const;
const TABS = ["all", "chat", "embedding", "rerank", "vision", "speech"] as const;
type Tab = (typeof TABS)[number];

const presetOf = (name: string) => PRESETS.find((x) => x.id === name);
const label = (p: LLMProvider) => presetOf(p.name)?.label ?? p.name;
// A custom OpenAI-compatible endpoint, a local gateway or vLLM, often needs no
// key, so only a known provider is flagged for a missing one.
const configured = (p: LLMProvider) =>
  p.kind === "luatdo" ||
  !!p.has_key ||
  !!p.env_key_set ||
  !!presetOf(p.name)?.noKey ||
  (!presetOf(p.name) && p.kind === "openai");

/** Which model types a provider can serve, as the server checks it. */
function supports(kind: ProviderKind, type: ModelType, base: string) {
  switch (type) {
    case "chat":
      return kind === "anthropic" || kind === "openai" || kind === "luatdo";
    case "embedding":
      return kind === "openai" || kind === "voyage" || kind === "jina";
    case "rerank":
      return kind === "voyage" || kind === "jina" || kind === "cohere" || (kind === "openai" && base !== "");
  }
}

const typeIcon: Record<Tab, ReactNode> = {
  all: <Shapes />,
  chat: <MessageSquare />,
  embedding: <Boxes />,
  rerank: <ListOrdered />,
  vision: <Eye />,
  speech: <AudioLines />,
};

/** One model as the page lists it. */
interface Entry {
  type: ModelType;
  model: string;
  provider?: LLMProvider;
  dims?: number;
  /** Declared in customsvn.yaml, not in these settings. */
  fileName?: string;
}

function ModelsPage() {
  const { t } = useTranslation();
  const q = useQuery({ queryKey: ["settings", "llm"], queryFn: chatApi.llm });
  return (
    <div className="mx-auto max-w-5xl space-y-4 p-4 lg:p-6">
      {q.data ? <Editor initial={q.data} /> : <PageTitle title={t("models.title")} subtitle={t("models.subtitle")} />}
      <ErrorLine error={q.error} />
      {!q.data && !q.error && <Skeleton rows={6} />}
    </div>
  );
}

function normalize(s: LLMSettings): LLMSettings {
  return {
    ...s,
    providers: s.providers.map((p) => ({ ...p, embeddings: p.embeddings ?? [], rerankers: p.rerankers ?? [] })),
    roles: { answer: [], agent: [], rewrite: [], router: [], ...s.roles },
    prices: s.prices ?? {},
  };
}

/** Holds the settings; every change is saved at once, as in Dify. */
function Editor({ initial }: { initial: LLMSettings }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const meta = useQuery({ queryKey: ["meta"], queryFn: api.meta, staleTime: 60_000 });
  const [s, setS] = useState(() => normalize(initial));
  const [problems, setProblems] = useState<string[]>([]);
  const [notice, setNotice] = useState("");
  const [tab, setTab] = useState<Tab>("all");
  const [query, setQuery] = useState("");
  const [adding, setAdding] = useState<ModelType | null>(null);
  const [editing, setEditing] = useState<LLMProvider | null>(null);
  const save = useMutation({
    mutationFn: (next: LLMSettings) => chatApi.saveLLM(next),
    onSuccess: (r) => {
      setS(normalize(r.settings));
      setProblems(r.problems ?? []);
      void qc.invalidateQueries({ queryKey: ["meta"] });
      qc.setQueryData(["settings", "llm"], r.settings);
    },
  });
  const commit = (next: LLMSettings) => {
    setS(next);
    save.mutate(next);
  };
  const stripRoles = (keep: (x: LLMTarget) => boolean) =>
    Object.fromEntries(Object.entries(s.roles).map(([r, ts]) => [r, ts.filter(keep)]));

  // Every model in one list: the settings' own, then those of the config file.
  const entries = useMemo(() => {
    const out: Entry[] = [];
    const named = new Set<string>();
    for (const p of s.providers) {
      for (const m of p.models) out.push({ type: "chat", model: m, provider: p });
      for (const e of p.embeddings ?? []) {
        out.push({ type: "embedding", model: e.model, dims: e.dims, provider: p });
        named.add(p.name + "/" + e.model);
      }
      for (const m of p.rerankers ?? []) {
        out.push({ type: "rerank", model: m, provider: p });
        named.add(p.name + "/" + m);
      }
    }
    for (const n of meta.data?.embedding_models ?? [])
      if (!named.has(n)) out.push({ type: "embedding", model: n, fileName: n });
    for (const n of meta.data?.rerank_models ?? [])
      if (!named.has(n)) out.push({ type: "rerank", model: n, fileName: n });
    return out;
  }, [s.providers, meta.data]);
  const count = (tb: Tab) => (tb === "all" ? entries.length : entries.filter((e) => e.type === tb).length);
  const needle = query.trim().toLowerCase();
  const shown = entries.filter(
    (e) =>
      (tab === "all" || e.type === tab) &&
      (!needle ||
        e.model.toLowerCase().includes(needle) ||
        (e.provider && label(e.provider).toLowerCase().includes(needle))),
  );
  const unused = tab === "vision" || tab === "speech";

  const rolesOf = (e: Entry) =>
    e.type !== "chat" || !e.provider
      ? []
      : ROLES.flatMap((r) =>
          (s.roles[r] ?? []).flatMap((x, i) =>
            x.provider === e.provider!.name && x.model === e.model
              ? [
                  i === 0
                    ? t("llm2.role." + r + ".title")
                    : t("models.fallbackOf", { role: t("llm2.role." + r + ".title") }),
                ]
              : [],
          ),
        );

  const remove = (e: Entry) => {
    const p = e.provider!;
    const roles = rolesOf(e);
    const msg = roles.length
      ? t("models.confirmRemoveRoles", { m: e.model, roles: roles.join(", ") })
      : t("models.confirmRemove", { m: e.model });
    if (!window.confirm(msg)) return;
    const next: LLMProvider = {
      ...p,
      models: e.type === "chat" ? p.models.filter((m) => m !== e.model) : p.models,
      embeddings: e.type === "embedding" ? (p.embeddings ?? []).filter((m) => m.model !== e.model) : p.embeddings,
      rerankers: e.type === "rerank" ? (p.rerankers ?? []).filter((m) => m !== e.model) : p.rerankers,
    };
    commit({
      ...s,
      providers: s.providers.map((x) => (x.name === p.name ? next : x)),
      roles: e.type === "chat" ? stripRoles((x) => !(x.provider === p.name && x.model === e.model)) : s.roles,
    });
  };

  return (
    <>
      <PageTitle
        title={t("models.title")}
        subtitle={t("models.subtitle")}
        actions={
          <Button onClick={() => setAdding(tab === "embedding" || tab === "rerank" ? tab : "chat")}>
            <Plus /> {t("models.add")}
          </Button>
        }
      />
      {!s.master_key && (
        <p className="rounded-md border p-3 text-xs text-force-partly">{t("llmSettings.noMasterKey")}</p>
      )}
      <div className="flex min-h-5 flex-wrap items-center gap-2 text-xs" role="status">
        {save.isPending && (
          <span className="flex items-center gap-1 text-muted-foreground">
            <Loader2 className="size-3.5 animate-spin" /> {t("llm2.saving")}
          </span>
        )}
        {save.isSuccess && !save.isPending && (
          <span className="flex items-center gap-1 text-force-in">
            <CheckCircle2 className="size-3.5" /> {t("llmSettings.saved")}
          </span>
        )}
        {notice && <span className="text-muted-foreground">{notice}</span>}
      </div>
      <ErrorLine error={save.error} />
      {problems.length > 0 && (
        <div className="rounded-md border border-force-partly/40 p-3 text-xs">
          <div className="font-medium">{t("llmSettings.problems")}</div>
          <ul className="list-disc pl-4">
            {problems.map((p) => (
              <li key={p}>{p}</li>
            ))}
          </ul>
        </div>
      )}

      <div className="flex flex-wrap items-end justify-between gap-3 border-b">
        <div role="tablist" aria-label={t("models.sType")} className="-mb-px flex gap-1 overflow-x-auto">
          {TABS.map((tb) => (
            <button
              key={tb}
              type="button"
              role="tab"
              aria-selected={tab === tb}
              onClick={() => setTab(tb)}
              className={cn(
                "flex shrink-0 cursor-pointer items-center gap-1.5 border-b-2 border-transparent px-2.5 py-2 text-sm text-muted-foreground hover:text-foreground [&_svg]:size-4",
                tab === tb && "border-primary font-medium text-foreground",
              )}
            >
              {typeIcon[tb]}
              {t("models.types." + tb)}
              <span
                className={cn(
                  "rounded-full bg-muted px-1.5 text-2xs tabular-nums",
                  tab === tb && "bg-accent text-accent-foreground",
                )}
              >
                {count(tb)}
              </span>
            </button>
          ))}
        </div>
        <div className="relative mb-2 w-full sm:w-56">
          <Search className="pointer-events-none absolute top-2.5 left-2.5 size-4 text-muted-foreground" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("models.search")}
            aria-label={t("models.search")}
            className="pl-8"
          />
        </div>
      </div>

      {tab !== "all" && <p className="text-xs text-muted-foreground">{t("models.typeText." + tab)}</p>}
      {unused ? (
        <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">
          <span className="[&_svg]:size-6">{typeIcon[tab]}</span>
          {t("models.unusedType")}
        </div>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {shown.map((e) => (
            <ModelCard
              key={e.type + (e.provider?.name ?? "file") + e.model}
              e={e}
              roles={rolesOf(e)}
              onRemove={e.provider ? () => remove(e) : undefined}
              onFix={e.provider ? () => setEditing(e.provider!) : undefined}
            />
          ))}
          <button
            type="button"
            onClick={() => setAdding(tab === "embedding" || tab === "rerank" ? tab : "chat")}
            className="flex min-h-32 cursor-pointer flex-col items-center justify-center gap-2 rounded-lg border border-dashed text-sm text-muted-foreground hover:bg-surface-2 hover:text-foreground"
          >
            <span className="flex size-8 items-center justify-center rounded-md bg-muted">
              <Plus className="size-4" />
            </span>
            {t("models.add")}
          </button>
          {shown.length === 0 && (
            <p className="self-center text-sm text-muted-foreground sm:col-span-1 lg:col-span-2">
              {needle ? t("models.noMatch", { q: query }) : entries.length ? t("models.empty") : t("models.emptyAll")}
            </p>
          )}
        </div>
      )}

      <Card>
        <CardHeader>
          <div>
            <CardTitle>{t("models.rolesTitle")}</CardTitle>
            <p className="mt-0.5 text-xs text-muted-foreground">
              {t("models.rolesText")}{" "}
              <Link to="/settings/knowledge" className="font-medium text-primary hover:underline">
                {t("models.knowledgeLink")}
              </Link>
            </p>
          </div>
        </CardHeader>
        <CardContent className="divide-y p-0">
          {ROLES.map((role) => (
            <RoleRow
              key={role}
              role={role}
              targets={s.roles[role] ?? []}
              providers={s.providers}
              onChange={(ts) => commit({ ...s, roles: { ...s.roles, [role]: ts } })}
            />
          ))}
        </CardContent>
      </Card>

      {s.providers.length > 0 && (
        <Card>
          <CardHeader>
            <div>
              <CardTitle>{t("models.providers")}</CardTitle>
              <p className="mt-0.5 text-xs text-muted-foreground">{t("models.providersText")}</p>
            </div>
          </CardHeader>
          <CardContent className="divide-y p-0">
            {s.providers.map((p) => (
              <div key={p.name} className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
                <div className="min-w-0">
                  <div className="flex items-center gap-2 text-sm font-medium">
                    {label(p)}
                    {configured(p) ? (
                      <Badge variant="in_force">{t("llm2.connected")}</Badge>
                    ) : (
                      <Badge variant="partly_expired">{t("llm2.needsKey")}</Badge>
                    )}
                  </div>
                  <div className="text-2xs text-muted-foreground">
                    {p.has_key && t("llm2.keyHint", { hint: p.key_hint ?? "" })}
                    {!p.has_key && p.api_key_env && t("llm2.envHint", { env: p.api_key_env })}
                    {p.base_url && <span className="ml-2 font-mono">{p.base_url}</span>}
                  </div>
                </div>
                <Button size="sm" variant="outline" onClick={() => setEditing(p)}>
                  {t("models.editProvider")}
                </Button>
              </div>
            ))}
          </CardContent>
        </Card>
      )}

      <details className="group rounded-lg border bg-surface">
        <summary className="flex cursor-pointer list-none items-center gap-2 px-4 py-3 text-sm font-semibold">
          <ChevronRight className="size-4 transition-transform group-open:rotate-90" />
          {t("llm2.advanced")}
          <span className="text-xs font-normal text-muted-foreground">{t("llm2.advancedHint")}</span>
        </summary>
        <div className="border-t p-4">
          <PricesEditor key={JSON.stringify(s.prices)} s={s} onSave={(prices) => commit({ ...s, prices })} />
        </div>
      </details>

      {adding && (
        <AddModelDialog
          initialType={adding}
          s={s}
          onClose={() => setAdding(null)}
          onSave={(next, msg) => {
            commit(next);
            setNotice(msg);
            setAdding(null);
          }}
        />
      )}
      {editing && (
        <ProviderDialog
          key={editing.name}
          provider={editing}
          masterKey={s.master_key}
          onClose={() => setEditing(null)}
          onSave={(p) => {
            commit({ ...s, providers: s.providers.map((x) => (x.name === p.name ? p : x)) });
            setEditing(null);
          }}
          onRemove={() => {
            commit({
              ...s,
              providers: s.providers.filter((x) => x.name !== editing.name),
              roles: stripRoles((x) => x.provider !== editing.name),
            });
            setEditing(null);
          }}
        />
      )}
    </>
  );
}

function ModelCard({
  e,
  roles,
  onRemove,
  onFix,
}: {
  e: Entry;
  roles: string[];
  onRemove?: () => void;
  onFix?: () => void;
}) {
  const { t } = useTranslation();
  const ok = !e.provider || configured(e.provider);
  return (
    <div className="flex min-h-32 flex-col justify-between gap-3 rounded-lg border bg-surface p-3">
      <div className="space-y-1.5">
        <div className="flex items-start gap-2">
          <span className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-md bg-accent text-accent-foreground [&_svg]:size-4">
            {typeIcon[e.type]}
          </span>
          <div className="min-w-0 flex-1">
            <div className="truncate font-mono text-sm font-medium" title={e.model}>
              {e.model}
            </div>
            <div className="flex flex-wrap items-center gap-x-1.5 text-2xs text-muted-foreground">
              {e.provider ? (
                label(e.provider)
              ) : (
                <span className="inline-flex items-center gap-1" title={t("models.fromFileText")}>
                  <FileCode2 className="size-3" /> {t("models.fromFile")}
                </span>
              )}
              {e.dims ? <span>· {t("models.dims", { n: e.dims.toLocaleString() })}</span> : null}
            </div>
          </div>
          <Badge variant="outline">{t("models.types." + e.type)}</Badge>
        </div>
        {roles.length > 0 && (
          <div className="flex flex-wrap gap-1">
            {roles.map((r) => (
              <Badge key={r} variant="accent">
                {r}
              </Badge>
            ))}
          </div>
        )}
        {!ok && (
          <button type="button" onClick={onFix} className="cursor-pointer">
            <Badge variant="partly_expired">{t("llm2.needsKey")}</Badge>
          </button>
        )}
      </div>
      {e.provider && (
        <div className="flex items-center justify-between gap-2">
          <TestButton provider={e.provider.name} type={e.type} model={e.model} />
          <Button
            variant="ghost"
            size="icon"
            className="text-muted-foreground hover:text-destructive"
            aria-label={t("models.remove", { m: e.model })}
            title={t("models.remove", { m: e.model })}
            onClick={onRemove}
          >
            <Trash2 />
          </Button>
        </div>
      )}
    </div>
  );
}

/** What a test found, in a sentence. */
function testLine(t: (k: string, o?: Record<string, unknown>) => string, type: ModelType, r: LLMTest) {
  if (!r.ok) return t("models.testFail", { msg: r.error ?? "" });
  if (type === "embedding") return t("models.testOkEmbedding", { ms: r.latency_ms, dims: r.dims ?? 0 });
  if (type === "rerank")
    return t("models.testOkRerank", {
      ms: r.latency_ms,
      a: (r.scores?.[0] ?? 0).toFixed(2),
      b: (r.scores?.[1] ?? 0).toFixed(2),
    });
  return t("models.testOkChat", { ms: r.latency_ms, text: r.text ?? "" });
}

function TestButton({ provider, type, model }: { provider: string; type: ModelType; model: string }) {
  const { t } = useTranslation();
  const [r, setR] = useState<LLMTest | null>(null);
  const test = useMutation({ mutationFn: () => chatApi.testModel({ name: provider }, type, model), onSuccess: setR });
  return (
    <span className="inline-flex min-w-0 items-center gap-1.5">
      <Button variant="outline" size="sm" disabled={test.isPending} onClick={() => test.mutate()}>
        {test.isPending ? <Loader2 className="animate-spin" /> : <Zap />}
        {t("llm2.test")}
      </Button>
      {r &&
        (r.ok ? (
          <CheckCircle2 className="size-4 shrink-0 text-force-in" aria-label={testLine(t, type, r)}>
            <title>{testLine(t, type, r)}</title>
          </CheckCircle2>
        ) : (
          <span className="truncate text-2xs text-destructive" title={r.error}>
            {r.error}
          </span>
        ))}
    </span>
  );
}

/**
 * Adding a model, as Dify and WeKnora do it: the type, the source, the
 * provider and its connection, the model, and a connection test before
 * anything is saved.
 */
function AddModelDialog({
  initialType,
  s,
  onClose,
  onSave,
}: {
  initialType: ModelType;
  s: LLMSettings;
  onClose: () => void;
  onSave: (next: LLMSettings, notice: string) => void;
}) {
  const { t } = useTranslation();
  const [type, setType] = useState<ModelType>(initialType);
  const [source, setSource] = useState<"api" | "local">("api");
  const [providerId, setProviderId] = useState("");
  const [customName, setCustomName] = useState("");
  const [baseUrl, setBaseUrl] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [changeKey, setChangeKey] = useState(false);
  const [model, setModel] = useState("");
  const [dims, setDims] = useState("");
  const [result, setResult] = useState<LLMTest | null>(null);

  // The providers that serve this type: presets, then the custom ones saved.
  const options = useMemo(() => {
    const out: { id: string; label: string }[] = [];
    for (const p of PRESETS) if (p.id !== "ollama" && p.types.includes(type)) out.push({ id: p.id, label: p.label });
    for (const p of s.providers)
      if (!presetOf(p.name) && supports(p.kind, type, p.base_url)) out.push({ id: p.name, label: p.name });
    return out;
  }, [type, s.providers]);
  const localOK = type !== "rerank";
  const id = source === "local" ? "ollama" : providerId || options[0]?.id || CUSTOM;
  const preset = presetOf(id);
  const existing = s.providers.find((p) => p.name === (id === CUSTOM ? customName : id));
  const custom = id === CUSTOM;
  const name = custom ? customName.trim() : id;
  const kind: ProviderKind = existing?.kind ?? preset?.kind ?? "openai";
  const showBase = custom || id === "ollama" || (!!existing && !preset);
  const base = showBase
    ? baseUrl || existing?.base_url || preset?.base || ""
    : (existing?.base_url ?? preset?.base ?? "");
  const needsKey = !preset?.noKey && kind !== "luatdo";
  const askKey = needsKey && (!existing || !configured(existing) || changeKey);
  const nameTaken = custom && !!existing;

  const draft: LLMProvider = {
    ...(existing ?? { name, kind, api_key_env: "", models: [], embeddings: [], rerankers: [] }),
    name,
    base_url: base,
    ...(apiKey.trim() ? { api_key: apiKey.trim() } : {}),
  };
  const reset = () => setResult(null);
  const test = useMutation({
    mutationFn: () => chatApi.testModel(draft, type, model.trim()),
    onSuccess: (r) => {
      setResult(r);
      if (r.ok && type === "embedding" && r.dims) setDims(String(r.dims));
    },
  });
  const fetchList = useMutation({
    mutationFn: () => request<{ models: string[]; error?: string }>("POST", "/api/settings/llm/models", draft),
  });
  const listed = (fetchList.data?.models ?? []).filter((m) =>
    type === "embedding" ? /embed/i.test(m) : type === "rerank" ? /rerank/i.test(m) : !/embed|rerank/i.test(m),
  );
  const suggestions = [...new Set([...(preset?.suggest?.[type] ?? []), ...listed])].slice(0, 40);
  const canList = kind === "anthropic" || kind === "openai";

  const m = model.trim();
  const has =
    !!existing &&
    (type === "chat"
      ? existing.models.includes(m)
      : type === "embedding"
        ? (existing.embeddings ?? []).some((x) => x.model === m)
        : (existing.rerankers ?? []).includes(m));
  const dimsN = Number(dims);
  const valid =
    name !== "" &&
    !/[\s/]/.test(name) &&
    m !== "" &&
    !has &&
    (!custom || !nameTaken) &&
    (!showBase || base !== "" || !custom) &&
    supports(kind, type, base) &&
    (type !== "embedding" || (Number.isInteger(dimsN) && dimsN > 0 && dimsN <= 4000));

  const submit = () => {
    const p: LLMProvider = { ...draft, embeddings: draft.embeddings ?? [], rerankers: draft.rerankers ?? [] };
    if (!existing && !preset?.noKey && !p.api_key && preset?.env && !s.master_key) p.api_key_env = preset.env;
    if (type === "chat") p.models = [...p.models, m];
    if (type === "embedding") p.embeddings = [...(p.embeddings ?? []), { model: m, dims: dimsN }];
    if (type === "rerank") p.rerankers = [...(p.rerankers ?? []), m];
    const providers = existing ? s.providers.map((x) => (x.name === existing.name ? p : x)) : [...s.providers, p];
    let roles = s.roles;
    let notice = "";
    if (type === "chat" && !(s.roles.answer ?? []).length) {
      roles = { ...s.roles, answer: [{ provider: p.name, model: m }] };
      notice = t("models.madeAnswer", { m });
    }
    if (type === "embedding") notice = t("models.embedNext", { m });
    onSave({ ...s, providers, roles }, notice);
  };

  const chip = (on: boolean, disabled?: boolean) =>
    cn(
      "inline-flex cursor-pointer items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm [&_svg]:size-4",
      on ? "border-primary bg-accent/40 font-medium text-accent-foreground" : "hover:bg-surface-2",
      disabled && "cursor-not-allowed opacity-50 hover:bg-transparent",
    );
  const section = (title: string) => (
    <div className="border-l-2 border-primary pl-2 text-sm font-semibold">{title}</div>
  );

  return (
    <Dialog
      title={t("models.add")}
      description={t("models.addText")}
      onClose={onClose}
      footer={
        <div className="flex w-full flex-wrap items-center gap-2">
          <Button variant="outline" onClick={() => test.mutate()} disabled={test.isPending || !m || !name}>
            {test.isPending ? <Loader2 className="animate-spin" /> : <Zap />}
            {t("models.testConn")}
          </Button>
          <span className="mr-auto hidden text-2xs text-muted-foreground sm:inline">{t("models.testText")}</span>
          <Button variant="outline" onClick={onClose}>
            {t("llm2.cancel")}
          </Button>
          <Button onClick={submit} disabled={!valid}>
            <Save /> {t("models.saveClose")}
          </Button>
        </div>
      }
    >
      <div className="space-y-5">
        <div className="space-y-2">
          {section(t("models.sType"))}
          <div className="flex flex-wrap gap-2">
            {(["chat", "embedding", "rerank", "vision", "speech"] as const).map((tp) => {
              const off = tp === "vision" || tp === "speech";
              return (
                <button
                  key={tp}
                  type="button"
                  disabled={off}
                  aria-pressed={type === tp}
                  title={off ? t("models.unusedType") : t("models.typeText." + tp)}
                  className={chip(type === tp, off)}
                  onClick={() => {
                    if (off) return;
                    setType(tp);
                    setProviderId("");
                    if (tp === "rerank") setSource("api");
                    reset();
                  }}
                >
                  {typeIcon[tp]}
                  {t("models.types." + tp)}
                  {off && <span className="text-2xs">· {t("models.soon")}</span>}
                </button>
              );
            })}
          </div>
          <p className="text-xs text-muted-foreground">{t("models.typeText." + type)}</p>
        </div>

        <div className="space-y-2">
          {section(t("models.sSource"))}
          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              aria-pressed={source === "api"}
              className={chip(source === "api")}
              onClick={() => setSource("api")}
            >
              <Cloud /> {t("models.sourceApi")}
            </button>
            <button
              type="button"
              disabled={!localOK}
              aria-pressed={source === "local"}
              className={chip(source === "local", !localOK)}
              onClick={() => {
                setSource("local");
                reset();
              }}
            >
              <HardDrive /> {t("models.sourceLocal")}
            </button>
          </div>
          {source === "local" && <p className="text-xs text-muted-foreground">{t("models.sourceLocalText")}</p>}
        </div>

        <div className="space-y-3">
          {section(t("models.sProvider"))}
          {source === "api" && (
            <Field label={t("models.provider")}>
              <Select
                value={id}
                onChange={(e) => {
                  setProviderId(e.target.value);
                  setChangeKey(false);
                  setApiKey("");
                  fetchList.reset();
                  reset();
                }}
              >
                {options.map((o) => (
                  <option key={o.id} value={o.id}>
                    {o.label}
                    {s.providers.some((p) => p.name === o.id) ? " ✓" : ""}
                  </option>
                ))}
                <option value={CUSTOM}>{t("models.otherProvider")}</option>
              </Select>
            </Field>
          )}
          {custom && (
            <Field label={t("models.providerId")}>
              <Input
                value={customName}
                placeholder="my-gateway"
                onChange={(e) => setCustomName(e.target.value.replace(/[\s/]/g, ""))}
              />
              {nameTaken && <span className="text-2xs text-destructive">{t("llm2.nameTaken")}</span>}
            </Field>
          )}
          {existing && configured(existing) && !changeKey && needsKey && (
            <p className="flex flex-wrap items-center gap-2 text-xs">
              <CheckCircle2 className="size-4 text-force-in" />
              {t("models.connected")} ·{" "}
              {existing.has_key
                ? t("llm2.keyHint", { hint: existing.key_hint ?? "" })
                : existing.api_key_env && t("llm2.envHint", { env: existing.api_key_env })}
              <button
                type="button"
                className="cursor-pointer text-primary hover:underline"
                onClick={() => setChangeKey(true)}
              >
                {t("models.changeKey")}
              </button>
            </p>
          )}
          {askKey && (
            <Field label={t("llm2.apiKey")}>
              <Input
                type="password"
                autoComplete="off"
                disabled={!s.master_key}
                value={apiKey}
                placeholder="sk-…"
                onChange={(e) => {
                  setApiKey(e.target.value);
                  reset();
                }}
              />
              <span className="flex flex-wrap items-center gap-x-3 text-2xs font-normal text-muted-foreground">
                {s.master_key ? t("llm2.keySafe") : t("llm2.keyNoMaster")}
                {preset?.keyUrl && (
                  <a
                    href={preset.keyUrl}
                    target="_blank"
                    rel="noreferrer"
                    className="inline-flex items-center gap-0.5 text-primary hover:underline"
                  >
                    {t("llm2.getKey")} <ExternalLink className="size-3" />
                  </a>
                )}
              </span>
            </Field>
          )}
          {showBase && (
            <Field label={t("llm2.baseUrl")}>
              <Input
                value={baseUrl || existing?.base_url || (custom ? "" : (preset?.base ?? ""))}
                placeholder="https://…/v1"
                onChange={(e) => {
                  setBaseUrl(e.target.value);
                  reset();
                }}
              />
              {type === "rerank" && (
                <span className="text-2xs font-normal text-muted-foreground">{t("models.rerankBase")}</span>
              )}
            </Field>
          )}
        </div>

        <div className="space-y-3">
          {section(t("models.modelName"))}
          <div className="flex gap-2">
            <Input
              value={model}
              aria-label={t("models.modelName")}
              placeholder={preset?.suggest?.[type]?.[0] ?? (type === "chat" ? "claude-sonnet-5-5" : "")}
              onChange={(e) => {
                setModel(e.target.value);
                reset();
              }}
            />
            {canList && (
              <Button variant="outline" onClick={() => fetchList.mutate()} disabled={fetchList.isPending || !name}>
                {fetchList.isPending ? <Loader2 className="animate-spin" /> : <RefreshCw />}
                <span className="hidden sm:inline">{t("llm2.fetch")}</span>
              </Button>
            )}
          </div>
          {has && <p className="text-xs text-destructive">{t("models.exists")}</p>}
          {fetchList.data?.error && (
            <p className="flex items-start gap-1 text-xs text-destructive">
              <CircleAlert className="mt-0.5 size-3.5 shrink-0" />{" "}
              {t("llm2.fetchFailed", { msg: fetchList.data.error })}
            </p>
          )}
          {suggestions.length > 0 && (
            <div className="flex max-h-32 flex-wrap items-center gap-1.5 overflow-y-auto">
              <span className="text-2xs text-muted-foreground">{t("models.suggestions")}</span>
              {suggestions.map((x) => (
                <button
                  key={x}
                  type="button"
                  onClick={() => {
                    setModel(x);
                    reset();
                  }}
                  className={cn(
                    "cursor-pointer rounded-md border px-2 py-0.5 font-mono text-2xs hover:bg-surface-2",
                    x === m && "border-primary bg-accent/40",
                  )}
                >
                  {x}
                </button>
              ))}
            </div>
          )}
          {type === "embedding" && (
            <Field label={t("models.dimsLabel")}>
              <Input
                type="number"
                min={1}
                max={4000}
                value={dims}
                onChange={(e) => setDims(e.target.value)}
                className="w-40"
              />
              <span className="text-2xs font-normal text-muted-foreground">{t("models.dimsText")}</span>
            </Field>
          )}
        </div>

        {result && (
          <p
            role="status"
            className={cn(
              "flex items-start gap-1.5 rounded-md border p-2.5 text-xs",
              result.ok ? "border-force-in/40 text-force-in" : "border-destructive/40 text-destructive",
            )}
          >
            {result.ok ? <CheckCircle2 className="size-4 shrink-0" /> : <CircleAlert className="size-4 shrink-0" />}
            {testLine(t, type, result)}
          </p>
        )}
        <ErrorLine error={test.error} />
      </div>
    </Dialog>
  );
}

/** A provider's connection: key, address, environment variable; or removing it. */
function ProviderDialog({
  provider,
  masterKey,
  onClose,
  onSave,
  onRemove,
}: {
  provider: LLMProvider;
  masterKey: boolean;
  onClose: () => void;
  onSave: (p: LLMProvider) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const preset = presetOf(provider.name);
  const [p, setP] = useState<LLMProvider>(provider);
  const needsKey = p.kind !== "luatdo" && !preset?.noKey;
  return (
    <Dialog
      title={t("models.providerDialog", { name: label(provider) })}
      description={t("models.providerDialogText")}
      onClose={onClose}
      footer={
        <>
          <Button
            variant="ghost"
            className="mr-auto text-destructive"
            onClick={() => {
              if (window.confirm(t("llm2.confirmRemove", { name: label(provider) }))) onRemove();
            }}
          >
            <Trash2 /> {t("llm2.remove")}
          </Button>
          <Button variant="outline" onClick={onClose}>
            {t("llm2.cancel")}
          </Button>
          <Button onClick={() => onSave(p)}>
            <Save /> {t("llm2.save")}
          </Button>
        </>
      }
    >
      {needsKey && (
        <Field label={t("llm2.apiKey")}>
          <Input
            type="password"
            autoComplete="off"
            disabled={!masterKey}
            value={p.api_key ?? ""}
            placeholder={provider.has_key ? t("llm2.keyKeep", { hint: provider.key_hint ?? "" }) : "sk-…"}
            onChange={(e) => setP({ ...p, api_key: e.target.value, clear_key: false })}
          />
          <span className="flex flex-wrap items-center gap-x-3 text-2xs font-normal text-muted-foreground">
            {masterKey ? t("llm2.keySafe") : t("llm2.keyNoMaster")}
            {preset?.keyUrl && (
              <a
                href={preset.keyUrl}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-0.5 text-primary hover:underline"
              >
                {t("llm2.getKey")} <ExternalLink className="size-3" />
              </a>
            )}
          </span>
        </Field>
      )}
      <Field label={p.kind === "luatdo" ? t("llmSettings.baseUrlLuatdo") : t("llm2.baseUrl")}>
        <Input
          value={p.base_url}
          placeholder={preset?.base || (p.kind === "anthropic" ? "https://api.anthropic.com" : "https://…/v1")}
          onChange={(e) => setP({ ...p, base_url: e.target.value })}
        />
      </Field>
      {needsKey && (
        <Field label={t("llm2.envVar")}>
          <Input
            value={p.api_key_env}
            placeholder={preset?.env || "MY_API_KEY"}
            onChange={(e) => setP({ ...p, api_key_env: e.target.value.trim() })}
          />
          <span className="text-2xs font-normal text-muted-foreground">{t("llm2.envVarText")}</span>
        </Field>
      )}
      {provider.has_key && (
        <label className="flex items-center gap-2 text-xs">
          <input
            type="checkbox"
            className="size-3.5 accent-[var(--primary)]"
            checked={!!p.clear_key}
            onChange={(e) => setP({ ...p, clear_key: e.target.checked, api_key: "" })}
          />
          {t("llmSettings.clearKey")}
        </label>
      )}
    </Dialog>
  );
}

function targetKey(x: LLMTarget) {
  return x.provider + "/" + x.model;
}

/** One role: its main model and an optional fallback, picked from the chat models. */
function RoleRow({
  role,
  targets,
  providers,
  onChange,
}: {
  role: (typeof ROLES)[number];
  targets: LLMTarget[];
  providers: LLMProvider[];
  onChange: (ts: LLMTarget[]) => void;
}) {
  const { t } = useTranslation();
  const options = providers.flatMap((p) => p.models.map((m) => ({ provider: p.name, model: m })));
  const [showFallback, setShowFallback] = useState(targets.length > 1);
  const pick = (i: number, key: string) => {
    const next = [...targets];
    const opt = options.find((o) => targetKey(o) === key);
    if (!opt) next.splice(i, i === 0 ? next.length : 1);
    else next[i] = opt;
    onChange(next.filter(Boolean));
  };
  const select = (i: number, ariaLabel: string) => (
    <Select
      aria-label={ariaLabel}
      value={targets[i] ? targetKey(targets[i]) : ""}
      onChange={(e) => pick(i, e.target.value)}
      className="min-w-0 flex-1"
    >
      <option value="">{i === 0 ? t(role === "answer" ? "llm2.choose" : "llm2.none") : t("llm2.noFallback")}</option>
      {providers.map((p) =>
        p.models.length ? (
          <optgroup key={p.name} label={label(p)}>
            {p.models.map((m) => (
              <option key={m} value={p.name + "/" + m}>
                {m}
              </option>
            ))}
          </optgroup>
        ) : null,
      )}
      {/* A stored target whose model was removed from the provider. */}
      {targets[i] && !options.some((o) => targetKey(o) === targetKey(targets[i])) && (
        <option value={targetKey(targets[i])}>{targetKey(targets[i])}</option>
      )}
    </Select>
  );
  const missing = role === "answer" && targets.length === 0;
  return (
    <div className="grid gap-3 p-4 md:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
      <div className="space-y-1">
        <div className="flex items-center gap-2 text-sm font-medium">
          {t("llm2.role." + role + ".title")}
          {role === "answer" ? (
            <Badge variant={missing ? "expired" : "accent"}>{t("llm2.required")}</Badge>
          ) : (
            <Badge variant="outline">{t("llm2.optional")}</Badge>
          )}
        </div>
        <p className="text-xs leading-relaxed text-muted-foreground">{t("llm2.role." + role + ".text")}</p>
      </div>
      <div className="space-y-2">
        {options.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t("llm2.addProviderFirst")}</p>
        ) : (
          <>
            <div className="flex items-center gap-2">
              {select(0, t("llm2.role." + role + ".title"))}
              {targets[0] && <TestButton provider={targets[0].provider} type="chat" model={targets[0].model} />}
            </div>
            {targets[0] &&
              (showFallback ? (
                <div className="space-y-1">
                  <span className="text-2xs text-muted-foreground">{t("llm2.fallback")}</span>
                  <div className="flex items-center gap-2">{select(1, t("llm2.fallback"))}</div>
                </div>
              ) : (
                <button
                  type="button"
                  className="cursor-pointer text-xs text-primary hover:underline"
                  onClick={() => setShowFallback(true)}
                >
                  + {t("llm2.addFallback")}
                </button>
              ))}
          </>
        )}
      </div>
    </div>
  );
}

/** Prices per chat model, so usage can be costed on the dashboard. */
function PricesEditor({ s, onSave }: { s: LLMSettings; onSave: (p: LLMSettings["prices"]) => void }) {
  const { t } = useTranslation();
  const [prices, setPrices] = useState(s.prices);
  const keys = s.providers.flatMap((p) => p.models.map((m) => p.name + "/" + m));
  const set = (key: string, side: "input" | "output", v: number) =>
    setPrices({ ...prices, [key]: { ...{ input: 0, output: 0 }, ...prices[key], [side]: v } });
  return (
    <div className="space-y-3">
      <div>
        <div className="text-sm font-medium">{t("llmSettings.prices")}</div>
        <p className="text-xs text-muted-foreground">{t("llmSettings.pricesHint")}</p>
      </div>
      {keys.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t("llmSettings.noModels")}</p>
      ) : (
        <div className="space-y-2">
          {keys.map((k) => (
            <div key={k} className="grid grid-cols-[minmax(0,1fr)_7rem_7rem] items-end gap-2">
              <span className="truncate pb-2 font-mono text-xs" title={k}>
                {k}
              </span>
              <Field label={t("llmSettings.priceIn")}>
                <Input
                  type="number"
                  min={0}
                  step="0.01"
                  value={prices[k]?.input ?? ""}
                  onChange={(e) => set(k, "input", Number(e.target.value))}
                />
              </Field>
              <Field label={t("llmSettings.priceOut")}>
                <Input
                  type="number"
                  min={0}
                  step="0.01"
                  value={prices[k]?.output ?? ""}
                  onChange={(e) => set(k, "output", Number(e.target.value))}
                />
              </Field>
            </div>
          ))}
          <Button size="sm" onClick={() => onSave(prices)}>
            <Save /> {t("llm2.savePrices")}
          </Button>
        </div>
      )}
    </div>
  );
}
