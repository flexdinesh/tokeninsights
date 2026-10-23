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
import type { ReactNode } from 'react'
import {
  AppShell,
  Button,
  Field,
  Heading,
  Input,
  PageContent,
  PageHeader,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  StartupShell,
  Text,
} from '@/ui'

export function SettingsPage() {
  const { bootstrap, onSignOut } = useIdentity()
  if (bootstrap.serverKind !== 'hosted')
    return (
      <StartupShell>
        <Heading as="h1" variant="page">
          Account settings unavailable
        </Heading>
        <p>Your local workspace manages identity automatically.</p>
        <UsageLink />
      </StartupShell>
    )
  return (
    <DashboardPreferencesProvider>
      <AppShell
        sidebar={
          <DashboardSidebar
            query={initialQuery(bootstrap.defaults)}
            localMachine={false}
            onSignOut={onSignOut}
          />
        }
        header={
          <PageHeader title="/ Account settings">
            <UsageLink />
          </PageHeader>
        }
      >
        <PageContent className="max-w-settings">
          <AccountSettings />
        </PageContent>
      </AppShell>
    </DashboardPreferencesProvider>
  )
}

function SettingsSection({
  id,
  title,
  icon,
  description,
  children,
}: {
  id: string
  title: string
  icon?: ReactNode
  description?: ReactNode
  children: ReactNode
}) {
  return (
    <section className="border-b border-line py-6" aria-labelledby={id}>
      <div className="mb-2 flex flex-wrap items-center gap-3 [&>svg]:size-5">
        {icon}
        <Heading id={id}>{title}</Heading>
      </div>
      {description && (
        <Text variant="secondary" className="mb-4">
          {description}
        </Text>
      )}
      {children}
    </section>
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
      <div className="py-4 text-ink-subtle">
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
      <SettingsSection id="account-heading" title="Account" icon={<UserRound aria-hidden="true" />}>
        <dl className="my-4">
          <dt className="text-ink-subtle">Username</dt>
          <dd className="my-1 font-semibold wrap-anywhere">{account.data.username}</dd>
        </dl>
        <Button variant="outline" onClick={() => void onSignOut()}>
          Sign out
        </Button>
      </SettingsSection>
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
    <SettingsSection
      id="password-heading"
      title="Password"
      description="Changing your password signs you out on all devices. API keys stay active."
    >
      <form className="grid max-w-form gap-4" onSubmit={(event) => void submit(event)}>
        <Field label="Current password">
          {(control) => (
            <Input
              {...control}
              type="password"
              autoComplete="current-password"
              value={currentPassword}
              onChange={(event) => setCurrentPassword(event.target.value)}
              required
              disabled={pending}
            />
          )}
        </Field>
        <Field
          label="New password"
          description={`${passwordMinCharacters}–${passwordMaxCharacters} characters.`}
        >
          {(control) => (
            <Input
              {...control}
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              required
              disabled={pending}
            />
          )}
        </Field>
        <Field label="Confirm new password" error={error || undefined}>
          {(control) => (
            <Input
              {...control}
              type="password"
              autoComplete="new-password"
              value={confirmation}
              onChange={(event) => setConfirmation(event.target.value)}
              required
              disabled={pending}
            />
          )}
        </Field>
        <Button type="submit" className="justify-self-start" disabled={pending}>
          {pending ? 'Changing password…' : 'Change password'}
        </Button>
      </form>
    </SettingsSection>
  )
}

function APIKeySettings() {
  const { bootstrap, onSessionEnded } = useIdentity()
  const client = useQueryClient()
  const keys = useAPIKeys(bootstrap.datasetId)
  const [label, setLabel] = useState('')
  const [access, setAccess] = useState<'sync' | 'read'>('sync')
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
    <SettingsSection
      id="keys-heading"
      title="API keys"
      description="Use a key in your collector configuration to sync usage to this account."
    >
      {created ? (
        <CreatedKey
          value={created}
          onDone={() => {
            restoreNameFocus.current = true
            setCreated(undefined)
          }}
        />
      ) : (
        <form className="grid max-w-form gap-4" onSubmit={(event) => void submit(event)}>
          <Field label="Key name">
            {(control) => (
              <Input
                {...control}
                ref={keyNameRef}
                placeholder="Work laptop"
                value={label}
                onChange={(event) => setLabel(event.target.value)}
                required
                disabled={pending}
              />
            )}
          </Field>
          <Field label="Access">
            {(control) => (
              <Select
                value={access}
                onValueChange={(value) => {
                  if (value === 'sync' || value === 'read') setAccess(value)
                }}
                disabled={pending}
              >
                <SelectTrigger {...control}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="sync">Sync usage (read and ingest)</SelectItem>
                  <SelectItem value="read">Read usage only</SelectItem>
                </SelectContent>
              </Select>
            )}
          </Field>
          <Field label="Expires on (optional)">
            {(control) => (
              <Input
                {...control}
                type="date"
                value={expires}
                onChange={(event) => setExpires(event.target.value)}
                disabled={pending}
              />
            )}
          </Field>
          <Button type="submit" className="justify-self-start" disabled={pending || !label.trim()}>
            {pending ? 'Creating key…' : 'Create API key'}
          </Button>
        </form>
      )}
      {error && (
        <Text variant="danger" role="alert" className="mt-4">
          {error}
        </Text>
      )}
      {keys.error ? (
        <div className="grid justify-items-start gap-2 py-4">
          <Text variant="danger" role="alert">
            Couldn’t load API keys. {keys.error.message}
          </Text>
          <Button variant="outline" onClick={() => void keys.refetch()}>
            Retry keys
          </Button>
        </div>
      ) : keys.data ? (
        keys.data.keys.length ? (
          <ul className="mt-6">
            {keys.data.keys.map((key) => (
              <li
                key={key.tokenId}
                className="flex flex-wrap items-center justify-between gap-3 border-t border-line py-4"
              >
                <KeyDetails value={key} />
                <Button
                  variant="outline"
                  size="compact"
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
          <Text variant="secondary" className="py-4">
            No API keys yet. Create one for your first collector.
          </Text>
        )
      ) : (
        <Text variant="secondary" className="py-4">
          Loading API keys…
        </Text>
      )}
    </SettingsSection>
  )
}

function KeyDetails({ value }: { value: APIKeyInfo }) {
  const [now] = useState(() => Date.now())
  const expired = value.expiresAt !== undefined && new Date(value.expiresAt).getTime() <= now
  return (
    <div className="min-w-0">
      <strong className="wrap-anywhere">{value.label}</strong>
      <Text variant="muted" className="mt-1">
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
      </Text>
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
    <div className="grid max-w-form gap-4 py-4">
      <div>
        <Heading as="h3" variant="sub">
          Save your API key
        </Heading>
        <Text variant="secondary" className="mt-1">
          This key is shown once. Copy it before closing.
        </Text>
      </div>
      <Field label="API key">
        {(control) => (
          <Input
            {...control}
            ref={keyRef}
            value={value.token}
            readOnly
            autoComplete="off"
            spellCheck={false}
          />
        )}
      </Field>
      <div className="flex flex-wrap items-center gap-3">
        <Button variant="outline" onClick={() => void copy()}>
          <Copy aria-hidden="true" />
          Copy key
        </Button>
        <Button onClick={onDone}>Done</Button>
      </div>
      <Text variant="secondary" role="status">
        {message}
      </Text>
    </div>
  )
}
