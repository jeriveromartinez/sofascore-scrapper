import { describe, it, expect } from "vitest";
import { headerString } from "./BaseApiService";

describe("headerString", () => {
  it("returns the string value for a matching key", () => {
    expect(
      headerString({ "content-type": "application/x-protobuf" }, "content-type"),
    ).toBe("application/x-protobuf");
  });

  it("returns an empty string when the key is missing", () => {
    expect(headerString({}, "content-type")).toBe("");
    expect(headerString({ authorization: "Bearer x" }, "content-type")).toBe("");
  });

  it("falls back to an empty string for non-string header values", () => {
    expect(headerString({ "content-type": 42 }, "content-type")).toBe("");
    expect(headerString({ "content-type": true }, "content-type")).toBe("");
    expect(headerString({ "content-type": null }, "content-type")).toBe("");
    expect(headerString({ "content-type": ["a", "b"] }, "content-type")).toBe("");
  });

  it("returns an empty string when headers is null, undefined, or not an object", () => {
    expect(headerString(null, "content-type")).toBe("");
    expect(headerString(undefined, "content-type")).toBe("");
    expect(headerString("not-an-object", "content-type")).toBe("");
    expect(headerString(42, "content-type")).toBe("");
  });

  it("does not throw on an AxiosHeaders-like instance", () => {
    class FakeAxiosHeaders {
      private readonly map: Record<string, string>;
      constructor(values: Record<string, string>) {
        this.map = values;
      }
      get(name: string): string | undefined {
        return this.map[name.toLowerCase()];
      }
    }
    const headers = new FakeAxiosHeaders({ "content-type": "application/json" });
    expect(headerString(headers, "content-type")).toBe("");
  });
});
