import { afterEach, expect, it, vi } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { MultiSelect } from './Filters'

afterEach(cleanup)

it('failed session search retains the draft and selection and retries the same search', async () => {
  const user = userEvent.setup()
  const retry = vi.fn<() => void>()
  const { rerender } = render(
    <MultiSelect
      label="Session"
      values={[]}
      selected={['saved-session']}
      onChange={() => undefined}
      error={{ message: 'Session values couldn’t load', onRetry: retry }}
    />,
  )
  await user.click(screen.getByRole('button', { name: 'Session 1' }))
  await user.type(screen.getByRole('textbox', { name: 'Search session' }), 'new')
  expect(screen.getByRole('alert')).toHaveTextContent('Session values couldn’t load')
  expect(screen.queryByText('No matching values')).not.toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Retry session search' }))
  expect(retry).toHaveBeenCalledOnce()
  rerender(
    <MultiSelect
      label="Session"
      values={['new-session']}
      selected={['saved-session']}
      onChange={() => undefined}
    />,
  )
  expect(screen.getByRole('textbox', { name: 'Search session' })).toHaveValue('new')
  expect(screen.getByRole('checkbox', { name: 'new-session' })).not.toBeChecked()
  await user.clear(screen.getByRole('textbox', { name: 'Search session' }))
  expect(screen.getByRole('checkbox', { name: 'saved-session' })).toBeChecked()
})

function Example() {
  const [selected, setSelected] = useState(['unavailable-model'])
  return (
    <>
      <MultiSelect
        label="Model"
        values={['model-a', 'model-b']}
        selected={selected}
        onChange={setSelected}
      />
      <output>{selected.join(',')}</output>
    </>
  )
}

function LocationExample() {
  const [selected, setSelected] = useState<string[]>([])
  return (
    <>
      <MultiSelect
        label="Repository"
        values={[
          { key: 'hash-1', name: 'tokeninsights' },
          { key: 'unknown', name: 'unknown' },
        ]}
        selected={selected}
        onChange={setSelected}
      />
      <output>{selected.join(',')}</output>
    </>
  )
}

it('keeps selected values available when other facets remove them, and supports search and clearing', async () => {
  const user = userEvent.setup()
  render(<Example />)
  await user.click(screen.getByRole('button', { name: /Model/ }))
  expect(screen.getByRole('checkbox', { name: 'unavailable-model' })).toBeChecked()
  await user.type(screen.getByRole('textbox', { name: 'Search model' }), 'model-a')
  expect(screen.queryByRole('checkbox', { name: 'model-b' })).not.toBeInTheDocument()
  await user.click(screen.getByRole('checkbox', { name: 'model-a' }))
  expect(screen.getByRole('status')).toHaveTextContent('unavailable-model,model-a')
  await user.click(screen.getByRole('button', { name: 'Clear' }))
  expect(screen.getByRole('status')).toBeEmptyDOMElement()
  await user.keyboard('{Escape}')
  expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
})

it('shows short location names while selecting stable keys', async () => {
  const user = userEvent.setup()
  render(<LocationExample />)
  await user.click(screen.getByRole('button', { name: 'Repository' }))
  await user.click(screen.getByRole('checkbox', { name: 'tokeninsights' }))
  expect(screen.getByRole('status')).toHaveTextContent('hash-1')
  expect(screen.getByRole('button', { name: 'Repository 1' })).toHaveTextContent('tokeninsights')
})

it('hides opaque keys when a selected location is unavailable', async () => {
  const user = userEvent.setup()
  render(
    <MultiSelect
      label="Repository"
      values={[]}
      selected={['opaque-key']}
      onChange={() => undefined}
      missingName="Repository (unavailable)"
    />,
  )
  const button = screen.getByRole('button', { name: 'Repository 1' })
  expect(button).toHaveTextContent('Repository (unavailable)')
  expect(button).not.toHaveTextContent('opaque-key')
  await user.click(button)
  expect(screen.getByRole('checkbox', { name: 'Repository (unavailable)' })).toBeChecked()
})
