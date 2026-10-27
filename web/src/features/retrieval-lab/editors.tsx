import { useTranslation } from "react-i18next";
import { Checkbox, Field, Input, Select } from "@/components/ui/form";
import type { Filters, Mode, SearchConfig } from "@/lib/api/types";

const sourceKinds = ["consolidated", "original", "rescued"];

export function FiltersEditor({ value, onChange }: { value: Filters; onChange: (f: Filters) => void }) {
  const { t } = useTranslation();
  const toggle = <T,>(list: T[] | undefined, v: T, all: T[]) => {
    const cur = list && list.length ? list : all;
    const next = cur.includes(v) ? cur.filter((x) => x !== v) : [...cur, v];
    return next.length === all.length ? [] : next;
  };
  const tiers = value.tiers && value.tiers.length ? value.tiers : [1, 2];
  const kinds = value.source_kinds && value.source_kinds.length ? value.source_kinds : sourceKinds;
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <Field label={t("lab.asOf")}>
        <Input type="date" value={value.as_of ?? ""} onChange={(e) => onChange({ ...value, as_of: e.target.value })} />
      </Field>
      <Field label={t("lab.tiers")}>
        <div className="flex h-9 items-center gap-4">
          {[1, 2].map((n) => (
            <Checkbox
              key={n}
              label={t("tier." + n)}
              checked={tiers.includes(n)}
              onChange={() => {
                const next = tiers.includes(n) ? tiers.filter((x) => x !== n) : [...tiers, n];
                onChange({ ...value, tiers: next.length ? next : [n === 1 ? 2 : 1] });
              }}
            />
          ))}
        </div>
      </Field>
      <Field label={t("lab.sourceKinds")} className="sm:col-span-2">
        <div className="flex flex-wrap items-center gap-4">
          {sourceKinds.map((k) => (
            <Checkbox
              key={k}
              label={t("source." + k)}
              checked={kinds.includes(k)}
              onChange={() => onChange({ ...value, source_kinds: toggle(value.source_kinds, k, sourceKinds) })}
            />
          ))}
        </div>
      </Field>
      <Checkbox
        className="sm:col-span-2"
        label={t("lab.includeExpired")}
        checked={!!value.include_expired}
        onChange={(e) => onChange({ ...value, include_expired: e.target.checked })}
      />
    </div>
  );
}

export function ConfigEditor({
  value,
  onChange,
  models,
  rerankAvailable,
}: {
  value: SearchConfig;
  onChange: (c: SearchConfig) => void;
  models: string[];
  rerankAvailable: boolean;
}) {
  const { t } = useTranslation();
  const num = (key: keyof SearchConfig, step = 1) => (
    <Input
      type="number"
      step={step}
      min={0}
      value={String(value[key] ?? "")}
      onChange={(e) => onChange({ ...value, [key]: e.target.value === "" ? 0 : Number(e.target.value) })}
    />
  );
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <Field label={t("lab.mode")}>
        <Select value={value.mode} onChange={(e) => onChange({ ...value, mode: e.target.value as Mode })}>
          {(["hybrid", "bm25", "vector"] as Mode[]).map((m) => (
            <option key={m} value={m}>
              {t("lab.modes." + m)}
            </option>
          ))}
        </Select>
      </Field>
      <Field label={t("lab.model")}>
        <Select
          value={value.model ?? ""}
          disabled={models.length === 0}
          onChange={(e) => onChange({ ...value, model: e.target.value })}
        >
          {models.length === 0 && <option value="">{t("lab.noModel")}</option>}
          {models.map((m) => (
            <option key={m} value={m}>
              {m}
            </option>
          ))}
        </Select>
      </Field>
      <Field label={t("lab.k")}>{num("k")}</Field>
      <Field label={t("lab.rrfk")}>{num("rrf_k")}</Field>
      <Field label={t("lab.bm25k")}>{num("bm25_k")}</Field>
      <Field label={t("lab.vectork")}>{num("vector_k")}</Field>
      <Field label={t("lab.bm25w")}>{num("bm25_weight", 0.1)}</Field>
      <Field label={t("lab.vectorw")}>{num("vector_weight", 0.1)}</Field>
      <div className="flex flex-col gap-2 sm:col-span-2">
        <Checkbox
          label={rerankAvailable ? t("lab.rerank") : t("lab.rerank") + " — " + t("lab.rerankUnavailable")}
          disabled={!rerankAvailable}
          checked={value.rerank}
          onChange={(e) => onChange({ ...value, rerank: e.target.checked })}
        />
        <Checkbox
          label={t("lab.keepOriginals")}
          checked={value.keep_originals}
          onChange={(e) => onChange({ ...value, keep_originals: e.target.checked })}
        />
        <Checkbox
          label={t("lab.ignoreRefs")}
          checked={value.ignore_refs}
          onChange={(e) => onChange({ ...value, ignore_refs: e.target.checked })}
        />
      </div>
    </div>
  );
}
