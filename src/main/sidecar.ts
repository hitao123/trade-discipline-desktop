import { spawn, type ChildProcessWithoutNullStreams, type SpawnOptionsWithoutStdio } from 'node:child_process'
import { createInterface } from 'node:readline'

export interface SidecarHandle {
  process: ChildProcessWithoutNullStreams
  baseURL: string
  schemaVersion: number
  stop: () => Promise<void>
}

interface StartSidecarOptions {
  executable: string
  dataDir: string
  token: string
  timeoutMs?: number
  spawnImpl?: (file: string, args: readonly string[], options: SpawnOptionsWithoutStdio) => ChildProcessWithoutNullStreams
  onStderr?: (line: string) => void
  onExit?: (code: number | null, signal: NodeJS.Signals | null) => void
}

interface ReadyMessage { event: 'ready'; port: number; schemaVersion: number }

export async function startSidecar(options: StartSidecarOptions): Promise<SidecarHandle> {
  if (options.token.length < 32) throw new Error('本地会话令牌长度不足')
  const spawnImpl = options.spawnImpl ?? spawn
  const child = spawnImpl(options.executable, [], {
    env: {
      ...process.env,
      DISCIPLINE_SESSION_TOKEN: options.token,
      DISCIPLINE_DATA_DIR: options.dataDir,
    },
    stdio: 'pipe',
    windowsHide: true,
  })
  const stderr = createInterface({ input: child.stderr })
  stderr.on('line', line => options.onStderr?.(line))

  const message = await new Promise<ReadyMessage>((resolve, reject) => {
    let settled = false
    const timer = setTimeout(() => finish(new Error('本地后端启动超时')), options.timeoutMs ?? 15_000)
    const stdout = createInterface({ input: child.stdout })

    function finish(error?: Error, ready?: ReadyMessage) {
      if (settled) return
      settled = true
      clearTimeout(timer)
      stdout.close()
      if (error) reject(error)
      else if (ready) resolve(ready)
    }

    stdout.once('line', (line) => {
      try {
        const parsed = JSON.parse(line) as Partial<ReadyMessage>
        if (parsed.event !== 'ready' || !Number.isInteger(parsed.port) || (parsed.port ?? 0) < 1 || (parsed.port ?? 0) > 65_535 || !Number.isInteger(parsed.schemaVersion)) {
          finish(new Error('本地后端启动消息无效'))
          return
        }
        finish(undefined, parsed as ReadyMessage)
      }
      catch {
        finish(new Error('本地后端启动消息无效'))
      }
    })
    child.once('error', error => finish(new Error(`无法启动本地后端：${error.message}`)))
    child.once('exit', (code, signal) => {
      if (!settled) finish(new Error(`本地后端提前退出（${code ?? signal ?? 'unknown'}）`))
    })
  })

  child.on('exit', (code, signal) => options.onExit?.(code, signal))
  return {
    process: child,
    baseURL: `http://127.0.0.1:${message.port}`,
    schemaVersion: message.schemaVersion,
    stop: () => stopSidecar(child),
  }
}

async function stopSidecar(child: ChildProcessWithoutNullStreams): Promise<void> {
  if (child.exitCode !== null || child.signalCode !== null) return
  await new Promise<void>((resolve) => {
    const force = setTimeout(() => {
      child.kill('SIGKILL')
      resolve()
    }, 5_000)
    child.once('exit', () => {
      clearTimeout(force)
      resolve()
    })
    child.kill('SIGTERM')
  })
}
