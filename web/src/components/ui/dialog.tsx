import { X } from "lucide-react";
import { useEffect, useId, useRef, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "./button";

/**
 * A modal dialog: Esc and the backdrop close it, focus moves in on open and
 * back to where it was on close.
 */
export function Dialog({
  title,
  description,
  onClose,
  children,
  footer,
}: {
  title: ReactNode;
  description?: ReactNode;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
}) {
  const { t } = useTranslation();
  const id = useId();
  const box = useRef<HTMLDivElement>(null);
  // Held in a ref: a parent that passes a new function each render must not
  // re-run the effect, which would pull focus back to the first field.
  const close = useRef(onClose);
  useEffect(() => {
    close.current = onClose;
  });
  useEffect(() => {
    const back = document.activeElement as HTMLElement | null;
    const first = box.current?.querySelector<HTMLElement>("input, select, textarea, button:not([data-close])");
    first?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        close.current();
      }
    };
    window.addEventListener("keydown", onKey, true);
    return () => {
      window.removeEventListener("keydown", onKey, true);
      back?.focus();
    };
  }, []);
  return (
    <div
      className="fixed inset-0 z-50 flex items-end justify-center bg-black/40 sm:items-center sm:p-4"
      onClick={onClose}
    >
      <div
        ref={box}
        role="dialog"
        aria-modal="true"
        aria-labelledby={id}
        className="flex max-h-[92dvh] w-full max-w-lg flex-col rounded-t-xl border bg-surface shadow-lg sm:rounded-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start justify-between gap-3 border-b px-5 py-4">
          <div className="space-y-1">
            <h2 id={id} className="text-base font-semibold">
              {title}
            </h2>
            {description && <p className="text-xs text-muted-foreground">{description}</p>}
          </div>
          <Button variant="ghost" size="icon" data-close aria-label={t("nav.close")} onClick={onClose}>
            <X />
          </Button>
        </div>
        <div className="min-h-0 flex-1 space-y-4 overflow-y-auto px-5 py-4">{children}</div>
        {footer && <div className="flex flex-wrap items-center justify-end gap-2 border-t px-5 py-3">{footer}</div>}
      </div>
    </div>
  );
}
