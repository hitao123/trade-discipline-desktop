import { copyFile, mkdir, rename, rm } from 'node:fs/promises'
import path from 'node:path'
import { randomBytes } from 'node:crypto'

import { app, BrowserWindow, dialog, ipcMain, Menu, shell } from 'electron'

import { startSidecar, type SidecarHandle } from './sidecar'

let mainWindow: BrowserWindow | null = null
let sidecar: SidecarHandle | null = null
let shuttingDown = false
let quitAfterSidecarStops = false
const sessionToken = randomBytes(32).toString('hex')

function dataPaths() {
  const root = app.getPath('userData')
  return { root, database: path.join(root, 'discipline.db'), backups: path.join(root, 'backups'), logs: path.join(root, 'logs') }
}

function sidecarExecutable() {
  return app.isPackaged
    ? path.join(process.resourcesPath, 'bin', 'discipline-server')
    : path.join(app.getAppPath(), 'resources', 'bin', 'discipline-server')
}

async function startBackend() {
  const paths = dataPaths()
  await Promise.all([mkdir(paths.backups, { recursive: true, mode: 0o700 }), mkdir(paths.logs, { recursive: true, mode: 0o700 })])
  sidecar = await startSidecar({
    executable: sidecarExecutable(),
    dataDir: paths.root,
    token: sessionToken,
    onStderr: line => console.error(`[sidecar] ${line}`),
    onExit: (code, signal) => {
      if (!shuttingDown && sidecar) showBackendFailure(`本地后端意外退出（${code ?? signal ?? 'unknown'}）`)
    },
  })
}

async function apiRequest<T>(endpoint: string, body?: unknown): Promise<T> {
  if (!sidecar) throw new Error('本地后端尚未启动')
  const requestInit: RequestInit = {
    method: body === undefined ? 'GET' : 'POST',
    headers: { Authorization: `Bearer ${sessionToken}`, Accept: 'application/json', ...(body === undefined ? {} : { 'Content-Type': 'application/json' }) },
  }
  if (body !== undefined) requestInit.body = JSON.stringify(body)
  const response = await fetch(`${sidecar.baseURL}${endpoint}`, requestInit)
  const envelope = await response.json() as { ok: boolean; data?: T; error?: { message: string } }
  if (!response.ok || !envelope.ok) throw new Error(envelope.error?.message ?? `本地请求失败（${response.status}）`)
  return envelope.data as T
}

async function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1320,
    height: 860,
    minWidth: 1024,
    minHeight: 680,
    title: 'Plain Rule',
    backgroundColor: '#f8f6f0',
    show: false,
    webPreferences: {
      preload: path.join(app.getAppPath(), 'dist', 'preload', 'index.cjs'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      devTools: true,
    },
  })
  mainWindow.webContents.setWindowOpenHandler(() => ({ action: 'deny' }))
  mainWindow.webContents.on('will-navigate', (event) => event.preventDefault())
  mainWindow.once('ready-to-show', () => {
    mainWindow?.show()
    // mainWindow?.webContents.openDevTools({ mode: 'right' })
  })
  await mainWindow.loadFile(path.join(app.getAppPath(), 'dist', 'renderer', 'index.html'))
}

function showBackendFailure(message: string) {
  if (!mainWindow || mainWindow.isDestroyed()) return
  const escaped = message.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;')
  const html = `<!doctype html><meta charset="utf-8"><title>本地后端异常</title><style>body{font-family:-apple-system,sans-serif;background:#f8f6f0;color:#20221f;padding:64px;max-width:720px}h1{font-family:serif;font-weight:500}p{line-height:1.7;color:#65675f}.risk{color:#b23b2b}</style><h1>本地数据服务暂时不可用</h1><p class="risk">${escaped}</p><p>请重新打开应用。日志位于 Application Support 的 logs 目录，数据库不会因为界面退出而被删除。</p>`
  void mainWindow.loadURL(`data:text/html;charset=utf-8,${encodeURIComponent(html)}`)
}

