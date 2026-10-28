import { Link } from "@tanstack/react-router";
import {
  Boxes,
  Database,
  FileJson,
  Gauge,
  Library,
  LogOut,
  MessageSquareWarning,
  Plug,
  SlidersHorizontal,
  UserRound,
  Users,
  X,
} from "lucide-react";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { useLogout } from "@/features/auth/use-logout";

interface Item {
  to: string;
  label: string;
  icon: ReactNode;
  exact?: boolean;
}

/**
 * Settings, account and administration in one place, as Dify and WeKnora lay
 * them out: sections on the left, the page on the right, a close button back
 * to where the user was.
 */
export function SettingsFrame({
  admin,
  onClose,
  children,
}: {
  admin: boolean;
  onClose: () => void;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const logout = useLogout();
  const sections: { title: string; items: Item[] }[] = [
    {
      title: t("set.sections.account"),
      items: [
        { to: "/settings/general", label: t("set.general"), icon: <SlidersHorizontal /> },
        { to: "/settings/account", label: t("set.account"), icon: <UserRound /> },
      ],
    },
  ];
  if (admin) {
    sections.push(
      {
        title: t("set.sections.ai"),
        items: [
          { to: "/settings/models", label: t("set.models"), icon: <Boxes /> },
          { to: "/settings/knowledge", label: t("set.knowledge"), icon: <Library /> },
        ],
      },
      {
        title: t("set.sections.admin"),
        items: [
          { to: "/admin", label: t("nav.dashboard"), icon: <Gauge />, exact: true },
          { to: "/admin/feedback", label: t("nav.feedback"), icon: <MessageSquareWarning /> },
          { to: "/admin/data", label: t("nav.data"), icon: <Database /> },
          { to: "/admin/users", label: t("nav.users"), icon: <Users /> },
        ],
      },
    );
  }
  sections.push({
    title: t("set.sections.integrate"),
    items: [
      { to: "/settings/integrations", label: t("set.integrations"), icon: <Plug /> },
      ...(admin ? [{ to: "/settings/config", label: t("set.config"), icon: <FileJson /> }] : []),
    ],
  });
  const link = (it: Item, compact?: boolean) => (
    <Link
      key={it.to}
      to={it.to}
      activeOptions={{ exact: !!it.exact }}
      className={
        compact
          ? "flex shrink-0 items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs whitespace-nowrap text-muted-foreground hover:bg-muted [&_svg]:size-3.5"
          : "flex items-center gap-2.5 rounded-md px-2.5 py-2 text-sm text-foreground/80 hover:bg-muted [&_svg]:size-4 [&_svg]:text-muted-foreground"
      }
      activeProps={{
        className: "bg-accent font-medium !text-accent-foreground [&_svg]:!text-primary",
        "aria-current": "page",
      }}
    >
      {it.icon}
      {it.label}
    </Link>
  );

  return (
    <div className="flex h-dvh overflow-hidden bg-background">
      <aside className="hidden w-60 shrink-0 flex-col border-r bg-surface lg:flex">
        <div className="flex h-14 items-center px-5 text-base font-semibold">{t("set.title")}</div>
        <nav aria-label={t("set.nav")} className="flex-1 space-y-5 overflow-y-auto px-3 pb-3">
          {sections.map((s) => (
            <div key={s.title} className="space-y-0.5">
              <div className="px-2.5 pb-1 text-2xs font-medium text-muted-foreground">{s.title}</div>
              {s.items.map((it) => link(it))}
            </div>
          ))}
        </nav>
        <div className="border-t p-3">
          <button
            type="button"
            onClick={() => void logout()}
            className="flex w-full cursor-pointer items-center gap-2.5 rounded-md px-2.5 py-2 text-sm text-destructive hover:bg-muted"
          >
            <LogOut className="size-4" />
            {t("auth.logout")}
          </button>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 shrink-0 items-center justify-between gap-3 border-b px-4 lg:border-b-0">
          <span className="font-semibold lg:hidden">{t("set.title")}</span>
          <span className="hidden lg:block" />
          <Button variant="ghost" size="icon" aria-label={t("set.close")} title={t("set.close")} onClick={onClose}>
            <X />
          </Button>
        </header>
        {/* On a phone the sections scroll sideways above the page. */}
        <nav aria-label={t("set.nav")} className="flex gap-1 overflow-x-auto border-b px-3 py-2 lg:hidden">
          {sections.flatMap((s) => s.items).map((it) => link(it, true))}
          <button
            type="button"
            onClick={() => void logout()}
            className="flex shrink-0 cursor-pointer items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs whitespace-nowrap text-destructive hover:bg-muted"
          >
            <LogOut className="size-3.5" />
            {t("auth.logout")}
          </button>
        </nav>
        <main id="main" tabIndex={-1} className="min-h-0 flex-1 overflow-y-auto focus:outline-none">
          {children}
        </main>
      </div>
    </div>
  );
}
