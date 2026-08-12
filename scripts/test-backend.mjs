import { mkdir } from 'node:fs/promises'
import { spawnSync } from 'node:child_process'
import path from 'node:path'

const root = process.cwd()
const cacheDir = path.join(root, '.cache', 'go-build')
await mkdir(cacheDir, { recursive: true })

const result = spawnSync('go', ['test', './...'], {
  cwd: path.join(root, 'backend'),
  stdio: 'inherit',
  env: { ...process.env, CGO_ENABLED: '0', GOCACHE: cacheDir },
})

process.exit(result.status ?? 1)
