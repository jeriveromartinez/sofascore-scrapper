import { describe, expect, it } from "vitest";
import { API_BASE_URL } from "../../constants";
import { authJsonAxios } from "./axiosAuth";
import { ScraperLeagueService } from "./ScraperLeagueService";

// This spec deliberately does NOT mock ./axiosAuth: the point is to check
// the URL the auth-aware instance actually builds. The instance already
// carries API_BASE_URL as its baseURL, so the service must pass a relative
// path — otherwise axios prefixes the base a second time and every request
// lands on /api/web/v1/api/web/v1/scraper-leagues.
describe("ScraperLeagueService request URL", () => {
  it("resolves the list path under the API base URL exactly once", () => {
    const svc = new ScraperLeagueService();
    expect(authJsonAxios.getUri({ url: svc.base })).toBe(
      `${API_BASE_URL}/scraper-leagues`,
    );
  });

  it("resolves the item and search paths under the API base URL exactly once", () => {
    const svc = new ScraperLeagueService();
    expect(authJsonAxios.getUri({ url: `${svc.base}/7` })).toBe(
      `${API_BASE_URL}/scraper-leagues/7`,
    );
    expect(authJsonAxios.getUri({ url: `${svc.base}/search` })).toBe(
      `${API_BASE_URL}/scraper-leagues/search`,
    );
  });
});
