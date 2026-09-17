import { mount, flushPromises } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App.vue'
import { routes } from './router'

const request = vi.fn(async (_method: string, path: string) => {
  if (path === '/api/meetings') return '[]'
  if (path === '/api/profile') return JSON.stringify({ name: 'Женя', phone: '+7 900', bank: 'Банк' })
  if (path === '/api/friends') return '[]'
  throw new Error('Неожиданный запрос: ' + path)
})

describe('Навигация', () => {
  beforeEach(() => {
    window.go = { main: { App: { Request: request, CopyText: vi.fn(), ExportParticipantPDF: vi.fn() } } }
  })
  afterEach(() => {
    delete window.go
    vi.clearAllMocks()
  })

  it('позволяет вернуться с неизвестного адреса на главную', async () => {
    const router = createRouter({ history: createMemoryHistory(), routes })
    await router.push('/missing')
    await router.isReady()
    const wrapper = mount(App, { global: { plugins: [router] } })
    expect(wrapper.find('h1').text()).toBe('Страница не найдена')
    await wrapper.get('main a').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/')
    expect(wrapper.find('h1').text()).toBe('Мои встречи')
    expect(wrapper.find('.main-tabs').exists()).toBe(true)
    wrapper.unmount()
  })
})
