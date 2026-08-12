import { chmod, mkdir } from 'node:fs/promises'
import { spawnSync } from 'node:child_process'
import path from 'node:path'

const root = process.cwd()
const outputDir = path.join(root, 'resources', 'bin')
const output = path.join(outputDir, 'discipline-server')
const cacheDir = path.join(root, '.cache', 'go-build')
await Promise.all([mkdir(outputDir, { recursive: true }), mkdir(cacheDir, { recursive: true })])
const result = spawnSync('go', ['build', '-trimpath', '-o', output, './cmd/discipline-server'], { cwd: path.join(root, 'backend'), stdio: 'inherit', env: { ...process.env, CGO_ENABLED: '0', GOCACHE: cacheDir } })
if (result.status !== 0) process.exit(result.status ?? 1)
await chmod(output, 0o755)
