import { useState } from 'react'
import type { FormEvent } from 'react'
import { changeBrowserSession } from '../api'
import { signup, validPassword } from '../account'
import { Brand } from './Brand'
import { Button } from './ui/button'
import { Input } from './ui/input'

export function PasswordLogin({
  onSignIn,
  signupEnabled,
  disabled = false,
  sessionError = '',
}: {
  onSignIn: () => Promise<void>
  signupEnabled: boolean
  disabled?: boolean
  sessionError?: string
}) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [creating, setCreating] = useState(false)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending || disabled) return
    if (creating && !validPassword(password)) {
      setError('Use a password between 15 and 128 characters.')
      return
    }
    setPending(true)
    setError('')
    try {
      const credentials = { username, password }
      if (creating) await signup(credentials)
      else await changeBrowserSession(credentials)
      setPassword('')
      await onSignIn()
    } catch {
      setError(
        creating
          ? 'Couldn’t create your account. Check your details and try again.'
          : 'Couldn’t sign in. Check your username and password and try again.',
      )
    } finally {
      setPending(false)
    }
  }

  return (
    <div className="app-shell">
      <header className="app-header">
        <Brand />
      </header>
      <main className="startup-state password-login">
        <h1>{creating ? 'Create your account' : 'Sign in to TokenInsights'}</h1>
        <p>
          {creating
            ? 'Your account keeps your synced usage together.'
            : 'View your synced usage and manage your API keys.'}
        </p>
        <form onSubmit={(event) => void submit(event)}>
          <label htmlFor="username">Username</label>
          <Input
            id="username"
            name="username"
            autoComplete="username"
            value={username}
            onChange={(event) => setUsername(event.target.value)}
            required
            disabled={pending || disabled}
            aria-describedby={creating ? 'username-help' : undefined}
          />
          {creating && (
            <p id="username-help" className="field-help">
              3–32 letters, numbers, underscores or hyphens. Start with a letter or number.
            </p>
          )}
          <label htmlFor="password">Password</label>
          <Input
            id="password"
            name="password"
            type="password"
            autoComplete={creating ? 'new-password' : 'current-password'}
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            required
            disabled={pending || disabled}
            aria-invalid={Boolean(error || sessionError)}
            aria-describedby={
              creating ? 'password-help' : error || sessionError ? 'login-error' : undefined
            }
          />
          {creating && (
            <p id="password-help" className="field-help">
              15–128 characters.
            </p>
          )}
          {(error || sessionError) && (
            <p id="login-error" role="alert">
              {error || sessionError}
            </p>
          )}
          <Button type="submit" disabled={pending || disabled || !username.trim() || !password}>
            {pending
              ? creating
                ? 'Creating account…'
                : 'Signing in…'
              : creating
                ? 'Create account'
                : 'Sign in'}
          </Button>
        </form>
        {signupEnabled && (
          <Button
            variant="ghost"
            disabled={pending || disabled}
            onClick={() => {
              setCreating(!creating)
              setError('')
              setPassword('')
            }}
          >
            {creating ? 'Already have an account? Sign in' : 'Create an account'}
          </Button>
        )}
      </main>
    </div>
  )
}
