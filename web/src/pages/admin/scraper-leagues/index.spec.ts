import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import Index from './index.vue'
import { useScraperLeaguesStore } from '../../../store/pinia/scraperLeaguesStore'
import { ScraperLeagueService } from '../../../store/services/ScraperLeagueService'

// Auto-mock the service so svc.list / svc.create / etc. are vi.fn() instances
// and can be stubbed per-test. Same pattern used in scraperLeaguesStore.spec.ts.
vi.mock('../../../store/services/ScraperLeagueService')

describe('ScraperLeagues index', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('renders table rows from store', async () => {
    const store = useScraperLeaguesStore()
    const svc = new ScraperLeagueService() as any
    svc.list.mockResolvedValue({ data: [{ id: 1, name: 'Premier League', country: 'GB', source: 'fotmob', source_league_id: '47', sport: 'football', enabled: true, created_at: '', updated_at: '' }], total: 1 })
    store.setService(svc)
    await store.fetch()

    const wrapper = mount(Index)
    expect(wrapper.text()).toContain('Premier League')
  })
})
