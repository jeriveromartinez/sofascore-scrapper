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

  it('submits the source of the selected suggestion', async () => {
    // A league id only means something inside the source that published it,
    // so picking a scores365 suggestion must not create a fotmob row.
    const store = useScraperLeaguesStore()
    const svc = new ScraperLeagueService() as any
    svc.search.mockResolvedValue([
      { source: 'scores365', source_league_id: '9001', name: 'La Liga', country: 'ES', sport: 'football' },
    ])
    svc.create.mockResolvedValue({ id: 2, name: 'La Liga' })
    store.setService(svc)

    const wrapper = mount(CreateOrEdit, { props: { item: null } })
    await wrapper.find('input.search').setValue('liga')
    await flushPromises()
    await new Promise(r => setTimeout(r, 300))  // debounce
    await flushPromises()
    await wrapper.find('.suggestion').trigger('click')
    await wrapper.find('button.submit').trigger('click')
    await flushPromises()

    expect(svc.create).toHaveBeenCalledWith(expect.objectContaining({
      source: 'scores365', source_league_id: '9001',
    }))
  })

  it('renders the modal root with the .modal-visible opt-in class', () => {
    // F1 (PR #123): assets/vendor/css/core.css forces `.modal { display: none }`
    // for the Bootstrap modal lifecycle. Because we never add the `.show` class
    // (no Bootstrap JS), the modal content would stay invisible without a
    // scoped override. The component ships `.modal-visible` to opt back in.
    const wrapper = mount(CreateOrEdit, { props: { item: null } })
    const modal = wrapper.find('.modal-visible')
    expect(modal.exists()).toBe(true)
  })

  it('handles a null search response without crashing (F3 guard)', async () => {
    // F3 (PR #123): the backend's FotMob /search endpoint returns
    //   {"data": null}   (the source returns nil when nothing matches).
    // The store currently hands that null straight through, so the template
    // tried `suggestions.length > 0` and threw TypeError. The component
    // must normalize null -> [] before assigning to `suggestions`.
    const store = useScraperLeaguesStore()
    const svc = new ScraperLeagueService() as any
    svc.search.mockResolvedValue(null)
    store.setService(svc)

    const wrapper = mount(CreateOrEdit, { props: { item: null } })
    await wrapper.find('input.search').setValue('premier')
    await flushPromises()
    await new Promise(r => setTimeout(r, 300))  // debounce
    await flushPromises()

    // No suggestions rendered, no crash. The next render must not throw.
    expect(wrapper.findAll('.suggestion')).toHaveLength(0)
  })

  it('prefills the form when the item prop changes', async () => {
    // The modal is reused between "add new" and "edit": reading the prop
    // only on mount leaves the previous league's values in the form.
    const wrapper = mount(CreateOrEdit, { props: { item: null } })
    await wrapper.setProps({
      item: {
        id: 7,
        source: 'fotmob',
        source_league_id: '47',
        name: 'Premier League',
        country: 'GB',
        sport: 'football',
        enabled: false,
        override_source: 'fotmob',
        created_at: '',
        updated_at: '',
      },
    })

    // inputs: search, name, country, sport, enabled-checkbox
    const inputs = wrapper.findAll('input')
    expect(inputs[1]!.element.value).toBe('Premier League')
    expect(inputs[2]!.element.value).toBe('GB')
    expect(inputs[3]!.element.value).toBe('football')
    expect((inputs[4]!.element as HTMLInputElement).checked).toBe(false)
    expect((wrapper.find('select').element as HTMLSelectElement).value).toBe('fotmob')
  })

  it('offers only the row\'s own source as a source override', async () => {
    // The backend rejects an override_source that names a different source
    // ("override_source must match source"), so the select must not offer a
    // foreign source at all.
    for (const source of ['fotmob', 'scores365']) {
      const wrapper = mount(CreateOrEdit, {
        props: {
          item: {
            id: 7,
            source,
            source_league_id: '47',
            name: 'Premier League',
            country: 'GB',
            sport: 'football',
            enabled: true,
            override_source: null,
            created_at: '',
            updated_at: '',
          },
        },
      })

      const options = wrapper.findAll('#override-source option')
      expect(options.map(o => o.text())).toEqual(['— use natural source —', source])
    }
  })

  it('cannot submit a source override that names a foreign source', async () => {
    const store = useScraperLeaguesStore()
    const svc = new ScraperLeagueService() as any
    svc.update.mockResolvedValue(undefined)
    svc.list.mockResolvedValue({ data: [], total: 0, page: 1, limit: 50 })
    store.setService(svc)

    const wrapper = mount(CreateOrEdit, {
      props: {
        item: {
          id: 7,
          source: 'fotmob',
          source_league_id: '47',
          name: 'Premier League',
          country: 'GB',
          sport: 'football',
          enabled: true,
          override_source: null,
          created_at: '',
          updated_at: '',
        },
      },
    })

    const select = wrapper.find('#override-source')
    ;(select.element as HTMLSelectElement).value = 'scores365'
    await select.trigger('change')
    await wrapper.find('button.submit').trigger('click')
    await flushPromises()

    expect(svc.update).toHaveBeenCalledWith(7, expect.objectContaining({ override_source: null }))
  })

  it('shows the error message and does not emit saved when the save fails', async () => {
    const store = useScraperLeaguesStore()
    const svc = new ScraperLeagueService() as any
    svc.create.mockRejectedValue(new Error('duplicate league'))
    store.setService(svc)

    const wrapper = mount(CreateOrEdit, { props: { item: null } })
    await wrapper.find('button.submit').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('duplicate league')
    expect(wrapper.emitted('saved')).toBeFalsy()
  })

  it('shows the search error instead of leaving an unhandled rejection', async () => {
    const store = useScraperLeaguesStore()
    const svc = new ScraperLeagueService() as any
    svc.search.mockRejectedValue(new Error('search down'))
    store.setService(svc)

    const wrapper = mount(CreateOrEdit, { props: { item: null } })
    await wrapper.find('input.search').setValue('premier')
    await flushPromises()
    await new Promise(r => setTimeout(r, 300))  // debounce
    await flushPromises()

    expect(wrapper.find('[role="alert"]').text()).toContain('search down')
    expect(wrapper.findAll('.suggestion')).toHaveLength(0)
  })
})
