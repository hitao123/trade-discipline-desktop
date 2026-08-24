import { contextBridge, ipcRenderer } from 'electron'

interface BackendConfig { apiBaseURL: string; sessionToken: string; appVersion: string }
const config = ipcRenderer.sendSync('backend-config') as BackendConfig

contextBridge.exposeInMainWorld('discipline', {
  ...config,
  selectCSV: () => ipcRenderer.invoke('select-csv'),
  recognizeExecutionScreenshot: () => ipcRenderer.invoke('recognize-execution-screenshot'),
  selectBackup: () => ipcRenderer.invoke('select-backup'),
  selectExportPath: () => ipcRenderer.invoke('select-export-path'),
  exportBackup: () => ipcRenderer.invoke('export-backup'),
  restoreBackup: () => ipcRenderer.invoke('restore-backup'),
  restartBackend: () => ipcRenderer.invoke('restart-backend'),
  openLogs: () => ipcRenderer.invoke('open-logs'),
  onMonitorAlert: (handler: (alertID: string) => void) => {
    const listener = (_event: Electron.IpcRendererEvent, alertID: string) => handler(alertID)
    ipcRenderer.on('monitor-alert-opened', listener)
    return () => ipcRenderer.removeListener('monitor-alert-opened', listener)
  },
})
