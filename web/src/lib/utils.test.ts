import { describe, expect, it } from "vitest";
import vi from "./i18n/vi";
import en from "./i18n/en";
import { fmt } from "./utils";

describe("fmt", () => {
  it("prints a dash for a missing number", () => {
    expect(fmt(undefined)).toBe("—");
    expect(fmt(Number.NaN)).toBe("—");
    expect(fmt(0.80512)).toBe("0.805");
  });
});

// Every key the Vietnamese UI has, the English one has, and the other way round.
function keys(o: object, prefix = ""): string[] {
  return Object.entries(o).flatMap(([k, v]) => (v && typeof v === "object" ? keys(v, prefix + k + ".") : [prefix + k]));
}

describe("i18n", () => {
  it("has the same keys in every language", () => {
    expect(keys(en).sort()).toEqual(keys(vi).sort());
  });
});
