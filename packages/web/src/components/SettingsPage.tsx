import { useCallback, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Copy, UserRound } from 'lucide-react'
import {
  changePassword,
  createAPIKey,
  revokeAPIKey,
  useAccount,
  useAPIKeys,
  passwordMaxCharacters,
  passwordMinCharacters,
  validPassword,
} from '../account'
import type { APIKeyCreated, APIKeyInfo } from '../contracts'
import { apiKeyLabelMaxCharacters } from '../contracts'
import { AuthenticationRequired } from '../http'
import { useIdentity } from '../identity'
import { initialQuery, parseDashboardSearch } from '../query'
import { DashboardPreferencesProvider } from '../state'
import { DashboardSidebar } from './DashboardSidebar'
import { Button } from './ui/button'
import { Input } from './ui/input'

export function SettingsPage() {
  const { bootstrap, onSignOut } = useIdentity()
  if (bootstrap.serverKind !== 'hosted')
    return (
      <main className="startup-state">
        <h1>Account settings unavailable</h1>
        <p>Your local workspace manages identity automatically.</p>
        <UsageLink />
      </main>
    )
  return (
    <DashboardPreferencesProvider>
      <div className="app-shell dashboard-shell">
        <DashboardSidebar
          query={initialQuery(bootstrap.defaults)}
          localMachine={false}
          onSignOut={onSignOut}
        />
        <div className="dashboard-main">
          <header className="app-header">
            <h1 className="header-view">/ Account settings</h1>
            <div className="header-actions">
              <UsageLink />
            </div>
          </header>
          <main className="dashboard account-settings">
            <AccountSettings />
          </main>
        </div>
      </div>
    </DashboardPreferencesProvider>
  )
}

function UsageLink() {
  return (
    <Button variant="ghost" asChild>
      <Link to="/$tab" params={{ tab: 'tokens' }} search={parseDashboardSearch({})}>
        Return to usage
      </Link>
    </Button>
  )
}

function AccountSettings() {
  const { bootstrap, onSignOut } = useIdentity()
  const account = useAccount(bootstrap.datasetId, true)
  if (!account.data)
    return (
      <div className="settings-state">
        {account.error ? (
          <>
            <p role="alert">Couldn’t load your account. {account.error.message}</p>
            <Button onClick={() => void account.refetch()}>Retry</Button>
          </>
        ) : (
          <p>Loading account…</p>
        )}
      </div>
    )
  return (
    <>
      <section className="settings-section" aria-labelledby="account-heading">
        <div className="settings-heading">
          <UserRound size="1.25em" />
          <h2 id="account-heading">Account</h2>
        </div>
        <dl className="account-details">
          <dt>Username</dt>
          <dd>{account.data.username}</dd>
        </dl>
        <Button variant="outline" onClick={() => void onSignOut()}>
          Sign out
        </Button>
      </section>
      <PasswordSettings />
      <APIKeySettings />
    </>
  )
}

