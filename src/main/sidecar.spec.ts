// @vitest-environment node
import { EventEmitter } from 'node:events'
import { PassThrough } from 'node:stream'
import type { SpawnOptionsWithoutStdio } from 'node:child_process'
import { describe, expect, it, vi } from 'vitest'

import { startSidecar } from './sidecar'

function fakeChild(line: string) {
  const child = new EventEmitter() as EventEmitter & { stdout: PassThrough; stderr: PassThrough; kill: ReturnType<typeof vi.fn>; pid: number }
  child.stdout = new PassThrough()
  child.stderr = new PassThrough()
  child.kill = vi.fn()
  child.pid = 4321
  queueMicrotask(() => child.stdout.write(line))
  return child
}

describe('startSidecar', () => {
  it('accepts one valid ready line and keeps the token out of argv', async () => {
    const child = fakeChild('{"event":"ready","port":43123,"schemaVersion":1}\n')
    const spawnImpl = vi.fn(() => child as never)
    const handle = await startSidecar({ executable: '/app/bin/server', dataDir: '/tmp/discipline-test', token: 's'.repeat(64), spawnImpl, timeoutMs: 1000 })
    expect(handle.baseURL).toBe('http://127.0.0.1:43123')
    const [, args, options] = spawnImpl.mock.calls[0] as unknown as [string, readonly string[], SpawnOptionsWithoutStdio]
    expect(args).toEqual([])
    expect(JSON.stringify(args)).not.toContain('s'.repeat(64))
    expect(options.env?.DISCIPLINE_SESSION_TOKEN).toBe('s'.repeat(64))
  })

  it('rejects malformed startup output', async () => {
    const child = fakeChild('{"event":"ready","port":99999}\n')
    await expect(startSidecar({ executable: '/app/bin/server', dataDir: '/tmp/discipline-test', token: 's'.repeat(64), spawnImpl: () => child as never, timeoutMs: 1000 })).rejects.toThrow('启动消息无效')
  })
})
