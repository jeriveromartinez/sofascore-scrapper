import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
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
  created_at: '',
  updated_at: '',
}

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
})
