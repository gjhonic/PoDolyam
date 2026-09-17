import { mount, flushPromises } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App.vue'
import { routes } from './router'

describe('Навигация', () => {
  beforeEach(() => { vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => ({ csrf: 'test', user: null }) })) })
  afterEach(() => { vi.unstubAllGlobals() })
  it('позволяет вернуться с неизвестного адреса на главную', async () => {
    const router = createRouter({ history: createMemoryHistory(), routes })
    await router.push('/missing')
    await router.isReady()
    const wrapper = mount(App, { global: { plugins: [router] } })
    expect(wrapper.find('h1').text()).toBe('Страница не найдена')
    await wrapper.get('main a').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/')
    expect(wrapper.find('h1').text()).toContain('Разделим счёт')
    expect(wrapper.find('input[type=email]').exists()).toBe(true)
    wrapper.unmount()
  })
})
