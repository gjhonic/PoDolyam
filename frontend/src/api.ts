import { isDesktop, desktop } from './desktop'
import { ref } from 'vue'

export const currentUser = ref<{ id: string; email: string } | null>(null)
let csrf = ''

// Сессионная cookie HttpOnly: JavaScript не читает её. В памяти хранится
// только CSRF-токен; после перезагрузки он запрашивается у сервера заново.
export async function loadSession() {
  const session = await request<{ csrf: string; user: typeof currentUser.value }>('/api/auth/session')
  csrf = session.csrf
  currentUser.value = session.user
}
export async function request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  if (isDesktop) return JSON.parse(await desktop().Request(method, path, JSON.stringify(body ?? {}))) as T
  if (method !== 'GET' && !csrf) await loadSession()
  const response = await fetch(path, {
    method,
    credentials: 'same-origin',
    headers: method === 'GET' ? {} : { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!response.ok) {
    const failure = await response.json().catch(() => null)
    throw new Error(failure?.error?.message ?? 'Не удалось связаться с сервером')
  }
  if (response.status === 204) return undefined as T
  const data = await response.json()
  if (path.startsWith('/api/auth/') && data.csrf) {
    csrf = data.csrf
    currentUser.value = data.user
  }
  return data as T
}
export function message(error: unknown): string {
  return typeof error === 'string' ? error : error instanceof Error ? error.message : 'Не удалось выполнить действие'
}
