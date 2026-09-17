import { execFileSync } from 'node:child_process'
import { mkdirSync, cpSync, rmSync, writeFileSync, readdirSync, existsSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const task = process.argv[2] ?? 'build'
if (process.platform !== 'win32') throw new Error('Используйте нативные Go и Node в Windows')
if (!['install', 'test', 'build', 'run'].includes(task)) throw new Error('Команды: install, test, build, run')
const backend = path.join(root, 'backend')
const frontend = path.join(root, 'frontend')
function envValue(name) {
  for (const filename of ['.env', '.env.example']) {
    const file = path.join(root, filename)
    if (!existsSync(file)) continue
    for (const sourceLine of readFileSync(file, 'utf8').split(/\r?\n/)) {
      const line = sourceLine.trim()
      if (!line || line.startsWith('#')) continue
      const separator = line.indexOf('=')
      if (separator > 0 && line.slice(0, separator).trim() === name) return line.slice(separator + 1).trim()
    }
  }
  return undefined
}
const appVersion = envValue('APP_VERSION') ?? 'dev'
function go(args) { execFileSync('go', args, { cwd: backend, stdio: 'inherit' }) }
function npm(args, env = process.env) {
  // Команды заданы самим скриптом, пользовательский ввод в shell не передаётся.
  execFileSync('cmd.exe', ['/d', '/s', '/c', 'npm.cmd ' + args.join(' ')], { cwd: frontend, stdio: 'inherit', env })
}
if (task === 'install') {
  npm(['ci'])
  go(['mod', 'download'])
} else if (task === 'test') {
  // Проверяем исходники проекта, не затрагивая модульный кэш Go.
  function sources(dir) {
    return readdirSync(dir, { withFileTypes: true }).flatMap(entry => {
      const name = path.join(dir, entry.name)
      return entry.isDirectory() ? sources(name) : entry.name.endsWith('.go') ? [name] : []
    })
  }
  const unformatted = execFileSync('gofmt', ['-l', ...sources(backend)], { encoding: 'utf8' }).trim()
  if (unformatted) throw new Error('Запустите gofmt для файлов:\n' + unformatted)
  go(['vet', './...'])
  go(['test', './...'])
  for (const target of ['FuzzAllocate', 'FuzzCalculate', 'FuzzInvalidNumbers']) {
    go(['test', './internal/money', '-run=^$', '-fuzz=^' + target + '$', '-fuzztime=5s', '-parallel=2'])
  }
  npm(['run', 'typecheck'])
  npm(['run', 'lint'])
  npm(['test'])
} else {
  npm(['run', 'build'], { ...process.env, VITE_DESKTOP: 'true', VITE_APP_VERSION: appVersion })
  const target = path.resolve(backend, 'cmd/desktop/ui')
  const expected = path.resolve(backend, 'cmd/desktop')
  // Удаляется исключительно генерируемая копия UI внутри текущего проекта.
  if (!target.startsWith(expected + path.sep) || path.basename(target) !== 'ui') throw new Error('Недопустимый путь UI')
  rmSync(target, { recursive: true, force: true })
  mkdirSync(target, { recursive: true })
  writeFileSync(path.join(target, 'README.txt'), 'Содержимое UI генерируется scripts/windows.cmd build.\n')
  cpSync(path.join(frontend, 'dist'), target, { recursive: true })
  mkdirSync(path.join(root, 'build'), { recursive: true })
  go(['build', '-trimpath', '-tags', 'desktop,production', '-ldflags', '-H windowsgui', '-o', '../build/PoDolyam.exe', './cmd/desktop'])
  console.log('Готово: ' + path.join(root, 'build/PoDolyam.exe'))
  if (task === 'run') execFileSync(path.join(root, 'build/PoDolyam.exe'), [], { stdio: 'inherit' })
}