function PasswordSettings() {
  const { bootstrap, onSessionEnded } = useIdentity()
  const [currentPassword, setCurrentPassword] = useState('')
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending) return
    if (!validPassword(password)) {
      setError(
        `Use a password between ${passwordMinCharacters} and ${passwordMaxCharacters} characters.`,
      )
      return
    }
    if (password !== confirmation) {
      setError('Your new passwords don’t match.')
      return
    }
    setPending(true)
    setError('')
    try {
      await changePassword(bootstrap.datasetId, currentPassword, password)
      setCurrentPassword('')
      setPassword('')
      setConfirmation('')
      await onSessionEnded('Password changed. Sign in with your new password.')
    } catch (failure) {
      if (failure instanceof AuthenticationRequired) await onSessionEnded()
      else setError('Couldn’t change your password. Check your current password and try again.')
    } finally {
      setPending(false)
    }
  }
  return (
    <section className="settings-section" aria-labelledby="password-heading">
      <h2 id="password-heading">Password</h2>
      <p>Changing your password signs you out on all devices. API keys stay active.</p>
      <form className="settings-form" onSubmit={(event) => void submit(event)}>
        <label htmlFor="current-password">Current password</label>
        <Input
          id="current-password"
          type="password"
          autoComplete="current-password"
          value={currentPassword}
          onChange={(event) => setCurrentPassword(event.target.value)}
          required
          disabled={pending}
        />
        <label htmlFor="new-password">New password</label>
        <Input
          id="new-password"
          type="password"
          autoComplete="new-password"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          required
          disabled={pending}
          aria-describedby="new-password-help"
        />
        <p id="new-password-help" className="field-help">
          {passwordMinCharacters}–{passwordMaxCharacters} characters.
        </p>
        <label htmlFor="confirm-password">Confirm new password</label>
        <Input
          id="confirm-password"
          type="password"
          autoComplete="new-password"
          value={confirmation}
          onChange={(event) => setConfirmation(event.target.value)}
          required
          disabled={pending}
          aria-invalid={Boolean(error)}
          aria-describedby={error ? 'password-error' : undefined}
        />
        {error && (
          <p id="password-error" role="alert">
            {error}
          </p>
        )}
        <Button type="submit" disabled={pending}>
          {pending ? 'Changing password…' : 'Change password'}
        </Button>
      </form>
    </section>
  )
}

function APIKeySettings() {
  const { bootstrap, onSessionEnded } = useIdentity()
  const client = useQueryClient()
  const keys = useAPIKeys(bootstrap.datasetId)
  const [label, setLabel] = useState('')
  const [access, setAccess] = useState('sync')
  const [expires, setExpires] = useState('')
  const [created, setCreated] = useState<APIKeyCreated>()
  const [pending, setPending] = useState(false)
  const [revoking, setRevoking] = useState('')
  const [error, setError] = useState('')
  const restoreNameFocus = useRef(false)
  const keyNameRef = useCallback((input: HTMLInputElement | null) => {
    if (input && restoreNameFocus.current) {
      input.focus()
      restoreNameFocus.current = false
    }
  }, [])
  async function failed(failure: unknown) {
    if (failure instanceof AuthenticationRequired) await onSessionEnded()
    else
      setError(failure instanceof Error ? failure.message : 'Couldn’t update API keys. Try again.')
  }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending || created || !label.trim()) return
    if (Array.from(label.trim()).length > apiKeyLabelMaxCharacters) {
      setError(`Use a key name with at most ${apiKeyLabelMaxCharacters} characters.`)
      return
    }
    setPending(true)
    setError('')
    try {
      const permissions: ('read' | 'ingest')[] = access === 'read' ? ['read'] : ['read', 'ingest']
      const expiresAt = expires ? new Date(`${expires}T23:59:59`).toISOString() : undefined
      setCreated(
        await createAPIKey(bootstrap.datasetId, { label: label.trim(), permissions, expiresAt }),
      )
      setLabel('')
      setExpires('')
      await client.invalidateQueries({ queryKey: ['account-keys', bootstrap.datasetId] })
    } catch (failure) {
      await failed(failure)
    } finally {
      setPending(false)
    }
  }
  async function revoke(tokenId: string) {
    if (revoking) return
    setRevoking(tokenId)
    setError('')
    try {
      await revokeAPIKey(bootstrap.datasetId, tokenId)
      if (created?.tokenId === tokenId) setCreated(undefined)
      await client.invalidateQueries({ queryKey: ['account-keys', bootstrap.datasetId] })
    } catch (failure) {
      await failed(failure)
    } finally {
      setRevoking('')
    }
  }
  return (
    <section className="settings-section" aria-labelledby="keys-heading">
      <h2 id="keys-heading">API keys</h2>
      <p>Use a key in your collector configuration to sync usage to this account.</p>
      {created ? (
        <CreatedKey
          value={created}
          onDone={() => {
            restoreNameFocus.current = true
            setCreated(undefined)
          }}
        />
      ) : (
        <form className="settings-form" onSubmit={(event) => void submit(event)}>
          <label htmlFor="key-label">Key name</label>
          <Input
            id="key-label"
            ref={keyNameRef}
            placeholder="Work laptop"
            value={label}
            onChange={(event) => setLabel(event.target.value)}
            required
            disabled={pending}
          />
          <label htmlFor="key-access">Access</label>
          <select
            id="key-access"
            value={access}
            onChange={(event) => setAccess(event.target.value)}
            disabled={pending}
          >
            <option value="sync">Sync usage (read and ingest)</option>
            <option value="read">Read usage only</option>
          </select>
          <label htmlFor="key-expiry">Expires on (optional)</label>
          <Input
            id="key-expiry"
            type="date"
            value={expires}
            onChange={(event) => setExpires(event.target.value)}
            disabled={pending}
          />
          <Button type="submit" disabled={pending || !label.trim()}>
            {pending ? 'Creating key…' : 'Create API key'}
          </Button>
        </form>
      )}
      {error && <p role="alert">{error}</p>}
      {keys.error ? (
        <div className="settings-state">
          <p role="alert">Couldn’t load API keys. {keys.error.message}</p>
          <Button variant="outline" onClick={() => void keys.refetch()}>
            Retry keys
          </Button>
        </div>
      ) : keys.data ? (
        keys.data.keys.length ? (
          <ul className="api-key-list">
            {keys.data.keys.map((key) => (
              <li key={key.tokenId}>
                <KeyDetails value={key} />
                <Button
                  variant="outline"
                  size="sm"
                  disabled={Boolean(revoking) || Boolean(key.revokedAt)}
                  onClick={() => void revoke(key.tokenId)}
                  aria-label={`Revoke ${key.label}`}
                >
                  {key.revokedAt ? 'Revoked' : revoking === key.tokenId ? 'Revoking…' : 'Revoke'}
                </Button>
              </li>
            ))}
          </ul>
        ) : (
          <p className="settings-state">No API keys yet. Create one for your first collector.</p>
        )
      ) : (
        <p className="settings-state">Loading API keys…</p>
      )}
    </section>
  )
}

