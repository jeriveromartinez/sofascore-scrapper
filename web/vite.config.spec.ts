// @vitest-environment node

import { describe, expect, it } from "vitest";

import config from "./vite.config";

describe("Vite development server", () => {
  it("proxies API requests to the default backend", () => {
    expect(config).toMatchObject({
      server: {
        proxy: {
          "/api": {
            target: "http://localhost:8080",
          },
        },
      },
    });
  });
});
