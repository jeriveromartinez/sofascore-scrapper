import { describe, it, expect, vi, beforeEach } from 'vitest'
import axios from 'axios'
import { ScraperLeagueService } from './ScraperLeagueService'

vi.mock('axios')

describe('ScraperLeagueService', () => {
  beforeEach(() => {
    const mockedAxios = axios as any
    mockedAxios.get.mockReset()
    mockedAxios.post.mockReset()
    mockedAxios.patch.mockReset()
    mockedAxios.delete.mockReset()
  })
  it('list calls GET /scraper-leagues', async () => {
    const mockedAxios = axios as any
    mockedAxios.get.mockResolvedValue({ data: { data: [], total: 0 } })
    const svc = new ScraperLeagueService()
    const result = await svc.list({ page: 1, limit: 50 })
    expect(mockedAxios.get).toHaveBeenCalledTimes(1)
    expect(mockedAxios.get.mock.calls[0][0]).toContain('/scraper-leagues')
    expect(mockedAxios.get.mock.calls[0][1]).toEqual(
      expect.objectContaining({ params: expect.objectContaining({ page: '1', limit: '50' }) })
    )
    expect(result.total).toBe(0)
  })

  it('create posts to /scraper-leagues', async () => {
    const mockedAxios = axios as any
    mockedAxios.post.mockResolvedValue({ data: { id: 1, name: 'PL' } })
    const svc = new ScraperLeagueService()
    await svc.create({ source: 'fotmob', source_league_id: '47', name: 'PL' } as any)
    expect(mockedAxios.post).toHaveBeenCalledTimes(1)
    expect(mockedAxios.post.mock.calls[0][0]).toContain('/scraper-leagues')
    expect(mockedAxios.post.mock.calls[0][1]).toEqual(
      expect.objectContaining({ name: 'PL' })
    )
  })

  it('update patches /scraper-leagues/:id', async () => {
    const mockedAxios = axios as any
    mockedAxios.patch.mockResolvedValue({ data: { ok: true } })
    const svc = new ScraperLeagueService()
    await svc.update(7, { name: 'PL' })
    expect(mockedAxios.patch).toHaveBeenCalledTimes(1)
    expect(mockedAxios.patch.mock.calls[0][0]).toMatch(/\/scraper-leagues\/7$/)
    expect(mockedAxios.patch.mock.calls[0][1]).toEqual(
      expect.objectContaining({ name: 'PL' })
    )
  })

  it('delete calls DELETE /scraper-leagues/:id', async () => {
    const mockedAxios = axios as any
    mockedAxios.delete.mockResolvedValue({ data: undefined })
    const svc = new ScraperLeagueService()
    await svc.delete(9)
    expect(mockedAxios.delete).toHaveBeenCalledTimes(1)
    expect(mockedAxios.delete.mock.calls[0][0]).toMatch(/\/scraper-leagues\/9$/)
  })

  it('search calls GET /scraper-leagues/search with q param', async () => {
    const mockedAxios = axios as any
    mockedAxios.get.mockResolvedValue({
      data: {
        data: [
          {
            source_league_id: '47',
            name: 'PL',
            country: 'England',
            sport: 'Soccer',
          },
        ],
      },
    })
    const svc = new ScraperLeagueService()
    const result = await svc.search('premier')
    expect(mockedAxios.get).toHaveBeenCalledTimes(1)
    expect(mockedAxios.get.mock.calls[0][0]).toContain('/scraper-leagues/search')
    expect(mockedAxios.get.mock.calls[0][1]).toEqual(
      expect.objectContaining({
        params: expect.objectContaining({ q: 'premier' }),
      })
    )
    expect(result).toHaveLength(1)
    expect(result[0]!.name).toBe('PL')
  })

  // --- F2 (PR #123) -------------------------------------------------------
  // The backend's catalog.ScraperLeague embeds gorm.Model without explicit
  // JSON tags, so the wire format uses Go's default field naming
  // (`ID`, `Source`, `SourceLeagueId`, `CreatedAt`, `UpdatedAt`). The
  // frontend types use snake_case lowercase. Without normalization the
  // response is silently unusable (item.id is undefined and the UI
  // builds URLs like `/scraper-leagues/undefined`).

  const gormCasedItem = {
    ID: 7,
    Source: 'fotmob',
    SourceLeagueId: '47',
    Name: 'Premier League',
    Country: 'GB',
    Sport: 'football',
    Enabled: true,
    CreatedAt: '2026-09-10T10:00:00Z',
    UpdatedAt: '2026-09-10T11:00:00Z',
  }

  it('list normalizes gorm.Model-style PascalCase fields to snake_case', async () => {
    const mockedAxios = axios as any
    mockedAxios.get.mockResolvedValue({ data: { data: [gormCasedItem], total: 1 } })
    const svc = new ScraperLeagueService()
    const result = await svc.list({ page: 1, limit: 50 })
    expect(result.data).toHaveLength(1)
    expect(result.data[0]).toEqual({
      id: 7,
      source: 'fotmob',
      source_league_id: '47',
      name: 'Premier League',
      country: 'GB',
      sport: 'football',
      enabled: true,
      created_at: '2026-09-10T10:00:00Z',
      updated_at: '2026-09-10T11:00:00Z',
    })
  })

  it('get normalizes gorm.Model-style PascalCase fields to snake_case', async () => {
    const mockedAxios = axios as any
    mockedAxios.get.mockResolvedValue({ data: gormCasedItem })
    const svc = new ScraperLeagueService()
    const item = await svc.get(7)
    expect(item.id).toBe(7)
    expect(item.source_league_id).toBe('47')
    expect(item.created_at).toBe('2026-09-10T10:00:00Z')
  })

  it('create normalizes gorm.Model-style PascalCase fields to snake_case', async () => {
    const mockedAxios = axios as any
    mockedAxios.post.mockResolvedValue({ data: gormCasedItem })
    const svc = new ScraperLeagueService()
    const item = await svc.create({ source: 'fotmob', source_league_id: '47' } as any)
    expect(item.id).toBe(7)
    expect(item.name).toBe('Premier League')
  })

  it('list preserves already-snake_cased responses (no double normalization regression)', async () => {
    const mockedAxios = axios as any
    mockedAxios.get.mockResolvedValue({
      data: {
        data: [{
          id: 3,
          source: 'fotmob',
          source_league_id: '47',
          name: 'PL',
          country: 'GB',
          sport: 'football',
          enabled: true,
          created_at: '2026-09-10T10:00:00Z',
          updated_at: '2026-09-10T11:00:00Z',
        }],
        total: 1,
      },
    })
    const svc = new ScraperLeagueService()
    const result = await svc.list({ page: 1, limit: 50 })
    expect(result.data[0].id).toBe(3)
    expect(result.data[0].created_at).toBe('2026-09-10T10:00:00Z')
  })
})
