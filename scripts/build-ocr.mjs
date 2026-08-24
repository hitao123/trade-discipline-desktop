import { execFile } from 'node:child_process'
import { existsSync } from 'node:fs'
import { mkdir } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { promisify } from 'node:util'

const run = promisify(execFile)

if (process.platform !== 'darwin') {
  throw new Error('plain-rule-ocr 只支持 macOS')
}

const moduleCache = join(tmpdir(), 'plain-rule-swift-module-cache')
const xcodeDeveloper = '/Applications/Xcode.app/Contents/Developer'
await Promise.all([
  mkdir('resources/bin', { recursive: true }),
  mkdir(moduleCache, { recursive: true }),
])
await run('xcrun', [
  'swiftc', '-O',
  '-framework', 'Vision',
  '-framework', 'AppKit',
  '-o', 'resources/bin/plain-rule-ocr',
  'native/ocr/main.swift',
], {
  maxBuffer: 2 * 1024 * 1024,
  env: {
    ...process.env,
    ...(existsSync(xcodeDeveloper) ? { DEVELOPER_DIR: xcodeDeveloper } : {}),
    CLANG_MODULE_CACHE_PATH: moduleCache,
    SWIFT_MODULECACHE_PATH: moduleCache,
  },
})