function KeyDetails({ value }: { value: APIKeyInfo }) {
  const [now] = useState(() => Date.now())
  const expired = value.expiresAt !== undefined && new Date(value.expiresAt).getTime() <= now
  return (
    <div>
      <strong>{value.label}</strong>
      <p>
        {value.permissions.includes('ingest')
          ? value.permissions.includes('read')
            ? 'Read and ingest'
            : 'Ingest only'
          : 'Read only'}{' '}
        · Created {new Date(value.createdAt).toLocaleDateString()} ·{' '}
        {value.revokedAt
          ? 'Revoked'
          : value.expiresAt
            ? `${expired ? 'Expired' : 'Expires'} ${new Date(value.expiresAt).toLocaleDateString()}`
            : 'No expiry'}
      </p>
    </div>
  )
}

function CreatedKey({ value, onDone }: { value: APIKeyCreated; onDone: () => void }) {
  const [message, setMessage] = useState('')
  const keyRef = useCallback((input: HTMLInputElement | null) => input?.focus(), [])
  async function copy() {
    try {
      await navigator.clipboard.writeText(value.token)
      setMessage('Copied.')
    } catch {
      setMessage('Couldn’t copy. Select and copy the key below.')
    }
  }
  return (
    <div className="created-key">
      <h3>Save your API key</h3>
      <p>This key is shown once. Copy it before closing.</p>
      <label htmlFor="created-key">API key</label>
      <Input
        id="created-key"
        ref={keyRef}
        value={value.token}
        readOnly
        autoComplete="off"
        spellCheck={false}
      />
      <div className="settings-actions">
        <Button variant="outline" onClick={() => void copy()}>
          <Copy size="1em" />
          Copy key
        </Button>
        <Button onClick={onDone}>Done</Button>
      </div>
      <p role="status">{message}</p>
    </div>
  )
}
