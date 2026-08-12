export {}

declare global {
  interface Window {
    discipline?: {
      apiBaseURL: string
      sessionToken: string
      appVersion: string
      selectCSV: () => Promise<{ name: string; content: string } | null>
      selectBackup: () => Promise<string | null>
      selectExportPath: () => Promise<string | null>
      exportBackup: () => Promise<string | null>
      restoreBackup: () => Promise<boolean>
      restartBackend: () => Promise<void>
      openLogs: () => Promise<void>
    }
  }
}
