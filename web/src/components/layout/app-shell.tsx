import { useQuery } from "@tanstack/react-query";
import { Link, Outlet, useNavigate, useRouterState } from "@tanstack/react-router";
import {
  ChevronsUpDown,
  FlaskConical,
  Gauge,
  Keyboard,
  ListTree,
  Loader2,
  LogOut,
  Menu,
  MessageSquarePlus,
  Settings,
  UserRound,
  X,
} from "lucide-react";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { LoginScreen } from "@/features/auth/login";
import { useLogout } from "@/features/auth/use-logout";
import { useMe } from "@/features/auth/use-me";
import { History } from "@/features/chat/history";
import { ChunkPanel } from "@/features/documents/chunk-panel";
import { isSettingsPath } from "@/features/settings/paths";
import { SettingsFrame } from "@/features/settings/settings-frame";
import { SHORTCUTS } from "@/features/settings/shortcuts";
import { api } from "@/lib/api/client";
import { cn } from "@/lib/utils";
import { applyTheme, useUI } from "@/stores/ui";

/** The login screen until someone is logged in, then the app. */
export function AppShell() {
  const { t } = useTranslation();
  const me = useMe();
  const theme = useUI((s) => s.theme);
  useEffect(() => {
    applyTheme(theme);
  }, [theme]);
  if (me.isPending) {
    return (
      <div className="flex h-dvh items-center justify-center text-muted-foreground" role="status">
        <Loader2 className="mr-2 size-4 animate-spin" />
        {t("common.loading")}
      </div>
    );
  }
  if (me.error) {
    return (
      <div className="flex h-dvh flex-col items-center justify-center gap-3 text-sm" role="alert">
        <p className="text-destructive">{t("corpus.unavailable")}</p>
        <Button variant="outline" onClick={() => void me.refetch()}>
          {t("common.retry")}
        </Button>
      </div>
    );
  }
  if (!me.data) return <LoginScreen />;
  return <Shell admin={me.data.role === "admin"} />;
}

/** Where closing the settings goes back to: the last screen outside them. */
let lastAppPath = "/";

/**
 * Sidebar on the left, the screen in the middle, the detail panel on the
 * right; settings and administration take the whole window instead.
 */
function Shell({ admin }: { admin: boolean }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const path = useRouterState({ select: (s) => s.location.pathname });
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
  const [shortcuts, setShortcuts] = useShortcuts();
  const inSettings = isSettingsPath(path);
  useEffect(() => {
    if (!inSettings) lastAppPath = path;
  }, [inSettings, path]);

  const skip = (
    <a
      href="#main"
      className="sr-only z-50 rounded-md bg-primary px-3 py-2 text-sm text-primary-foreground focus:not-sr-only focus:fixed focus:top-2 focus:left-2"
    >
      {t("nav.skip")}
    </a>
  );
  if (inSettings) {
    return (
      <>
        {skip}
        <SettingsFrame admin={admin} onClose={() => void navigate({ to: lastAppPath })}>
          <Outlet />
        </SettingsFrame>
        {shortcuts && <ShortcutsDialog onClose={() => setShortcuts(false)} />}
      </>
    );
  }

  return (
    <div className="flex h-dvh overflow-hidden">
      {skip}
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
        <nav className="space-y-0.5 p-3 pb-2" onClick={() => setSidebarOpen(false)}>
          <NavLink to="/" icon={<MessageSquarePlus className="size-4" />} active={path === "/"}>
            {t("chat.newChat")}
          </NavLink>
          <NavLink to="/hs" icon={<ListTree className="size-4" />} active={path.startsWith("/hs")}>
            {t("nav.hs")}
          </NavLink>
          {admin && (
            <NavLink to="/lab/retrieval" icon={<FlaskConical className="size-4" />} active={path.startsWith("/lab")}>
              {t("nav.labRoom")}
            </NavLink>
          )}
          <NavLink to="/settings/general" icon={<Settings className="size-4" />} active={false}>
            {t("nav.settingsAll")}
          </NavLink>
        </nav>
        <div className="min-h-0 flex-1 overflow-y-auto border-t px-3 pt-3 pb-3" onClick={() => setSidebarOpen(false)}>
          <div className="px-2.5 pb-1.5 text-2xs font-medium tracking-wide text-muted-foreground uppercase">
            {t("history.title")}
          </div>
          <History />
        </div>
        <UserMenu admin={admin} onShortcuts={() => setShortcuts(true)} />
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
        <main id="main" tabIndex={-1} className="min-h-0 flex-1 overflow-y-auto focus:outline-none">
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
      {shortcuts && <ShortcutsDialog onClose={() => setShortcuts(false)} />}
    </div>
  );
}

function isTyping(e: KeyboardEvent) {
  const el = e.target as HTMLElement | null;
  return (
    !!el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.tagName === "SELECT" || el.isContentEditable)
  );
}

/**
 * Keyboard shortcuts: Ctrl+K (or Cmd+K) starts a new question, "/" jumps to
 * the question box, Esc closes the panel and the menu, "?" lists them.
 */
