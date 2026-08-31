import { describe, it, expect, vi, beforeEach } from "vitest";
import { BuildInfo } from "../../proto/api";
import { BuildInfoService } from "./BuildInfoService";

describe("BuildInfoService", () => {
  let service: BuildInfoService;

  beforeEach(() => {
    service = new BuildInfoService();
  });

  it("decodes a BuildInfo proto from the backend", async () => {
    const expected = BuildInfo.create({ version: "v0.0.4", commit: "a0db9ad" });
    // Stub the protected get() method on the BaseApiService.
    vi.spyOn(service as unknown as { get: (url: string, decoder: unknown) => Promise<unknown> }, "get").mockResolvedValue(expected);

    const got = await service.getBuildInfo();
    expect(got.version).toBe("v0.0.4");
    expect(got.commit).toBe("a0db9ad");
  });

  it("uses the /api/web/v1/version path", async () => {
    const expected = BuildInfo.create({ version: "v0.0.0-dev", commit: "unknown" });
    const getSpy = vi.spyOn(service as unknown as { get: (url: string, decoder: unknown) => Promise<unknown> }, "get").mockResolvedValue(expected);

    await service.getBuildInfo();
    expect(getSpy).toHaveBeenCalledWith("/version", expect.objectContaining({ decode: expect.any(Function) }));
  });
});
