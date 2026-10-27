import { useQuery } from "@tanstack/react-query";
import { Link, Outlet } from "@tanstack/react-router";
import { Bot, FlaskConical, Menu, MessageSquarePlus, Monitor, Moon, Settings2, Sun, X } from "lucide-react";
import { useEffect, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { History } from "@/features/chat/history";
import { ChunkPanel } from "@/features/documents/chunk-panel";
import { api } from "@/lib/api/client";
import { languages, setLanguage, type Language } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { applyTheme, useUI, type Theme } from "@/stores/ui";

/** Sidebar on the left, the screen in the middle, the detail panel on the right. */
export function AppShell() {
  const { t } = useTranslation();
  const theme = useUI((s) => s.theme);
  const sidebarOpen = useUI((s) => s.sidebarOpen);
  const setSidebarOpen = useUI((s) => s.setSidebarOpen);
  const panel = useUI((s) => s.panelChunk);
  const openChunk = useUI((s) => s.openChunk);

  useEffect(() => {
    applyTheme(theme);
    if (theme !== "system") return;
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const on = () => applyTheme("system");
    mq.addEventListener("change", on);
    return () => mq.removeEventListener("change", on);
  }, [theme]);

  return (
    <div className="flex h-dvh overflow-hidden">
      {/* Sidebar: fixed column on wide screens, an overlay on narrow ones. */}
      <div
        className={cn("fixed inset-0 z-30 bg-black/40 lg:hidden", sidebarOpen ? "block" : "hidden")}
        onClick={() => setSidebarOpen(false)}
      />
      <aside
        className={cn(
          "fixed inset-y-0 left-0 z-40 flex w-[var(--sidebar-width)] flex-col border-r bg-surface transition-transform lg:static lg:translate-x-0",
          sidebarOpen ? "translate-x-0" : "-translate-x-full",
        )}
      >
        <div className="flex h-14 items-center justify-between border-b px-4">
          <div>
            <div className="text-base font-semibold text-primary">{t("app.name")}</div>
            <div className="text-2xs text-muted-foreground">{t("app.tagline")}</div>
          </div>
          <Button
            variant="ghost"
            size="icon"
            className="lg:hidden"
            aria-label={t("nav.close")}
            onClick={() => setSidebarOpen(false)}
          >
            <X />
          </Button>
        </div>
        <nav className="flex-1 space-y-5 overflow-y-auto p-3" onClick={() => setSidebarOpen(false)}>
          <NavSection title={t("nav.sectionWork")}>
            <NavLink to="/" icon={<MessageSquarePlus className="size-4" />} exact>
              {t("chat.newChat")}
            </NavLink>
            <History />
          </NavSection>
          <NavSection title={t("nav.sectionLab")}>
            <NavLink to="/lab/retrieval" icon={<FlaskConical className="size-4" />}>
              {t("nav.lab")}
            </NavLink>
          </NavSection>
          <NavSection title={t("nav.sectionSettings")}>
            <NavLink to="/settings/retrieval" icon={<Settings2 className="size-4" />}>
              {t("nav.settings")}
            </NavLink>
            <NavLink to="/settings/llm" icon={<Bot className="size-4" />}>
              {t("nav.llm")}
            </NavLink>
          </NavSection>
        </nav>
        <Preferences />
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 shrink-0 items-center gap-3 border-b bg-surface px-4">
          <Button
            variant="ghost"
            size="icon"
            className="lg:hidden"
            aria-label={t("nav.menu")}
            onClick={() => setSidebarOpen(true)}
          >
            <Menu />
          </Button>
          <CorpusStatus />
        </header>
        <main className="min-h-0 flex-1 overflow-y-auto">
          <Outlet />
        </main>
        <footer className="shrink-0 border-t px-4 py-1.5 text-2xs text-muted-foreground">{t("disclaimer")}</footer>
      </div>

      {/* Right panel: a column on very wide screens, a drawer otherwise. */}
      {panel && (
        <>
          <div className="fixed inset-0 z-30 bg-black/40 xl:hidden" onClick={() => openChunk(null)} />
          <aside className="fixed inset-y-0 right-0 z-40 w-full max-w-[var(--panel-width)] border-l bg-surface shadow-lg xl:static xl:z-auto xl:shadow-none">
            <ChunkPanel />
          </aside>
        </>
      )}
    </div>
  );
}

function NavSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="space-y-1">
      <div className="px-2.5 text-2xs font-medium tracking-wide text-muted-foreground uppercase">{title}</div>
      {children}
    </div>
  );
}

function NavLink({ to, icon, children, exact }: { to: string; icon: ReactNode; children: ReactNode; exact?: boolean }) {
  return (
    <Link
      to={to}
      className="flex items-center gap-2 rounded-md px-2.5 py-2 text-sm hover:bg-muted"
      activeOptions={{ exact: !!exact }}
      activeProps={{ className: "bg-accent text-accent-foreground font-medium" }}
    >
      {icon}
      {children}
    </Link>
  );
}

function CorpusStatus() {
  const { t } = useTranslation();
  const q = useQuery({ queryKey: ["meta"], queryFn: api.meta, staleTime: 60_000 });
  if (q.error) return <span className="text-xs text-destructive">{t("corpus.unavailable")}</span>;
  if (!q.data) return <span className="text-xs text-muted-foreground">{t("common.loading")}</span>;
  return (
    <div className="flex min-w-0 items-baseline gap-2 text-xs">
      <span className="font-medium">{t("corpus.version", { v: q.data.corpus_version })}</span>
      <span className="truncate text-muted-foreground">
        {t("corpus.summary", {
          docs: q.data.documents.toLocaleString(),
          chunks: q.data.chunks.toLocaleString(),
          asOf: q.data.corpus_as_of,
        })}
      </span>
    </div>
  );
}

function Preferences() {
  const { t, i18n } = useTranslation();
  const theme = useUI((s) => s.theme);
  const setTheme = useUI((s) => s.setTheme);
  const themes: { value: Theme; icon: ReactNode }[] = [
    { value: "light", icon: <Sun /> },
    { value: "dark", icon: <Moon /> },
    { value: "system", icon: <Monitor /> },
  ];
  return (
    <div className="space-y-2 border-t p-3">
      <div className="flex items-center justify-between">
        <span className="text-2xs text-muted-foreground">{t("theme.label")}</span>
        <div className="flex rounded-md bg-muted p-0.5">
          {themes.map((th) => (
            <button
              key={th.value}
              type="button"
              title={t("theme." + th.value)}
              aria-label={t("theme." + th.value)}
              aria-pressed={theme === th.value}
              onClick={() => setTheme(th.value)}
              className={cn(
                "cursor-pointer rounded-sm p-1.5 text-muted-foreground [&_svg]:size-3.5",
                theme === th.value && "bg-surface text-foreground shadow-xs",
              )}
            >
              {th.icon}
            </button>
          ))}
        </div>
      </div>
      <div className="flex items-center justify-between">
        <span className="text-2xs text-muted-foreground">{t("lang.label")}</span>
        <div className="flex rounded-md bg-muted p-0.5">
          {languages.map((l: Language) => (
            <button
              key={l}
              type="button"
              aria-pressed={i18n.language === l}
              onClick={() => setLanguage(l)}
              className={cn(
                "cursor-pointer rounded-sm px-2 py-0.5 text-2xs font-medium text-muted-foreground uppercase",
                i18n.language === l && "bg-surface text-foreground shadow-xs",
              )}
            >
              {l}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