function useShortcuts(): [boolean, (open: boolean) => void] {
  const navigate = useNavigate();
  const openChunk = useUI((s) => s.openChunk);
  const setSidebarOpen = useUI((s) => s.setSidebarOpen);
  const [help, setHelp] = useState(false);
  useEffect(() => {
    const focusQuestion = () => window.setTimeout(() => document.getElementById("question")?.focus(), 50);
    const on = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        void navigate({ to: "/" });
        focusQuestion();
        return;
      }
      if (e.key === "Escape") {
        openChunk(null);
        setSidebarOpen(false);
        setHelp(false);
        return;
      }
      if (isTyping(e) || e.ctrlKey || e.metaKey || e.altKey) return;
      if (e.key === "/") {
        const q = document.getElementById("question");
        if (q) {
          e.preventDefault();
          q.focus();
        }
      } else if (e.key === "?") {
        setHelp(true);
      }
    };
    window.addEventListener("keydown", on);
    return () => window.removeEventListener("keydown", on);
  }, [navigate, openChunk, setSidebarOpen]);
  return [help, setHelp];
}

function ShortcutsDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" onClick={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="shortcuts-title"
        className="w-full max-w-sm rounded-lg border bg-surface p-5 shadow-lg"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-3 flex items-center justify-between">
          <h2 id="shortcuts-title" className="text-sm font-semibold">
            {t("shortcuts.title")}
          </h2>
          <Button variant="ghost" size="icon" aria-label={t("nav.close")} onClick={onClose} autoFocus>
            <X />
          </Button>
        </div>
        <ShortcutList />
      </div>
    </div>
  );
}

export function ShortcutList() {
  const { t } = useTranslation();
  return (
    <dl className="space-y-2 text-sm">
      {SHORTCUTS.map(([k, label]) => (
        <div key={k} className="flex items-center justify-between gap-4">
          <dt className="text-muted-foreground">{t(label)}</dt>
          <dd>
            <kbd className="rounded border bg-muted px-1.5 py-0.5 font-mono text-2xs">{k}</kbd>
          </dd>
        </div>
      ))}
    </dl>
  );
}

/**
 * The user bar at the foot of the sidebar. It opens a menu with the settings,
 * the account, administration for admins, the shortcuts and logging out.
 */
function UserMenu({ admin, onShortcuts }: { admin: boolean; onShortcuts: () => void }) {
  const { t } = useTranslation();
  const me = useMe().data;
  const logout = useLogout();
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const away = (e: MouseEvent) => {
      if (!box.current?.contains(e.target as Node)) setOpen(false);
    };
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", away);
    document.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("mousedown", away);
      document.removeEventListener("keydown", esc);
    };
  }, [open]);
  if (!me) return null;
  const name = me.name || me.email;
  const item =
    "flex w-full cursor-pointer items-center gap-2.5 rounded-md px-2.5 py-2 text-sm hover:bg-muted [&_svg]:size-4 [&_svg]:text-muted-foreground";
  const close = () => setOpen(false);
  return (
    <div ref={box} className="relative border-t p-2">
      {open && (
        <div
          role="menu"
          aria-label={t("nav.userMenu")}
          className="absolute right-2 bottom-full left-2 mb-1 space-y-0.5 rounded-lg border bg-surface p-1.5 shadow-lg"
        >
          <div className="px-2.5 pt-1 pb-2">
            <div className="truncate text-xs font-medium">{me.email}</div>
            <div className="text-2xs text-muted-foreground">{t("auth.roles." + me.role)}</div>
          </div>
          <Link role="menuitem" to="/settings/general" className={item} onClick={close}>
            <Settings /> {t("nav.settingsAll")}
          </Link>
          <Link role="menuitem" to="/settings/account" className={item} onClick={close}>
            <UserRound /> {t("set.account")}
          </Link>
          {admin && (
            <Link role="menuitem" to="/admin" className={item} onClick={close}>
              <Gauge /> {t("nav.admin")}
            </Link>
          )}
          <button
            role="menuitem"
            type="button"
            className={item}
            onClick={() => {
              close();
              onShortcuts();
            }}
          >
            <Keyboard /> {t("shortcuts.title")}
          </button>
          <div className="my-1 border-t" />
          <button role="menuitem" type="button" className={cn(item, "text-destructive")} onClick={() => void logout()}>
            <LogOut className="!text-destructive" /> {t("auth.logout")}
          </button>
        </div>
      )}
      <button
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={t("nav.userMenu")}
        onClick={() => setOpen(!open)}
        className="flex w-full cursor-pointer items-center gap-2.5 rounded-md p-1.5 text-left hover:bg-muted"
      >
        <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-accent text-sm font-semibold text-accent-foreground uppercase">
          {name.slice(0, 1)}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-xs font-medium" title={me.email}>
            {name}
          </span>
          <span className="block text-2xs text-muted-foreground">{t("auth.roles." + me.role)}</span>
        </span>
        <ChevronsUpDown className="size-4 shrink-0 text-muted-foreground" />
      </button>
    </div>
  );
}

function NavLink({
  to,
  icon,
  children,
  active,
}: {
  to: string;
  icon: ReactNode;
  children: ReactNode;
  active: boolean;
}) {
  return (
    <Link
      to={to}
      className={cn(
        "flex items-center gap-2 rounded-md px-2.5 py-2 text-sm hover:bg-muted",
        active && "bg-accent font-medium text-accent-foreground",
      )}
      aria-current={active ? "page" : undefined}
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
