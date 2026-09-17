// Интерфейс обращается напрямую к Go через Wails: сетевой сервер не нужен.
interface Bridge {
  Request(method: string, path: string, body: string): Promise<string>
  CopyText(text: string): Promise<void>
  ExportParticipantPDF(id: string, participant: string): Promise<string>
}

declare global { interface Window { go?: { main: { App: Bridge } } } }

export function desktop(): Bridge {
  const bridge = window.go?.main.App
  if (!bridge) throw new Error('Откройте PoDolyam.exe: связь с приложением недоступна')
  return bridge
}

export async function copyText(text: string) {
  await desktop().CopyText(text)
}
