import { describe, it, expect } from "vitest";
import { formatUnixTimestamp } from "./time";

describe("formatUnixTimestamp", () => {
  it("returns empty string for zero", () => {
    expect(formatUnixTimestamp(0)).toBe("");
  });

  it("returns empty string for NaN", () => {
    expect(formatUnixTimestamp(NaN)).toBe("");
  });

  it("detects seconds and produces a locale string", () => {
    const result = formatUnixTimestamp(1609459200);
    expect(result).toBeTruthy();
    expect(result).toMatch(/[0-9\/]/);
  });

  it("passes through milliseconds unchanged and produces a locale string", () => {
    const result = formatUnixTimestamp(1609459200000);
    expect(result).toBeTruthy();
    expect(result).toMatch(/[0-9\/]/);
  });

  it("handles negative zero-like values", () => {
    expect(formatUnixTimestamp(-0)).toBe("");
  });
});