function registerIPC() {
  ipcMain.on('backend-config', (event) => {
    event.returnValue = { apiBaseURL: sidecar?.baseURL ?? '', sessionToken, appVersion: app.getVersion() }
  })
  ipcMain.handle('select-csv', async () => {
    const result = await dialog.showOpenDialog({ properties: ['openFile'], filters: [{ name: '收盘榜单 CSV', extensions: ['csv'] }] })
    if (result.canceled || !result.filePaths[0]) return null
    const filePath = result.filePaths[0]
    const content = await import('node:fs/promises').then(fs => fs.readFile(filePath, 'utf8'))
    return { name: path.basename(filePath), content }
  })
  ipcMain.handle('select-backup', async () => {
    const result = await dialog.showOpenDialog({ properties: ['openFile'], filters: [{ name: 'Plain Rule Backup', extensions: ['db'] }] })
    return result.canceled ? null : (result.filePaths[0] ?? null)
  })
  ipcMain.handle('select-export-path', async () => {
    const result = await dialog.showSaveDialog({ defaultPath: path.join(dataPaths().backups, `discipline-${timestamp()}.db`), filters: [{ name: 'Plain Rule Backup', extensions: ['db'] }] })
    return result.canceled ? null : (result.filePath ?? null)
  })
  ipcMain.handle('export-backup', async () => {
    const result = await dialog.showSaveDialog({ defaultPath: path.join(dataPaths().backups, `discipline-${timestamp()}.db`), filters: [{ name: 'Plain Rule Backup', extensions: ['db'] }] })
    if (result.canceled || !result.filePath) return null
    const destination = result.filePath.endsWith('.db') ? result.filePath : `${result.filePath}.db`
    await apiRequest('/api/backup/export', { path: destination })
    return destination
  })
  ipcMain.handle('restore-backup', async () => restoreBackup())
  ipcMain.handle('restart-backend', async () => restartBackend())
  ipcMain.handle('open-logs', async () => shell.openPath(dataPaths().logs))
}

async function restartBackend() {
  const old = sidecar
  sidecar = null
  await old?.stop()
  await startBackend()
  mainWindow?.reload()
}

async function restoreBackup(): Promise<boolean> {
  const result = await dialog.showOpenDialog({ properties: ['openFile'], filters: [{ name: 'Plain Rule Backup', extensions: ['db'] }] })
  const source = result.filePaths[0]
  if (result.canceled || !source) return false
  await apiRequest('/api/backup/validate', { path: source })
  const paths = dataPaths()
  const safetyBackup = path.join(paths.backups, `before-restore-${timestamp()}.db`)
  await apiRequest('/api/backup/export', { path: safetyBackup })
  const old = sidecar
  sidecar = null
  await old?.stop()
  const temporary = `${paths.database}.restore-tmp`
  try {
    await rm(temporary, { force: true })
    await copyFile(source, temporary)
    await rename(temporary, paths.database)
    await Promise.all([rm(`${paths.database}-wal`, { force: true }), rm(`${paths.database}-shm`, { force: true })])
    await startBackend()
    mainWindow?.reload()
    return true
  }
  catch (error) {
    await rm(temporary, { force: true })
    await startBackend()
    throw error
  }
}

function timestamp() {
  return new Date().toISOString().replaceAll(':', '').replaceAll('.', '-')
}

function installMenu() {
  Menu.setApplicationMenu(Menu.buildFromTemplate([
    { role: 'appMenu', submenu: [{ role: 'about', label: 'About Plain Rule' }, { type: 'separator' }, { role: 'quit', label: 'Quit Plain Rule' }] },
    { label: '编辑', submenu: [{ role: 'undo', label: '撤销' }, { role: 'redo', label: '重做' }, { type: 'separator' }, { role: 'cut', label: '剪切' }, { role: 'copy', label: '复制' }, { role: 'paste', label: '粘贴' }, { role: 'selectAll', label: '全选' }] },
    { label: '视图', submenu: [{ role: 'reload', label: '重新加载' }, { role: 'forceReload', label: '强制重新加载' }, { type: 'separator' }, { role: 'toggleDevTools', label: '开发者工具' }, { type: 'separator' }, { role: 'resetZoom', label: '实际大小' }, { role: 'zoomIn', label: '放大' }, { role: 'zoomOut', label: '缩小' }, { type: 'separator' }, { role: 'togglefullscreen', label: '全屏' }] },
    { label: '窗口', submenu: [{ role: 'minimize', label: '最小化' }, { role: 'zoom', label: '缩放' }, { role: 'front', label: '前置全部窗口' }] },
  ]))
}

if (!app.requestSingleInstanceLock()) app.quit()
else {
  app.on('second-instance', () => { if (mainWindow) { if (mainWindow.isMinimized()) mainWindow.restore(); mainWindow.focus() } })
  app.on('before-quit', (event) => {
    if (!sidecar || quitAfterSidecarStops) return
    event.preventDefault()
    shuttingDown = true
    const active = sidecar
    sidecar = null
    void active.stop().finally(() => {
      quitAfterSidecarStops = true
      app.quit()
    })
  })
  app.on('window-all-closed', () => { if (process.platform !== 'darwin') app.quit() })
  app.on('activate', () => { if (BrowserWindow.getAllWindows().length === 0 && sidecar) void createWindow() })
  void app.whenReady().then(async () => {
    registerIPC()
    installMenu()
    try { await startBackend(); await createWindow() }
    catch (error) { await createWindow(); showBackendFailure(error instanceof Error ? error.message : '未知启动错误') }
  })
}
