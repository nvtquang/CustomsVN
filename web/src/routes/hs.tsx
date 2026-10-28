import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { Search } from "lucide-react";
import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/form";
import { HSLineView } from "@/features/chat/cards";
import { chatApi } from "@/lib/api/chat";

export const Route = createFileRoute("/hs")({
  validateSearch: (s: Record<string, unknown>): { q?: string } => ({
    // The router parses "8703.23" as a number; a code is still a code.
    q: typeof s.q === "string" || typeof s.q === "number" ? String(s.q) : undefined,
  }),
  component: HSLookup,
});

/** HS lookup: by code prefix or by description, with the parent chain and both languages. */
function HSLookup() {
  const { t } = useTranslation();
  const { q } = Route.useSearch();
  const navigate = useNavigate({ from: Route.fullPath });
  const [input, setInput] = useState(q ?? "");
  const res = useQuery({ queryKey: ["hs", q], queryFn: () => chatApi.hs(q!), enabled: !!q });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (input.trim()) void navigate({ search: { q: input.trim() } });
  };
  return (
    <div className="mx-auto max-w-4xl space-y-4 p-4 lg:p-6">
      <div>
        <h1 className="text-xl font-semibold">{t("hs.title")}</h1>
        <p className="text-xs text-muted-foreground">{t("hs.subtitle")}</p>
      </div>
      <form onSubmit={submit} className="flex gap-2">
        <Input
          aria-label={t("hs.placeholder")}
          placeholder={t("hs.placeholder")}
          value={input}
          onChange={(e) => setInput(e.target.value)}
        />
        <Button type="submit" disabled={!input.trim()}>
          <Search /> {t("hs.search")}
        </Button>
      </form>
      {res.error && <p className="text-destructive">{t("common.error", { msg: res.error.message })}</p>}
      {res.data && (
        <Card>
          <CardContent className="space-y-1">
            <div className="flex flex-wrap items-center justify-between gap-2 pb-2 text-xs text-muted-foreground">
              <span>{t(res.data.by_code ? "hs.byCode" : "hs.byText", { n: res.data.lines.length })}</span>
              <Link
                to="/doc/$id"
                params={{ id: res.data.source_doc_id }}
                search={{}}
                className="text-primary hover:underline"
              >
                {t("hs.source", { doc: res.data.source })}
              </Link>
            </div>
            {res.data.lines.length === 0 && <p className="text-sm text-muted-foreground">{t("hs.none")}</p>}
            <ul className="divide-y">
              {res.data.lines.map((l, i) => (
                <li key={l.position} className="py-3">
                  <HSLineView l={l} open={res.data!.by_code && i === 0} />
                </li>
              ))}
            </ul>
            <p className="pt-2 text-2xs text-muted-foreground">{t("hs.note")}</p>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
