import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import FriendView from './FriendView.vue'

const mocks = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('vue-router', () => ({ useRoute: () => ({ params: { id: 'dima' } }) }))
vi.mock('../api', () => ({
  request: mocks.request,
  message: (error: unknown) => error instanceof Error ? error.message : String(error),
}))

const details = {
  friend: { id: 'dima', name: 'Дима', phone: '', birthday: '', description: 'Друг со школы' },
  stats: {
    meeting_count: 2, confirmed_count: 1, total_spent: 152168, average_spent: 152168,
    biggest_spent: 152168, outstanding: 52168,
    meetings: [
      { id: 'm2', title: 'Следующий ужин', date: '2026-10-01', venue: '', state: 'draft', amount: 152168, item_count: 6 },
      { id: 'm1', title: 'Ужин', date: '2026-09-17', venue: 'Токио-City', state: 'finalized', amount: 152168, item_count: 6 },
    ],
  },
}

afterEach(() => vi.clearAllMocks())

describe('карточка друга', () => {
  it('показывает статистику и сохраняет подробности отдельно от списка', async () => {
    mocks.request.mockImplementation((path: string, method = 'GET', body?: unknown) => {
      if (path === '/api/friends/dima' && method === 'GET') return Promise.resolve(structuredClone(details))
      if (path === '/api/friends/dima' && method === 'PUT') return Promise.resolve(body)
      return Promise.reject(new Error('Неожиданный запрос'))
    })
    const wrapper = mount(FriendView, { global: { stubs: { RouterLink: { props: ['to'], template: '<a><slot /></a>' } } } })
    await flushPromises()

    expect(wrapper.get('h1').text()).toBe('Дима')
    expect(wrapper.text()).toContain('Ужин')
    expect(wrapper.findAll('.spending-row')).toHaveLength(1)
    expect(wrapper.text()).toContain('2 встреч')

    await wrapper.get('#detail-description').setValue('Любит рамен')
    await wrapper.get('.friend-details-form').trigger('submit')
    await flushPromises()

    expect(mocks.request).toHaveBeenCalledWith('/api/friends/dima', 'PUT', expect.objectContaining({ description: 'Любит рамен' }))
    expect(wrapper.text()).toContain('Карточка друга сохранена')
  })
})