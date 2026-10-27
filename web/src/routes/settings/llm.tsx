import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { Plus, Save, Trash2, Zap } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox, Field, Input, Select } from "@/components/ui/form";
import { chatApi, type LLMProvider, type LLMSettings, type LLMTarget, type LLMTest } from "@/lib/api/chat";

export const Route = createFileRoute("/settings/llm")({ component: LLMSettingsPage });

const roles = ["answer", "rewrite"] as const;

function LLMSettingsPage() {
  const { t } = useTranslation();
  const q = useQuery({ queryKey: ["settings", "llm"], queryFn: chatApi.llm });
  return (
    <div className="mx-auto max-w-4xl space-y-4 p-4 lg:p-6">
      <div>
        <h1 className="text-xl font-semibold">{t("llmSettings.title")}</h1>
        <p className="text-xs text-muted-foreground">{t("llmSettings.subtitle")}</p>
      </div>
      {q.error && <p className="text-destructive">{t("common.error", { msg: q.error.message })}</p>}
      {q.data && <Form initial={q.data} />}
    </div>
  );
}

function Form({ initial }: { initial: LLMSettings }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [s, setS] = useState<LLMSettings>({ ...initial, roles: { answer: [], rewrite: [], ...initial.roles } });
  const [problems, setProblems] = useState<string[]>([]);
  const save = useMutation({
    mutationFn: () => chatApi.saveLLM(s),
    onSuccess: (r) => {
      setS({ ...r.settings, roles: { answer: [], rewrite: [], ...r.settings.roles } });
      setProblems(r.problems ?? []);
      void qc.invalidateQueries({ queryKey: ["meta"] });
      void qc.invalidateQueries({ queryKey: ["settings", "llm"] });
    },
  });
  const setProvider = (i: number, p: LLMProvider) =>
    setS({ ...s, providers: s.providers.map((x, j) => (j === i ? p : x)) });
  const names = s.providers.map((p) => p.name).filter(Boolean);

  return (
    <>
      {!s.master_key && (
        <p className="rounded-md border p-3 text-xs text-force-partly">{t("llmSettings.noMasterKey")}</p>
      )}
      <Card>
        <CardHeader>
          <CardTitle>{t("llmSettings.providers")}</CardTitle>
          <Button
            variant="outline"
            size="sm"
            onClick={() =>
              setS({
                ...s,
                providers: [...s.providers, { name: "", kind: "anthropic", base_url: "", api_key_env: "", models: [] }],
              })
            }
          >
            <Plus /> {t("llmSettings.addProvider")}
          </Button>
        </CardHeader>
        <CardContent className="space-y-4">
          {s.providers.map((p, i) => (
            <ProviderEditor
              key={i}
              p={p}
              masterKey={s.master_key}
              onChange={(np) => setProvider(i, np)}
              onRemove={() => setS({ ...s, providers: s.providers.filter((_, j) => j !== i) })}
            />
          ))}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t("llmSettings.roles")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-xs text-muted-foreground">{t("llmSettings.roleHint")}</p>
          {roles.map((role) => (
            <RoleEditor
              key={role}
              role={role}
              targets={s.roles[role] ?? []}
              providers={s.providers}
              names={names}
              onChange={(ts) => setS({ ...s, roles: { ...s.roles, [role]: ts } })}
            />
          ))}
        </CardContent>
      </Card>
      <div className="flex flex-wrap items-center gap-2">
        <Button onClick={() => save.mutate()} disabled={save.isPending}>
          <Save /> {t("llmSettings.save")}
        </Button>
        {save.isSuccess && <span className="text-xs text-force-in">{t("llmSettings.saved")}</span>}
        {save.error && (
          <span className="text-xs text-destructive">{t("common.error", { msg: save.error.message })}</span>
        )}
      </div>
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
    </>
  );
}

