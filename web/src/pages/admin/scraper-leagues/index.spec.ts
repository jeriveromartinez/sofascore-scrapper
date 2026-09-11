import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import Index from './index.vue'
import { useScraperLeaguesStore } from '../../../store/pinia/scraperLeaguesStore'
import { ScraperLeagueService } from '../../../store/services/ScraperLeagueService'

// Auto-mock the service so svc.list / svc.create / etc. are vi.fn() instances
// and can be stubbed per-test. Same pattern used in scraperLeaguesStore.spec.ts.
vi.mock('../../../store/services/ScraperLeagueService')

const sampleItem = {
  id: 1,
  name: 'Premier League',
  country: 'GB',
  source: 'fotmob',
  source_league_id: '47',
  sport: 'football',
  enabled: true,
  created_at: '',
  updated_at: '',
}

describe('ScraperLeagues index', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('renders table rows from store', async () => {
    const store = useScraperLeaguesStore()
    const svc = new ScraperLeagueService() as any
    svc.list.mockResolvedValue({ data: [sampleItem], total: 1 })
    store.setService(svc)
    await store.fetch()

    const wrapper = mount(Index)
    expect(wrapper.text()).toContain('Premier League')
  })

  it('clicking "Add new" opens CreateOrEdit modal in create mode', async () => {
    const store = useScraperLeaguesStore()
    const svc = new ScraperLeagueService() as any
    svc.list.mockResolvedValue({ data: [], total: 0 })
    store.setService(svc)
    await store.fetch()

    const wrapper = mount(Index)
    await flushPromises()
    await wrapper.find('button.add-new').trigger('click')
    await flushPromises()

    const h2 = wrapper.find('.modal h2')
    expect(h2.exists()).toBe(true)
    expect(h2.text()).toBe('Add new league')
  })

  it('clicking "Edit" on a row opens CreateOrEdit modal in edit mode', async () => {
    const store = useScraperLeaguesStore()
    const svc = new ScraperLeagueService() as any
    svc.list.mockResolvedValue({ data: [sampleItem], total: 1 })
    store.setService(svc)
    await store.fetch()

    const wrapper = mount(Index)
    await flushPromises()
    await wrapper.find('button.edit').trigger('click')
    await flushPromises()

    const h2 = wrapper.find('.modal h2')
    expect(h2.exists()).toBe(true)
    expect(h2.text()).toBe('Edit league')
  })

  it('clicking "Delete" on a row opens ConfirmDelete modal', async () => {
    const store = useScraperLeaguesStore()
    const svc = new ScraperLeagueService() as any
    svc.list.mockResolvedValue({ data: [sampleItem], total: 1 })
    store.setService(svc)
    await store.fetch()

    const wrapper = mount(Index)
    await flushPromises()
    await wrapper.find('button.delete').trigger('click')
    await flushPromises()

    expect(wrapper.find('.modal-backdrop').exists()).toBe(true)
    expect(wrapper.text()).toContain("'Premier League'")
  })

  it('clicking Cancel in ConfirmDelete closes the modal without calling svc.delete', async () => {
    const store = useScraperLeaguesStore()
    const svc = new ScraperLeagueService() as any
    svc.list.mockResolvedValue({ data: [sampleItem], total: 1 })
    store.setService(svc)
    await store.fetch()

    const wrapper = mount(Index)
    await flushPromises()
    await wrapper.find('button.delete').trigger('click')
    await flushPromises()
    // Cancel is the first button in the ConfirmDelete modal (Delete is the .danger second one)
    const buttons = wrapper.findAll('.modal-backdrop button')
    const cancelBtn = buttons[0]
    expect(cancelBtn).toBeDefined()
    await cancelBtn!.trigger('click')
    await flushPromises()

    expect(svc.delete).not.toHaveBeenCalled()
    expect(wrapper.find('.modal-backdrop').exists()).toBe(false)
  })

  it('clicking Delete in ConfirmDelete calls svc.delete once with the right id', async () => {
    const store = useScraperLeaguesStore()
    const svc = new ScraperLeagueService() as any
    svc.list.mockResolvedValue({ data: [sampleItem], total: 0 })
    svc.delete.mockResolvedValue(undefined)
    store.setService(svc)
    await store.fetch()

    const wrapper = mount(Index)
    await flushPromises()
    await wrapper.find('button.delete').trigger('click')
    await flushPromises()
    await wrapper.find('.modal-backdrop button.danger').trigger('click')
    await flushPromises()

    expect(svc.delete).toHaveBeenCalledTimes(1)
    expect(svc.delete).toHaveBeenCalledWith(1)
  })

  it('clicking StatusToggle calls svc.update with { enabled: !prev }', async () => {
    const store = useScraperLeaguesStore()
    const svc = new ScraperLeagueService() as any
    svc.list.mockResolvedValue({ data: [{ ...sampleItem, enabled: true }], total: 1 })
    svc.update.mockResolvedValue(undefined)
    store.setService(svc)
    await store.fetch()

    const wrapper = mount(Index)
    await flushPromises()
    await wrapper.find('.status-toggle').trigger('click')
    await flushPromises()

    expect(svc.update).toHaveBeenCalledWith(1, { enabled: false })
  })
})
