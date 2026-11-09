import { describe, expect, it } from "vitest";
import { cx } from "./utils";
import { variants } from "./variants";

describe("cx", () => {
  it("joins class names like clsx", () => {
    expect(cx("a", false, null, undefined, 0, "", true, "b")).toBe("a b");
    expect(cx(["a", ["b", { c: true, d: false }]], { e: 1, f: 0 }, 7)).toBe("a b c e 7");
    expect(cx()).toBe("");
  });
});

describe("variants", () => {
  const button = variants("base", {
    variants: { variant: { solid: "v-solid", ghost: "v-ghost" }, size: { sm: "s-sm", lg: "s-lg" } },
    defaultVariants: { variant: "solid", size: "sm" },
  });

  it("uses the defaults when a variant is not set", () => {
    expect(button()).toBe("base v-solid s-sm");
    expect(button({ variant: undefined })).toBe("base v-solid s-sm");
  });

  it("uses the chosen variant, and null turns it off", () => {
    expect(button({ variant: "ghost", size: "lg" })).toBe("base v-ghost s-lg");
    expect(button({ size: null })).toBe("base v-solid");
  });
});
