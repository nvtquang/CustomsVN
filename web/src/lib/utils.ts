import { twMerge } from "tailwind-merge";

export type ClassValue = string | number | bigint | boolean | null | undefined | ClassValue[] | Record<string, unknown>;

/** Joins class names the way clsx does: falsy values are skipped, arrays are flattened, objects add their truthy keys. */
export function cx(...inputs: ClassValue[]): string {
  let out = "";
  for (const v of inputs) {
    if (!v || v === true) continue;
    let s = "";
    if (typeof v === "string" || typeof v === "number" || typeof v === "bigint") s = String(v);
    else if (Array.isArray(v)) s = cx(...v);
    else
      s = Object.keys(v)
        .filter((k) => v[k])
        .join(" ");
    if (s) out = out ? out + " " + s : s;
  }
  return out;
}

export function cn(...inputs: ClassValue[]) {
  return twMerge(cx(inputs));
}

/** Formats a number to a fixed precision, or a dash when there is none. */
export function fmt(n: number | null | undefined, digits = 3): string {
  return n == null || Number.isNaN(n) ? "—" : n.toFixed(digits);
}

/** Formats an ISO timestamp for a table cell. */
export function when(iso?: string | null): string {
  if (!iso) return "—";
  const d = new Date(iso);
  return d.toLocaleString(undefined, { dateStyle: "short", timeStyle: "short" });
}
