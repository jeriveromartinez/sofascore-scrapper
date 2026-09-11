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
})
