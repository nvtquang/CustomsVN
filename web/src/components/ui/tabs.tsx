import { cn } from "@/lib/utils";

/** A row of tabs. The caller owns the selected value. */
export function Tabs<T extends string>({
  value,
  onChange,
  items,
  className,
}: {
  value: T;
  onChange: (v: T) => void;
  items: { value: T; label: string }[];
  className?: string;
}) {
  return (
    <div role="tablist" className={cn("inline-flex rounded-md bg-muted p-1", className)}>
      {items.map((it) => (
        <button
          key={it.value}
          role="tab"
          type="button"
          aria-selected={value === it.value}
          onClick={() => onChange(it.value)}
          className={cn(
            "cursor-pointer rounded-sm px-3 py-1.5 text-sm font-medium text-muted-foreground transition-colors",
            value === it.value && "bg-surface text-foreground shadow-xs",
          )}
        >
          {it.label}
        </button>
      ))}
    </div>
  );
}
