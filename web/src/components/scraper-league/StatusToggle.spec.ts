import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import StatusToggle from './StatusToggle.vue'

describe('StatusToggle', () => {
  it('renders ON when modelValue is true', () => {
    const wrapper = mount(StatusToggle, { props: { modelValue: true } })
    expect(wrapper.text()).toBe('ON')
    expect(wrapper.classes()).toContain('status-toggle')
    expect(wrapper.classes()).toContain('on')
    expect(wrapper.classes()).not.toContain('off')
  })

  it('renders OFF when modelValue is false', () => {
    const wrapper = mount(StatusToggle, { props: { modelValue: false } })
    expect(wrapper.text()).toBe('OFF')
    expect(wrapper.classes()).toContain('status-toggle')
    expect(wrapper.classes()).toContain('off')
    expect(wrapper.classes()).not.toContain('on')
  })

  it('clicking emits update:modelValue with the opposite boolean', async () => {
    const wrapper = mount(StatusToggle, { props: { modelValue: true } })
    await wrapper.trigger('click')

    const events = wrapper.emitted('update:modelValue')
    expect(events).toBeTruthy()
    expect(events).toHaveLength(1)
    expect(events![0]).toEqual([false])
  })

  it('clicking when OFF emits true', async () => {
    const wrapper = mount(StatusToggle, { props: { modelValue: false } })
    await wrapper.trigger('click')

    const events = wrapper.emitted('update:modelValue')
    expect(events).toBeTruthy()
    expect(events![0]).toEqual([true])
  })
})
