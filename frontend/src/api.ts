import { desktop } from './desktop'

export async function request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  const data = await desktop().Request(method, path, JSON.stringify(body ?? {}))
  return JSON.parse(data) as T
}

export function message(error: unknown): string {
  return typeof error === 'string' ? error : error instanceof Error ? error.message : 'Не удалось выполнить действие'
}
