import { afterEach, describe, it, expect } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import ConfirmDelete from './ConfirmDelete.vue'
import type { ScraperLeague } from '../../store/pinia/scraperLeaguesStore'

const item: ScraperLeague = {
  id: 1,
  source: 'fotmob',
  source_league_id: '47',
  name: 'Premier League',
  country: 'GB',
  sport: 'football',
  enabled: true,
  override_source: null,
  created_at: '',
  updated_at: '',
}

enableAutoUnmount(afterEach)

describe('ConfirmDelete', () => {
  it("renders item.name inside the message", () => {
    const wrapper = mount(ConfirmDelete, { props: { item } })
    expect(wrapper.text()).toContain("'Premier League'")
  })

  it('clicking Cancel emits cancel and not confirm', async () => {
    const wrapper = mount(ConfirmDelete, { props: { item } })
    // Cancel is the first button in the modal (Delete is the second one, with class .danger)
    const buttons = wrapper.findAll('.modal-backdrop button')
    expect(buttons).toHaveLength(2)
    const cancelBtn = buttons[0]
    expect(cancelBtn).toBeDefined()
    await cancelBtn!.trigger('click')

    expect(wrapper.emitted('cancel')).toBeTruthy()
    expect(wrapper.emitted('cancel')).toHaveLength(1)
    expect(wrapper.emitted('confirm')).toBeFalsy()
  })

  it('clicking Delete emits confirm and not cancel', async () => {
    const wrapper = mount(ConfirmDelete, { props: { item } })
    const deleteBtn = wrapper.find('.modal-backdrop button.danger')
    expect(deleteBtn.exists()).toBe(true)
    await deleteBtn.trigger('click')

    expect(wrapper.emitted('confirm')).toBeTruthy()
    expect(wrapper.emitted('confirm')).toHaveLength(1)
    expect(wrapper.emitted('cancel')).toBeFalsy()
  })

  it('renders the modal root with the .modal-visible opt-in class', () => {
    // F1 (PR #123): assets/vendor/css/core.css forces `.modal { display: none }`.
    // Without the scoped `.modal-visible` override the modal content would be
    // hidden even when the backdrop is on screen.
    const wrapper = mount(ConfirmDelete, { props: { item } })
    const modal = wrapper.find('.modal-visible')
    expect(modal.exists()).toBe(true)
  })

  it('is a modal dialog labelled by its title', () => {
    const wrapper = mount(ConfirmDelete, { props: { item } })
    const dialog = wrapper.find('.modal')
    expect(dialog.attributes('role')).toBe('dialog')
    expect(dialog.attributes('aria-modal')).toBe('true')
    const labelledBy = dialog.attributes('aria-labelledby')
    expect(labelledBy).toBeTruthy()
    expect(wrapper.find(`[id="${labelledBy}"]`).text()).toBe('Delete league')
  })

  it('emits cancel on Escape and locks page scroll while mounted', async () => {
    const wrapper = mount(ConfirmDelete, { props: { item }, attachTo: document.body })
    expect(document.body.style.overflow).toBe('hidden')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(wrapper.emitted('cancel')).toHaveLength(1)
  })
})