function ProviderEditor({
  p,
  masterKey,
  onChange,
  onRemove,
}: {
  p: LLMProvider;
  masterKey: boolean;
  onChange: (p: LLMProvider) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const [models, setModels] = useState(p.models.join(", "));
  return (
    <div className="space-y-3 rounded-md border p-3">
      <div className="grid gap-3 sm:grid-cols-3">
        <Field label={t("llmSettings.name")}>
          <Input value={p.name} onChange={(e) => onChange({ ...p, name: e.target.value.trim() })} />
        </Field>
        <Field label={t("llmSettings.kind")}>
          <Select value={p.kind} onChange={(e) => onChange({ ...p, kind: e.target.value as LLMProvider["kind"] })}>
            {(["anthropic", "openai", "luatdo"] as const).map((k) => (
              <option key={k} value={k}>
                {t("llmSettings.kinds." + k)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label={p.kind === "luatdo" ? t("llmSettings.baseUrlLuatdo") : t("llmSettings.baseUrl")}>
          <Input
            value={p.base_url}
            placeholder={
              p.kind === "anthropic"
                ? "https://api.anthropic.com"
                : p.kind === "openai"
                  ? "https://api.openai.com/v1"
                  : ""
            }
            onChange={(e) => onChange({ ...p, base_url: e.target.value })}
          />
        </Field>
      </div>
      {p.kind !== "luatdo" && (
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label={t("llmSettings.apiKey")}>
            <Input
              type="password"
              autoComplete="off"
              disabled={!masterKey}
              value={p.api_key ?? ""}
              placeholder={p.has_key ? t("llmSettings.keyStored", { hint: p.key_hint ?? "" }) : t("llmSettings.noKey")}
              onChange={(e) => onChange({ ...p, api_key: e.target.value, clear_key: false })}
            />
          </Field>
          <Field label={t("llmSettings.apiKeyEnv")}>
            <Input
              value={p.api_key_env}
              placeholder="ANTHROPIC_API_KEY"
              onChange={(e) => onChange({ ...p, api_key_env: e.target.value.trim() })}
            />
          </Field>
          <div className="flex flex-wrap items-center gap-2 text-2xs sm:col-span-2">
            {p.has_key && <Badge variant="in_force">{t("llmSettings.keyStored", { hint: p.key_hint ?? "" })}</Badge>}
            {!p.has_key && p.api_key_env && (
              <Badge variant={p.env_key_set ? "in_force" : "expired"}>
                {p.env_key_set
                  ? t("llmSettings.keyFromEnv", { env: p.api_key_env })
                  : t("llmSettings.envMissing", { env: p.api_key_env })}
              </Badge>
            )}
            {p.has_key && (
              <Checkbox
                label={t("llmSettings.clearKey")}
                checked={!!p.clear_key}
                onChange={(e) => onChange({ ...p, clear_key: e.target.checked, api_key: "" })}
              />
            )}
          </div>
        </div>
      )}
      <div className="flex flex-wrap items-end gap-3">
        <Field label={t("llmSettings.models")} className="min-w-64 flex-1">
          <Input
            value={models}
            placeholder="claude-sonnet-5-5, claude-haiku-5-5"
            onChange={(e) => setModels(e.target.value)}
            onBlur={() =>
              onChange({
                ...p,
                models: models
                  .split(",")
                  .map((m) => m.trim())
                  .filter(Boolean),
              })
            }
          />
        </Field>
        <Button variant="ghost" onClick={onRemove}>
          <Trash2 /> {t("llmSettings.remove")}
        </Button>
      </div>
    </div>
  );
}

function RoleEditor({
  role,
  targets,
  providers,
  names,
  onChange,
}: {
  role: string;
  targets: LLMTarget[];
  providers: LLMProvider[];
  names: string[];
  onChange: (t: LLMTarget[]) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <div className="text-sm font-medium">{t("llmSettings.role." + role)}</div>
      {targets.map((tg, i) => {
        const models = providers.find((p) => p.name === tg.provider)?.models ?? [];
        return (
          <div key={i} className="flex flex-wrap items-center gap-2">
            <span className="w-5 text-right font-mono text-xs text-muted-foreground">{i + 1}.</span>
            <Select
              className="w-44"
              value={tg.provider}
              onChange={(e) => onChange(targets.map((x, j) => (j === i ? { ...x, provider: e.target.value } : x)))}
            >
              <option value="" />
              {names.map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </Select>
            <Input
              className="w-64"
              list={"models-" + role + i}
              value={tg.model}
              onChange={(e) => onChange(targets.map((x, j) => (j === i ? { ...x, model: e.target.value } : x)))}
            />
            <datalist id={"models-" + role + i}>
              {models.map((m) => (
                <option key={m} value={m} />
              ))}
            </datalist>
            <TestButton provider={tg.provider} model={tg.model} />
            <Button
              variant="ghost"
              size="icon"
              aria-label={t("llmSettings.remove")}
              onClick={() => onChange(targets.filter((_, j) => j !== i))}
            >
              <Trash2 />
            </Button>
          </div>
        );
      })}
      <Button
        variant="outline"
        size="sm"
        onClick={() => onChange([...targets, { provider: names[0] ?? "", model: "" }])}
      >
        <Plus /> {t("llmSettings.addTarget")}
      </Button>
    </div>
  );
}

function TestButton({ provider, model }: { provider: string; model: string }) {
  const { t } = useTranslation();
  const [r, setR] = useState<LLMTest | null>(null);
  const test = useMutation({ mutationFn: () => chatApi.testLLM(provider, model), onSuccess: setR });
  return (
    <span className="inline-flex items-center gap-2">
      <Button
        variant="outline"
        size="sm"
        disabled={!provider || !model || test.isPending}
        onClick={() => test.mutate()}
      >
        <Zap /> {t("llmSettings.test")}
      </Button>
      {r && (
        <span className={r.ok ? "text-2xs text-force-in" : "text-2xs text-destructive"}>
          {r.ok
            ? t("llmSettings.testOk", { ms: r.latency_ms, model: r.model })
            : (r.kind ? r.kind + ": " : "") + r.error}
        </span>
      )}
    </span>
  );
}
