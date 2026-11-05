import { app, BrowserWindow, dialog, shell } from 'electron'
import { join } from 'node:path'
import { randomUUID } from 'node:crypto'
import type { BackendConnection } from './backend'
import { externalURL, launchURL, localNavigation } from './links'
import { connectTarget } from './launch'

app.setName('TokenInsights Electron')
app.setAppUserModelId('com.flexdinesh.tokeninsights.electron')
app.enableSandbox()

const windows = new Map<BrowserWindow, BackendConnection>()
const handledLinks = new Set<string>()
const pendingLinks: string[] = []
let quitting = false
let initialized = false
let invalidInitialRequest = false
let connecting = 0

function openExternal(value: string) {
  const url = externalURL(value)
  if (url)
    void shell
      .openExternal(url)
      .catch(() =>
        dialog.showErrorBox('Couldn’t open browser', 'Open the address in your browser.'),
      )
}

// Each invocation creates its own window, connector session and bridge. An
// existing window cannot silently consume another connection's launch policy.
async function startWindow(link?: string): Promise<void> {
  if (link !== undefined) {
    if (handledLinks.has(link)) return
    handledLinks.add(link)
  }
  connecting += 1
  let connection: BackendConnection | undefined
  let current: BrowserWindow | undefined
  try {
    connection = await connectTarget(
      join(process.resourcesPath, 'tokeninsights-backend'),
      () => {
        if (quitting) return
        dialog.showErrorBox(
          'Backend disconnected',
          'Reopen this connection. Saved usage remains available.',
        )
        current?.close()
      },
      link,
    )
    if (quitting) {
      connection.close()
      return
    }
    const backendURL = connection.descriptor.url
    current = new BrowserWindow({
      title: 'TokenInsights Electron',
      width: 1280,
      height: 850,
      minWidth: 360,
      minHeight: 480,
      show: false,
      webPreferences: {
        partition: randomUUID(),
        nodeIntegration: false,
        nodeIntegrationInWorker: false,
        nodeIntegrationInSubFrames: false,
        contextIsolation: true,
        sandbox: true,
        webSecurity: true,
        webviewTag: false,
        devTools: false,
        navigateOnDragDrop: false,
      },
    })
    const window = current
    const owned = connection
    windows.set(window, owned)
    const session = window.webContents.session
    session.setPermissionRequestHandler((_contents, _permission, callback) => callback(false))
    session.setPermissionCheckHandler(() => false)
    session.on('will-download', (event) => event.preventDefault())
    window.webContents.on('will-attach-webview', (event) => event.preventDefault())
    window.webContents.on('will-navigate', (event, url) => {
      if (!localNavigation(url, backendURL)) {
        event.preventDefault()
        openExternal(url)
      }
    })
    window.webContents.on('will-frame-navigate', (event) => {
      if (!event.isMainFrame || !localNavigation(event.url, backendURL)) event.preventDefault()
    })
    window.webContents.on('will-redirect', (event, url) => {
      if (!localNavigation(url, backendURL)) event.preventDefault()
    })
    window.webContents.setWindowOpenHandler(({ url }) => {
      openExternal(url)
      return { action: 'deny' }
    })
    // A private session prevents one window's request filter from replacing
    // another window's filter when they target different bridges.
    session.webRequest.onBeforeRequest((details, callback) =>
      callback({ cancel: !localNavigation(details.url, backendURL) }),
    )
    window.once('ready-to-show', () => window.show())
    window.once('closed', () => {
      windows.delete(window)
      owned.close()
    })
    await window.loadURL(backendURL)
    if (!window.isDestroyed()) owned.acknowledge()
  } catch (error) {
    connection?.close()
    current?.destroy()
    if (!quitting)
      dialog.showErrorBox(
        'Couldn’t start TokenInsights Electron',
        error instanceof Error ? error.message : 'Backend startup failed.',
      )
  } finally {
    connecting -= 1
    if (windows.size === 0 && connecting === 0) app.quit()
  }
}

function handleArguments(args: string[]): boolean {
  let requested = false
  for (const value of args) {
    const link = launchURL(value)
    if (link !== undefined) {
      requested = true
      if (!app.isReady()) pendingLinks.push(link)
      else void startWindow(link)
    } else if (value.includes('://')) {
      requested = true
      if (!initialized) invalidInitialRequest = true
      dialog.showErrorBox(
        'Invalid desktop launch',
        'Run tokeninsights connect URI to select a connection.',
      )
    }
  }
  return requested
}

function focus() {
  const first = windows.keys().next().value
  if (first !== undefined) {
    first.show()
    first.focus()
  } else if (connecting === 0) void startWindow()
}

if (!app.requestSingleInstanceLock()) app.quit()
else {
  app.on('second-instance', (_event, args) => {
    if (!handleArguments(args)) focus()
  })
  app.on('open-url', (event, url) => {
    event.preventDefault()
    handleArguments([url])
  })
  app.on('activate', () => {
    if (initialized && pendingLinks.length === 0) focus()
  })
  app.on('before-quit', () => {
    quitting = true
    for (const connection of windows.values()) connection.close()
  })
  app.on('window-all-closed', () => {
    if (connecting === 0) app.quit()
  })
  void app.whenReady().then(() => {
    app.setAsDefaultProtocolClient('tokeninsights-electron')
    const links = [...process.argv.slice(1), ...pendingLinks]
    pendingLinks.length = 0
    const requested = handleArguments(links)
    initialized = true
    if (!requested && !invalidInitialRequest) void startWindow()
    else if (connecting === 0 && windows.size === 0) app.quit()
  })
}
