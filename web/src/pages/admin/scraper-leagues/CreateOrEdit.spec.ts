import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import CreateOrEdit from './CreateOrEdit.vue'
import { useScraperLeaguesStore } from '../../../store/pinia/scraperLeaguesStore'
import { ScraperLeagueService } from '../../../store/services/ScraperLeagueService'

// vue-i18n is NOT installed in this project, so we mount without the i18n
// plugin and use plain English string literals in the template (matches the
// repo-wide convention).
vi.mock('../../../store/services/ScraperLeagueService')

describe('CreateOrEdit', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('autocomplete search calls service.search', async () => {
    const store = useScraperLeaguesStore()
    const svc = new ScraperLeagueService() as any
    svc.search.mockResolvedValue([{ source_league_id: '47', name: 'Premier League', country: 'GB', sport: 'football' }])
    store.setService(svc)

    const wrapper = mount(CreateOrEdit, { props: { item: null } })
    await wrapper.find('input.search').setValue('premier')
    await flushPromises()
    await new Promise(r => setTimeout(r, 300))  // debounce
    await flushPromises()

    expect(svc.search).toHaveBeenCalledWith('premier')
    expect(wrapper.text()).toContain('Premier League')
  })

  it('submits a new league with selected suggestion', async () => {
    const store = useScraperLeaguesStore()
    const svc = new ScraperLeagueService() as any
    svc.search.mockResolvedValue([{ source_league_id: '47', name: 'Premier League', country: 'GB', sport: 'football' }])
    svc.create.mockResolvedValue({ id: 1, name: 'Premier League' })
    store.setService(svc)

    const wrapper = mount(CreateOrEdit, { props: { item: null } })
    await wrapper.find('input.search').setValue('premier')
    await flushPromises()
    await new Promise(r => setTimeout(r, 300))
    await flushPromises()
    await wrapper.find('.suggestion').trigger('click')
    await wrapper.find('button.submit').trigger('click')
    await flushPromises()

    expect(svc.create).toHaveBeenCalledWith(expect.objectContaining({
      source: 'fotmob', source_league_id: '47', name: 'Premier League', country: 'GB', sport: 'football',
    }))
  })
})
