// ScraperLeagueService
//
// HTTP client for the catalog admin endpoints that manage scraper leagues
// (e.g. FotMob/Opta/Goalserve league allow-list). The backend exposes JSON
// CRUD under /api/web/v1/scraper-leagues (the admin middleware is composed
// on top of that group in the router, not as a path prefix).
//
// Wire format: application/json (this endpoint is intentionally not
// protobuf-backed, unlike most other catalog endpoints). We therefore use
// the top-level axios client rather than the proto-bound helpers on
// BaseApiService.

import axios from 'axios'
import { API_BASE_URL } from '../../constants'
import { readAuthStorage } from '../authStorage'

export interface ScraperLeague {
  id: number
  source: string
  source_league_id: string
  name: string
  country: string
  sport: string
  enabled: boolean
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

export class ScraperLeagueService {
  private readonly base = `${API_BASE_URL}/scraper-leagues`

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
    const res = await axios.get(this.base, { params, headers: this.getAuthHeaders() })
    return res.data
  }

  async get(id: number): Promise<ScraperLeague> {
    const res = await axios.get(`${this.base}/${id}`, { headers: this.getAuthHeaders() })
    return res.data
  }

  async create(payload: Partial<ScraperLeague>): Promise<ScraperLeague> {
    const res = await axios.post(this.base, payload, { headers: this.getAuthHeaders() })
    return res.data
  }

  async update(id: number, fields: Partial<ScraperLeague>): Promise<void> {
    await axios.patch(`${this.base}/${id}`, fields, { headers: this.getAuthHeaders() })
  }

  async delete(id: number): Promise<void> {
    await axios.delete(`${this.base}/${id}`, { headers: this.getAuthHeaders() })
  }

  async search(q: string): Promise<ScraperLeagueSearchResult[]> {
    const res = await axios.get(`${this.base}/search`, { params: { q }, headers: this.getAuthHeaders() })
    return res.data.data as ScraperLeagueSearchResult[]
  }
}
