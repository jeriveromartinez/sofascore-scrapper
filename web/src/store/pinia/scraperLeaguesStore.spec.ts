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
})
