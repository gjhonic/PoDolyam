// Desktop-сборка обращается напрямую к Go через Wails. Сетевой сервер не нужен.
export const isDesktop = import.meta.env.VITE_DESKTOP === 'true'
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
  if (isDesktop) await desktop().CopyText(text)
  else await navigator.clipboard.writeText(text)
}
