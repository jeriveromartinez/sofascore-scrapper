import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useScraperLeaguesStore } from './scraperLeaguesStore'
import { ScraperLeagueService } from '../services/ScraperLeagueService'

vi.mock('../services/ScraperLeagueService')

describe('scraperLeaguesStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('fetch loads items and total', async () => {
    const svc = new ScraperLeagueService() as any
    svc.list.mockResolvedValue({ data: [{ id: 1, name: 'PL' }], total: 1 })
    const store = useScraperLeaguesStore()
    store.setService(svc)
    await store.fetch({ page: 1, limit: 50 })
    expect(store.items).toHaveLength(1)
    expect(store.total).toBe(1)
    expect(store.loading).toBe(false)
  })

  it('create posts payload and refreshes items', async () => {
    const svc = new ScraperLeagueService() as any
    svc.create.mockResolvedValue({ id: 2, name: 'LL' })
    svc.list.mockResolvedValue({ data: [{ id: 2, name: 'LL' }], total: 1 })
    const store = useScraperLeaguesStore()
    store.setService(svc)
    const created = await store.create({ source: 'fotmob', source_league_id: '5', name: 'LL' })
    expect(created.id).toBe(2)
    expect(svc.create).toHaveBeenCalledTimes(1)
    expect(store.items).toHaveLength(1)
    expect(store.total).toBe(1)
  })

  it('remove deletes and refreshes items', async () => {
    const svc = new ScraperLeagueService() as any
    svc.list
      .mockResolvedValueOnce({ data: [{ id: 1, name: 'PL' }], total: 1 })
      .mockResolvedValueOnce({ data: [], total: 0 })
    svc.delete.mockResolvedValue(undefined)
    const store = useScraperLeaguesStore()
    store.setService(svc)
    await store.fetch({ page: 1, limit: 50 })
    expect(store.items).toHaveLength(1)
    await store.remove(1)
    expect(svc.delete).toHaveBeenCalledWith(1)
    expect(store.items).toHaveLength(0)
    expect(store.total).toBe(0)
  })

  it('ignores a stale list response that resolves after a newer one', async () => {
    // The search box fires a request per keystroke: a slow response for an
    // older term must not overwrite the rows of the newer one.
    const svc = new ScraperLeagueService() as any
    let resolveFirst: (value: unknown) => void = () => {}
    svc.list.mockImplementationOnce(
      () => new Promise((resolve) => { resolveFirst = resolve }),
    )
    svc.list.mockResolvedValueOnce({ data: [{ id: 2, name: 'NEW' }], total: 1 })
    const store = useScraperLeaguesStore()
    store.setService(svc)

    const first = store.fetch({ page: 1, limit: 50 })
    await store.fetch({ page: 1, limit: 50, q: 'prem' })
    resolveFirst({ data: [{ id: 1, name: 'OLD' }], total: 1 })
    await first

    expect(store.items[0]?.name).toBe('NEW')
    expect(store.total).toBe(1)
  })

  it('keeps the error of the latest failed request', async () => {
    const svc = new ScraperLeagueService() as any
    svc.list.mockRejectedValue(new Error('catalog unavailable'))
    const store = useScraperLeaguesStore()
    store.setService(svc)

    await store.fetch({ page: 1, limit: 50 })

    expect(store.error).toBe('catalog unavailable')
    expect(store.loading).toBe(false)
  })
})
