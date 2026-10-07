import { useState } from 'react'
import type { FormEvent } from 'react'
import { changeBrowserSession } from '../api'
import { Button } from './ui/button'

export function TokenLogin({
  onSignIn,
  disabled = false,
  sessionError = '',
}: {
  onSignIn: () => Promise<void>
  disabled?: boolean
  sessionError?: string
}) {
  const [token, setToken] = useState('')
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (pending || disabled || !token.trim()) return
    setPending(true)
    setError('')
    try {
      await changeBrowserSession(token.trim())
      setToken('')
      await onSignIn()
    } catch {
      setError('Couldn’t sign in. Check your token and try again.')
    } finally {
      setPending(false)
    }
  }

  return (
    <main className="startup-state token-login">
      <h1>Sign in to TokenInsights</h1>
      <p>Enter the access token provided by your server administrator.</p>
      <form onSubmit={(event) => void submit(event)}>
        <label htmlFor="access-token">Access token</label>
        <input
          id="access-token"
          name="token"
          type="password"
          autoComplete="off"
          value={token}
          onChange={(event) => setToken(event.target.value)}
          aria-invalid={Boolean(error || sessionError)}
          aria-describedby={error || sessionError ? 'login-error' : undefined}
          required
          disabled={pending || disabled}
        />
        {(error || sessionError) && (
          <p id="login-error" role="alert">
            {error || sessionError}
          </p>
        )}
        <Button type="submit" disabled={pending || disabled || !token.trim()}>
          {pending ? 'Signing in…' : 'Sign in'}
        </Button>
      </form>
    </main>
  )
}
