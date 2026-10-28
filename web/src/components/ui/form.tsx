import type {
  InputHTMLAttributes,
  LabelHTMLAttributes,
  ReactNode,
  SelectHTMLAttributes,
  TextareaHTMLAttributes,
} from "react";
import { cn } from "@/lib/utils";

const field =
  "w-full rounded-md border border-input bg-surface px-2.5 text-sm shadow-xs placeholder:text-muted-foreground focus-visible:outline-2 focus-visible:outline-ring disabled:opacity-50";

export function Input({ className, ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return <input className={cn(field, "h-9", className)} {...props} />;
}

export function Textarea({ className, ...props }: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea className={cn(field, "min-h-20 py-2 leading-relaxed", className)} {...props} />;
}

export function Select({ className, ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return <select className={cn(field, "h-9 pr-8", className)} {...props} />;
}

export function Label({ className, ...props }: LabelHTMLAttributes<HTMLLabelElement>) {
  return <label className={cn("text-xs font-medium text-muted-foreground", className)} {...props} />;
}

export function Checkbox({
  label,
  className,
  ...props
}: Omit<InputHTMLAttributes<HTMLInputElement>, "type"> & { label: ReactNode }) {
  return (
    <label className={cn("inline-flex cursor-pointer items-center gap-2 text-sm", className)}>
      <input type="checkbox" className="size-4 accent-[var(--primary)]" {...props} />
      {label}
    </label>
  );
}

/**
 * A labelled control in a form grid. The label wraps the control, so the
 * control takes its accessible name from it without an id to keep in sync.
 */
export function Field({ label, children, className }: { label: ReactNode; children: ReactNode; className?: string }) {
  return (
    <label className={cn("flex flex-col gap-1", className)}>
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      {children}
    </label>
  );
}
