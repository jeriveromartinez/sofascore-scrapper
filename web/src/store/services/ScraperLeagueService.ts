// ScraperLeagueService
//
// HTTP client for the catalog admin endpoints that manage scraper leagues
// (e.g. FotMob/Opta/Goalserve league allow-list). The backend exposes JSON
// CRUD under /api/web/v1/scraper-leagues (the admin middleware is composed
// on top of that group in the router, not as a path prefix).
//
// Wire format: application/json (this endpoint is intentionally not
// protobuf-backed, unlike most other catalog endpoints). We therefore use
// the JSON axios instance rather than the proto-bound helpers on
// BaseApiService. The instance is the auth-aware one (see ./axiosAuth) so
// 401s are handled the same way as the protobuf services.

import { readAuthStorage } from '../authStorage'
// F4 (PR #123): route every request through the auth-aware JSON axios
// instance so 401s refresh the bearer token transparently. Without this,
// the access-token expiry (1h) breaks the admin page silently while
// the refresh token (7d) is still valid.
import { authJsonAxios } from './axiosAuth'

export interface ScraperLeague {
  id: number
  source: string
  source_league_id: string
  name: string
  country: string
  sport: string
  enabled: boolean
  override_source: string | null
  created_at: string
  updated_at: string
}

export interface ListFilters {
  enabled?: boolean
  source?: string
  country?: string
  q?: string
  page: number
  limit: number
}

export interface ScraperLeagueSearchResult {
  source: string
  source_league_id: string
  name: string
  country: string
  sport: string
}

/**
 * Normalize a single ScraperLeague payload from the wire to the camelCase /
 * snake_case shape the rest of the frontend expects.
 *
 * Background: the Go model `catalog.ScraperLeague` embeds `gorm.Model` and
 * now emits explicit snake_case `json:` tags (`json:"source"`,
 * `json:"source_league_id"`, …), so the wire payload already uses the field
 * names this helper returns.
 *
 * This helper accepts BOTH casings so it keeps tolerating the legacy
 * PascalCase casing (the Go field names `ID`, `Source`, `SourceLeagueId`,
 * `CreatedAt`, `UpdatedAt`) that the payload carried before those tags were
 * added.
 */
export function normalizeScraperLeague(raw: unknown): ScraperLeague {
  const r = (raw ?? {}) as Record<string, unknown>
  return {
    id: (r.ID ?? r.id) as number,
    source: (r.Source ?? r.source) as string,
    source_league_id: (r.SourceLeagueId ?? r.source_league_id) as string,
    name: (r.Name ?? r.name) as string,
    country: (r.Country ?? r.country) as string,
    sport: (r.Sport ?? r.sport) as string,
    enabled: (r.Enabled ?? r.enabled) as boolean,
    override_source: (r.OverrideSource ?? r.override_source ?? null) as string | null,
    created_at: (r.CreatedAt ?? r.created_at) as string,
    updated_at: (r.UpdatedAt ?? r.updated_at) as string,
  }
}

/**
 * Normalize a search hit from the wire.
 *
 * `scraper.LeagueSearchResult` carries no JSON tags, so the backend
 * serializes it with Go field names (`Source`, `SourceLeagueId`, `Name`,
 * `Country`, `Sport`). Both casings are accepted so the service keeps
 * working once the upstream tags are added.
 */
export function normalizeScraperLeagueSearchResult(raw: unknown): ScraperLeagueSearchResult {
  const r = (raw ?? {}) as Record<string, unknown>
  return {
    source: (r.Source ?? r.source) as string,
    source_league_id: (r.SourceLeagueId ?? r.source_league_id) as string,
    name: (r.Name ?? r.name) as string,
    country: (r.Country ?? r.country) as string,
    sport: (r.Sport ?? r.sport) as string,
  }
}

export class ScraperLeagueService {
  // Relative to the axios instance baseURL (API_BASE_URL): an absolute
  // path here would be prefixed a second time by axios and produce
  // "/api/web/v1/api/web/v1/scraper-leagues".
  readonly base = '/scraper-leagues'

  // Catalog routes are gated by `adminThenRl` middleware on the backend, so every
  // request must carry a Bearer token. We read the token straight from the
  // shared authStorage instead of extending BaseApiService (the catalog endpoints
  // return plain JSON, not protobuf, so the proto-bound parent does not fit).
  private getAuthHeaders(): Record<string, string> {
    const token = readAuthStorage().user?.token ?? ''
    return { Authorization: `Bearer ${token}` }
  }

  async list(filters: ListFilters): Promise<{ data: ScraperLeague[]; total: number; page: number; limit: number }> {
    const params: Record<string, string> = {
      page: String(filters.page),
      limit: String(filters.limit),
    }
    if (filters.enabled !== undefined) params.enabled = String(filters.enabled)
    if (filters.source) params.source = filters.source
    if (filters.country) params.country = filters.country
    if (filters.q) params.q = filters.q
    const res = await authJsonAxios.get(this.base, { params, headers: this.getAuthHeaders() })
    const payload = (res.data ?? {}) as { data?: unknown[]; total?: number; page?: number; limit?: number }
    return {
      data: Array.isArray(payload.data) ? payload.data.map(normalizeScraperLeague) : [],
      total: payload.total ?? 0,
      page: payload.page ?? filters.page,
      limit: payload.limit ?? filters.limit,
    }
  }

  async get(id: number): Promise<ScraperLeague> {
    const res = await authJsonAxios.get(`${this.base}/${id}`, { headers: this.getAuthHeaders() })
    return normalizeScraperLeague(res.data)
  }

  async create(payload: Partial<ScraperLeague>): Promise<ScraperLeague> {
    const res = await authJsonAxios.post(this.base, payload, { headers: this.getAuthHeaders() })
    return normalizeScraperLeague(res.data)
  }

  async update(id: number, fields: Partial<ScraperLeague>): Promise<void> {
    await authJsonAxios.patch(`${this.base}/${id}`, fields, { headers: this.getAuthHeaders() })
  }

  async delete(id: number): Promise<void> {
    await authJsonAxios.delete(`${this.base}/${id}`, { headers: this.getAuthHeaders() })
  }

  async search(q: string): Promise<ScraperLeagueSearchResult[]> {
    const res = await authJsonAxios.get(`${this.base}/search`, { params: { q }, headers: this.getAuthHeaders() })
    const payload = (res.data ?? {}) as { data?: unknown }
    // The backend answers {"data": null} when the source has no match.
    return Array.isArray(payload.data)
      ? payload.data.map(normalizeScraperLeagueSearchResult)
      : []
  }
}
