import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

/** Formats a number to a fixed precision, or a dash when there is none. */
export function fmt(n: number | null | undefined, digits = 3): string {
  return n == null || Number.isNaN(n) ? "—" : n.toFixed(digits);
}

/** Today in the browser's time zone, YYYY-MM-DD. */
export function today(): string {
  const d = new Date();
  const p = (n: number) => String(n).padStart(2, "0");
  return d.getFullYear() + "-" + p(d.getMonth() + 1) + "-" + p(d.getDate());
}

/** Formats an ISO timestamp for a table cell. */
export function when(iso?: string | null): string {
  if (!iso) return "—";
  const d = new Date(iso);
  return d.toLocaleString(undefined, { dateStyle: "short", timeStyle: "short" });
}
