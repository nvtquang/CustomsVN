import { Check } from "lucide-react";
import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

/** An on/off switch with its label and an optional line of explanation. */
export function Switch({
  checked,
  onChange,
  label,
  description,
  disabled,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  label: ReactNode;
  description?: ReactNode;
  disabled?: boolean;
}) {
  return (
    <label
      className={cn(
        "flex cursor-pointer items-start justify-between gap-4",
        disabled && "cursor-not-allowed opacity-60",
      )}
    >
      <span className="space-y-0.5">
        <span className="block text-sm font-medium">{label}</span>
        {description && <span className="block text-xs text-muted-foreground">{description}</span>}
      </span>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        disabled={disabled}
        onClick={() => onChange(!checked)}
        className={cn(
          "relative mt-0.5 inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full transition-colors focus-visible:outline-2 focus-visible:outline-ring disabled:cursor-not-allowed",
          checked ? "bg-primary" : "bg-input",
        )}
      >
        <span
          className={cn(
            "inline-block size-4 rounded-full bg-surface shadow-xs transition-transform",
            checked ? "translate-x-4.5" : "translate-x-0.5",
          )}
        />
      </button>
    </label>
  );
}

/**
 * A selectable card: one of a set (radio) or any of a set (checkbox), with a
 * title, an explanation and an optional badge such as "Khuyên dùng".
 */
export function ChoiceCard({
  type,
  name,
  checked,
  onChange,
  title,
  description,
  badge,
  icon,
  disabled,
  note,
}: {
  type: "radio" | "checkbox";
  name?: string;
  checked: boolean;
  onChange: (v: boolean) => void;
  title: ReactNode;
  description?: ReactNode;
  badge?: ReactNode;
  icon?: ReactNode;
  disabled?: boolean;
  note?: ReactNode;
}) {
  return (
    <label
      className={cn(
        "relative flex cursor-pointer gap-3 rounded-lg border bg-surface p-3 transition-colors hover:bg-surface-2 has-focus-visible:outline-2 has-focus-visible:outline-ring",
        checked && "border-primary bg-accent/40 hover:bg-accent/50",
        disabled && "cursor-not-allowed opacity-60 hover:bg-surface",
      )}
    >
      <input
        type={type}
        name={name}
        className="sr-only"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
      />
      {icon && <span className="mt-0.5 text-primary [&_svg]:size-5">{icon}</span>}
      <span className="min-w-0 flex-1 space-y-1">
        <span className="flex flex-wrap items-center gap-2 text-sm font-medium">
          {title}
          {badge}
        </span>
        {description && <span className="block text-xs leading-relaxed text-muted-foreground">{description}</span>}
        {note && <span className="block text-xs text-force-partly">{note}</span>}
      </span>
      <span
        aria-hidden
        className={cn(
          "mt-0.5 flex size-4 shrink-0 items-center justify-center border",
          type === "radio" ? "rounded-full" : "rounded-sm",
          checked ? "border-primary bg-primary text-primary-foreground" : "border-input",
        )}
      >
        {checked && <Check className="size-3" strokeWidth={3} />}
      </span>
    </label>
  );
}
