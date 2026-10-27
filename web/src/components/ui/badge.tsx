import { cva, type VariantProps } from "class-variance-authority";
import type { HTMLAttributes } from "react";
import { cn } from "@/lib/utils";

const badgeVariants = cva(
  "inline-flex items-center gap-1 rounded-sm px-1.5 py-0.5 text-2xs font-medium whitespace-nowrap",
  {
    variants: {
      variant: {
        default: "bg-muted text-foreground",
        accent: "bg-accent text-accent-foreground",
        outline: "border text-muted-foreground",
        in_force: "bg-force-in-bg text-force-in",
        partly_expired: "bg-force-partly-bg text-force-partly",
        not_yet_in_force: "bg-force-notyet-bg text-force-notyet",
        expired: "bg-force-expired-bg text-force-expired",
        unknown: "bg-force-unknown-bg text-force-unknown",
      },
    },
    defaultVariants: { variant: "default" },
  },
);

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement>, VariantProps<typeof badgeVariants> {}

export function Badge({ className, variant, ...props }: BadgeProps) {
  return <span className={cn(badgeVariants({ variant }), className)} {...props} />;
}
