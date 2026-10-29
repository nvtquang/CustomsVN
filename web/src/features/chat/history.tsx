import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { Check, Pencil, Search, Trash2, X } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/form";
import { chatApi, type Conversation } from "@/lib/api/chat";
import { cn } from "@/lib/utils";

/** The conversation list in the sidebar: search, open, rename, delete. */
export function History() {
  const { t } = useTranslation();
  const [q, setQ] = useState("");
  const list = useQuery({ queryKey: ["conversations", q], queryFn: () => chatApi.conversations(q) });
  return (
    <div className="space-y-1.5">
      <div className="relative">
        <Search className="pointer-events-none absolute top-2.5 left-2 size-3.5 text-muted-foreground" />
        <Input
          aria-label={t("history.search")}
          placeholder={t("history.search")}
          className="h-8 pl-7 text-xs"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onClick={(e) => e.stopPropagation()}
        />
      </div>
      {list.data && list.data.length === 0 && (
        <p className="px-2.5 text-2xs text-muted-foreground">{t("history.empty")}</p>
      )}
      <ul className="space-y-0.5">{list.data?.map((c) => <Item key={c.id} c={c} />)}</ul>
    </div>
  );
}

function Item({ c }: { c: Conversation }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const params = useParams({ strict: false }) as { id?: string };
  const active = params.id === String(c.id);
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState(c.title);
  const rename = useMutation({
    mutationFn: () => chatApi.rename(c.id, title),
    onSuccess: () => {
      setEditing(false);
      void qc.invalidateQueries({ queryKey: ["conversations"] });
    },
  });
  const remove = useMutation({
    mutationFn: () => chatApi.remove(c.id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["conversations"] });
      if (active) void navigate({ to: "/" });
    },
  });
  if (editing) {
    return (
      <li onClick={(e) => e.stopPropagation()}>
        <form
          className="flex items-center gap-1"
          onSubmit={(e) => {
            e.preventDefault();
            rename.mutate();
          }}
        >
          <Input autoFocus className="h-8 text-xs" value={title} onChange={(e) => setTitle(e.target.value)} />
          <button type="submit" aria-label={t("history.save")} className="cursor-pointer p-1 text-muted-foreground">
            <Check className="size-3.5" />
          </button>
          <button
            type="button"
            aria-label={t("history.cancel")}
            className="cursor-pointer p-1 text-muted-foreground"
            onClick={() => setEditing(false)}
          >
            <X className="size-3.5" />
          </button>
        </form>
      </li>
    );
  }
  return (
    <li className="group relative">
      <Link
        to="/c/$id"
        params={{ id: String(c.id) }}
        className={cn(
          "block truncate rounded-md px-2.5 py-1.5 pr-12 text-sm hover:bg-muted",
          active && "bg-accent font-medium text-accent-foreground",
        )}
        title={c.title}
      >
        {c.title}
      </Link>
      <div className="absolute top-1 right-1 hidden gap-0.5 group-hover:flex" onClick={(e) => e.stopPropagation()}>
        <button
          type="button"
          aria-label={t("history.rename")}
          title={t("history.rename")}
          className="cursor-pointer rounded p-1 text-muted-foreground hover:bg-surface"
          onClick={() => setEditing(true)}
        >
          <Pencil className="size-3" />
        </button>
        <button
          type="button"
          aria-label={t("history.delete")}
          title={t("history.delete")}
          className="cursor-pointer rounded p-1 text-muted-foreground hover:bg-surface hover:text-destructive"
          onClick={() => window.confirm(t("history.confirmDelete")) && remove.mutate()}
        >
          <Trash2 className="size-3" />
        </button>
      </div>
    </li>
  );
}
