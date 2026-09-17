import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import HomeView from './HomeView.vue'

const mocks = vi.hoisted(() => ({ request: vi.fn(), push: vi.fn() }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: mocks.push }), useRoute: () => ({ query: {} }) }))
vi.mock('../api', () => ({
  request: mocks.request,
  message: (error: unknown) => error instanceof Error ? error.message : String(error),
}))

afterEach(() => {
  document.body.innerHTML = ''
  vi.clearAllMocks()
})

describe('главная страница', () => {
  it('показывает предупреждение и удаляет встречу только после подтверждения', async () => {
    mocks.request.mockImplementation((path: string, method = 'GET') => {
      if (path === '/api/meetings' && method === 'GET') return Promise.resolve([{ id: 'meeting-1', title: 'Ужин', date: '2026-09-17', state: 'draft' }])
      if (path === '/api/profile') return Promise.resolve({ name: 'Женя', phone: '+7 900', bank: 'Банк' })
      if (path === '/api/friends') return Promise.resolve([])
      if (path === '/api/meetings/meeting-1' && method === 'DELETE') return Promise.resolve({})
      return Promise.reject(new Error('Неожиданный запрос'))
    })
    const wrapper = mount(HomeView, { attachTo: document.body, global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await flushPromises()
    await wrapper.get('.delete-meeting-button').trigger('click')
    const dialog = document.querySelector<HTMLElement>('[role="alertdialog"]')
    expect(dialog?.textContent).toContain('Ужин')
    expect(mocks.request).not.toHaveBeenCalledWith('/api/meetings/meeting-1', 'DELETE')
    const confirm = [...dialog!.querySelectorAll('button')].find(button => button.textContent?.includes('Удалить навсегда')) as HTMLButtonElement
    confirm.click()
    await flushPromises()
    expect(mocks.request).toHaveBeenCalledWith('/api/meetings/meeting-1', 'DELETE')
    expect(wrapper.text()).not.toContain('Ужин ·')
    expect(document.querySelector('[role="alertdialog"]')).toBeNull()
    wrapper.unmount()
  })

  it('добавляет друга только по имени', async () => {
    mocks.request.mockImplementation((path: string, method = 'GET', body?: unknown) => {
      if (path === '/api/meetings') return Promise.resolve([])
      if (path === '/api/profile') return Promise.resolve({ name: 'Женя', phone: '+7 900', bank: 'Банк' })
      if (path === '/api/friends' && method === 'GET') return Promise.resolve([])
      if (path === '/api/friends' && method === 'POST') return Promise.resolve({ id: 'dima', name: (body as { name: string }).name, phone: '', birthday: '', description: '' })
      return Promise.reject(new Error('Неожиданный запрос'))
    })
    const wrapper = mount(HomeView, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await flushPromises()
    await wrapper.findAll('.main-tabs button')[1]!.trigger('click')
    await wrapper.get('#friend-name').setValue('Дима')
    await wrapper.get('.friend-form').trigger('submit')
    await flushPromises()
    expect(mocks.request).toHaveBeenCalledWith('/api/friends', 'POST', { name: 'Дима' })
    expect(wrapper.text()).toContain('Карточка пока без описания')
    wrapper.unmount()
  })
})