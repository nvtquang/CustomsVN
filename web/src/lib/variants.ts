import { cx } from "./utils";

type Variants = Record<string, Record<string, string>>;
type Selection<V extends Variants> = { [K in keyof V]?: keyof V[K] | null };

/**
 * Class names for a component with named variants (the subset of class-variance-authority the
 * UI uses): an unset variant takes its default, and null turns the variant off.
 */
export function variants<V extends Variants>(base: string, config: { variants: V; defaultVariants?: Selection<V> }) {
  const defaults = (config.defaultVariants ?? {}) as Record<string, string | null | undefined>;
  return (props: Selection<V> = {}) => {
    const set = props as Record<string, string | null | undefined>;
    return cx(
      base,
      Object.keys(config.variants).map((k) => {
        const chosen = set[k] === null ? null : (set[k] ?? defaults[k]);
        return chosen == null ? undefined : config.variants[k][chosen];
      }),
    );
  };
}

/** The variant props a `variants()` function accepts, for a component's props type. */
export type VariantProps<F> = F extends (props?: infer P) => string ? NonNullable<P> : never;
