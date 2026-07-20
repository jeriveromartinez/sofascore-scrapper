import { describe, expect, it } from "vitest";

import { API_BASE_URL } from "./constants";

describe("API_BASE_URL", () => {
  it("defaults to the same-origin web API", () => {
    expect(API_BASE_URL).toBe("/api/web/v1");
  });
});
