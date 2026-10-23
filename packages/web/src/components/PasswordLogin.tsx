import { useState } from 'react'
import type { FormEvent } from 'react'
import { changeBrowserSession } from '../api'
import { passwordMaxCharacters, passwordMinCharacters, signup, validPassword } from '../account'
import { Button, Field, Heading, Input, StartupShell } from '@/ui'

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
      setError(
        `Use a password between ${passwordMinCharacters} and ${passwordMaxCharacters} characters.`,
      )
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
    <StartupShell>
      <Heading as="h1" variant="page">
        {creating ? 'Create your account' : 'Sign in to TokenInsights'}
      </Heading>
      <p>
        {creating
          ? 'Your account keeps your synced usage together.'
          : 'View your synced usage and manage your API keys.'}
      </p>
      <form
        className="grid w-full max-w-login gap-4 text-start"
        onSubmit={(event) => void submit(event)}
      >
        <Field
          label="Username"
          description={
            creating
              ? '3–32 letters, numbers, underscores or hyphens. Start with a letter or number.'
              : undefined
          }
        >
          {(control) => (
            <Input
              {...control}
              name="username"
              autoComplete="username"
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              required
              disabled={pending || disabled}
            />
          )}
        </Field>
        <Field
          label="Password"
          description={
            creating ? `${passwordMinCharacters}–${passwordMaxCharacters} characters.` : undefined
          }
          error={error || sessionError || undefined}
        >
          {(control) => (
            <Input
              {...control}
              name="password"
              type="password"
              autoComplete={creating ? 'new-password' : 'current-password'}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              required
              disabled={pending || disabled}
            />
          )}
        </Field>
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
          variant="link"
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
    </StartupShell>
  )
}
