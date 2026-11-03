import { app, BrowserWindow, dialog, shell } from 'electron'
import { join } from 'node:path'
import type { BackendConnection } from './backend'
import { externalURL, launchURL, localNavigation } from './links'
import { connectTarget, localLaunch } from './launch'

app.setName('TokenInsights Electron')
app.setAppUserModelId('com.flexdinesh.tokeninsights.electron')
app.enableSandbox()

let window: BrowserWindow | undefined
let connection: BackendConnection | undefined
let quitting = false
let cleaningUp = false
let requestedEndpoint: string | undefined
let requestedURL: string | undefined
const pendingLinks: string[] = []

function openExternal(value: string) {
  const url = externalURL(value)
  if (url)
    void shell
      .openExternal(url)
      .catch(() =>
        dialog.showErrorBox('Couldn’t open browser', 'Open the address in your browser.'),
      )
}

async function handleArguments(args: string[]) {
  let local = false
  let external = false
  const urls = args.map(launchURL).filter((url): url is string => url !== undefined)
  const targets = await Promise.all(
    urls.map(async (url) => ({ url, local: await localLaunch(url) })),
  )
  for (const target of targets) {
    if (target.local) {
      const endpoint = new URL(target.url).origin
      const current = connection?.descriptor.url ?? requestedEndpoint
      if (current !== undefined && endpoint !== new URL(current).origin) {
        openExternal(target.url)
        external = true
      } else {
        requestedEndpoint = endpoint
        requestedURL = target.url
        local = true
      }
    } else {
      openExternal(target.url)
      external = true
    }
  }
  return external && !local
}

async function startWindow(): Promise<void> {
  try {
    connection = await connectTarget(
      join(process.resourcesPath, 'tokeninsights-backend'),
      () => {
        if (quitting) return
        dialog.showErrorBox(
          'Local backend disconnected',
          'Reopen TokenInsights Electron to reconnect. Saved usage remains available.',
        )
        app.quit()
      },
      requestedURL,
      (url) => {
        if (!quitting) openExternal(url)
      },
    )
    if (connection === undefined) {
      app.quit()
      return
    }
    if (quitting) {
      connection.close()
      return
    }
    const backendURL = connection.descriptor.url
    const current = new BrowserWindow({
      title: 'TokenInsights Electron',
      width: 1280,
      height: 850,
      minWidth: 360,
      minHeight: 480,
      show: false,
      webPreferences: {
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
    window = current
    const session = current.webContents.session
    session.setPermissionRequestHandler((_contents, _permission, callback) => callback(false))
    session.setPermissionCheckHandler(() => false)
    session.on('will-download', (event) => event.preventDefault())
    current.webContents.on('will-attach-webview', (event) => event.preventDefault())
    current.webContents.on('will-navigate', (event, url) => {
      if (!localNavigation(url, backendURL)) {
        event.preventDefault()
        openExternal(url)
      }
    })
    current.webContents.on('will-frame-navigate', (event) => {
      if (!event.isMainFrame || !localNavigation(event.url, backendURL)) event.preventDefault()
    })
    current.webContents.on('will-redirect', (event, url) => {
      if (!localNavigation(url, backendURL)) event.preventDefault()
    })
    current.webContents.setWindowOpenHandler(({ url }) => {
      openExternal(url)
      return { action: 'deny' }
    })
    // Backend assets compose the shared dashboard with its browser HTTP client.
    // No preload or IPC is needed: the renderer has browser capabilities only.
    session.webRequest.onBeforeRequest((details, callback) => {
      callback({ cancel: !localNavigation(details.url, backendURL) })
    })
    current.once('ready-to-show', () => current.show())
    current.once('closed', () => {
      connection?.close()
      connection = undefined
      window = undefined
      if (!cleaningUp) app.quit()
    })
    await current.loadURL(backendURL)
  } catch (error) {
    connection?.close()
    connection = undefined
    cleaningUp = true
    window?.destroy()
    window = undefined
    cleaningUp = false
    if (quitting) return
    const result = await dialog.showMessageBox({
      type: 'error',
      title: 'Couldn’t start TokenInsights Electron',
      message: error instanceof Error ? error.message : 'Local backend startup failed.',
      buttons: ['Retry', 'Quit'],
      defaultId: 0,
      cancelId: 1,
    })
    if (result.response === 0) await startWindow()
    else app.quit()
  }
}

if (!app.requestSingleInstanceLock()) app.quit()
else {
  app.on('second-instance', (_event, args) => {
    void handleArguments(args).then((externalOnly) => {
      if (!externalOnly) {
        window?.show()
        window?.focus()
      }
    })
  })
  app.on('open-url', (event, url) => {
    event.preventDefault()
    if (!app.isReady()) pendingLinks.push(url)
    else {
      void handleArguments([url]).then((externalOnly) => {
        if (!externalOnly) {
          window?.show()
          window?.focus()
        }
      })
    }
  })
  app.on('before-quit', () => {
    quitting = true
    connection?.close()
  })
  void app.whenReady().then(async () => {
    app.setAsDefaultProtocolClient('tokeninsights-electron')
    if (await handleArguments([...process.argv.slice(1), ...pendingLinks])) {
      app.quit()
      return
    }
    await startWindow()
  })
}
