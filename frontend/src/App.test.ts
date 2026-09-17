import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import App from './App.vue'

afterEach(() => vi.useRealTimers())

describe('заставка приложения', () => {
  it('показывается при старте и автоматически освобождает интерфейс', async () => {
    vi.useFakeTimers()
    const wrapper = mount(App, { global: { mocks: { $route: { fullPath: '/' } }, stubs: { RouterLink: true, RouterView: true } } })
    expect(wrapper.find('.splash-screen').exists()).toBe(true)
    expect(wrapper.get('.splash-brand').text()).toContain('PoDolyam ↗')
    vi.advanceTimersByTime(1200)
    await nextTick()
    expect(wrapper.find('.splash-screen').exists()).toBe(false)
    wrapper.unmount()
  })
})