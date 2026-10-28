import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useMe } from "@/features/auth/use-me";

/**
 * Wraps an admin screen. The server refuses the calls anyway; this spares a
 * user who follows a link a screen of errors.
 */
export function AdminOnly({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const me = useMe().data;
  if (me?.role !== "admin") return <p className="p-6 text-sm text-muted-foreground">{t("admin.only")}</p>;
  return <>{children}</>;
}

/** A page heading with its one-line purpose. */
export function PageTitle({ title, subtitle, actions }: { title: string; subtitle?: string; actions?: ReactNode }) {
  return (
    <div className="flex flex-wrap items-end justify-between gap-3">
      <div>
        <h1 className="text-xl font-semibold">{title}</h1>
        {subtitle && <p className="text-xs text-muted-foreground">{subtitle}</p>}
      </div>
      {actions}
    </div>
  );
}

/** Placeholder rows while a table loads, so the layout does not jump. */
export function Skeleton({ rows = 4 }: { rows?: number }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2" role="status" aria-label={t("common.loading")}>
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="h-8 animate-pulse rounded-md bg-muted" />
      ))}
    </div>
  );
}

export function ErrorLine({ error }: { error: Error | null }) {
  const { t } = useTranslation();
  if (!error) return null;
  return (
    <p role="alert" className="text-sm text-destructive">
      {t("common.error", { msg: error.message })}
    </p>
  );
}
