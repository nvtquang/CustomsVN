import type { ReactNode } from "react";

/**
 * Marks the first occurrence of quote in text, ignoring differences in white
 * space, which is how the server verified it. Pieces of a quote joined by an
 * ellipsis are marked one by one.
 */
export function highlight(text: string, quote: string | null | undefined): ReactNode {
  if (!quote) return text;
  const pieces = quote
    .replace(/^["'“”‘’\s]+|["'“”‘’\s]+$/g, "")
    .split(/\s*(?:\.\.\.|…)\s*/)
    .map((p) => p.trim().replace(/[.;,:]+$/, ""))
    .filter(Boolean);
  const out: ReactNode[] = [];
  let rest = text;
  let key = 0;
  for (const p of pieces) {
    const re = new RegExp(
      p
        .split(/\s+/)
        .map((w) => w.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"))
        .join("\\s+"),
    );
    const m = re.exec(rest);
    if (!m) continue;
    out.push(rest.slice(0, m.index));
    out.push(
      <mark key={key++} className="rounded-sm bg-force-partly-bg px-0.5 text-foreground" data-quote>
        {m[0]}
      </mark>,
    );
    rest = rest.slice(m.index + m[0].length);
  }
  out.push(rest);
  return out;
}

/**
 * Marks the part of a quote that falls in one provision. A quote taken from a
 * chunk can run across a clause and its points, which the document viewer
 * shows as separate provisions with their markers ("1.", "a)") outside the
 * text; the quote is split at those markers and each provision marks the
 * pieces it holds.
 */
export function highlightPart(text: string, quote: string | null | undefined): ReactNode {
  if (!quote) return text;
  const pieces = quote
    .split(/\s*(?:^|\s)(?:\d+[a-zđ]?\.|[a-zđ]\))\s+/)
    .map((p) => p.trim())
    .filter((p) => p.length >= 8);
  for (const p of pieces) {
    const marked = highlight(text, p);
    if (Array.isArray(marked) && marked.length > 1) return marked;
  }
  return text;
}
